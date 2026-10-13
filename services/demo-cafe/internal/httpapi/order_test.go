package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	orderservice "github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/service/order"
)

func TestSubmitOrderAccepted(t *testing.T) {
	service := stubOrderService{result: orderservice.Result{
		Status:          orderservice.StatusAccepted,
		ExternalOrderID: "cafe-order-1",
	}}
	recorder := performSubmitOrder(t, service, validOrderJSON())

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	want := `{"status":"ACCEPTED","external_order_id":"cafe-order-1"}`
	if strings.TrimSpace(recorder.Body.String()) != want {
		t.Errorf("body = %q, want %q", recorder.Body.String(), want)
	}
}

func TestSubmitOrderRejectedStaleSnapshot(t *testing.T) {
	version := int64(2)
	service := stubOrderService{result: orderservice.Result{
		Status:                 orderservice.StatusRejected,
		RejectionReason:        orderservice.RejectionReasonStaleMenuSnapshot,
		CurrentSnapshotVersion: &version,
	}}
	recorder := performSubmitOrder(t, service, validOrderJSON())

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	want := `{"status":"REJECTED","reason":"STALE_MENU_SNAPSHOT","current_snapshot_version":2}`
	if strings.TrimSpace(recorder.Body.String()) != want {
		t.Errorf("body = %q, want %q", recorder.Body.String(), want)
	}
}

func TestSubmitOrderBadRequest(t *testing.T) {
	tests := []struct {
		name    string
		service stubOrderService
		body    string
		code    string
	}{
		{
			name: "malformed json",
			body: `{`,
			code: "INVALID_REQUEST",
		},
		{
			name:    "invalid domain input",
			service: stubOrderService{err: orderservice.ErrInvalidOrder},
			body:    validOrderJSON(),
			code:    "INVALID_REQUEST",
		},
		{
			name:    "reused avito order id",
			service: stubOrderService{err: orderservice.ErrAvitoOrderIDReused},
			body:    validOrderJSON(),
			code:    "AVITO_ORDER_ID_REUSED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performSubmitOrder(t, tt.service, tt.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if !strings.Contains(recorder.Body.String(), `"code":"`+tt.code+`"`) {
				t.Errorf("body = %q, want error code %s", recorder.Body.String(), tt.code)
			}
		})
	}
}

func TestSubmitOrderInternalError(t *testing.T) {
	recorder := performSubmitOrder(t, stubOrderService{err: errors.New("boom")}, validOrderJSON())
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Errorf("body = %q, want INTERNAL_ERROR", recorder.Body.String())
	}
}

func performSubmitOrder(t *testing.T, service stubOrderService, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(recorder, request)
	return recorder
}

func validOrderJSON() string {
	return `{
		"avito_order_id":"00000000-0000-4000-8000-000000000001",
		"restaurant_external_id":"demo-cafe-1",
		"menu_snapshot_version":1,
		"items":[{"external_item_id":"pizza-1","quantity":1}]
	}`
}
