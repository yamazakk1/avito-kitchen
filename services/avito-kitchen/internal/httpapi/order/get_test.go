package order

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

func TestGetOrderHandler(t *testing.T) {
	createdAt := time.Date(2026, time.August, 29, 10, 0, 0, 0, time.UTC)
	service := serviceStub{get: func(orderID string) (orderservice.Order, error) {
		if orderID != "30000000-0000-4000-8000-000000000001" {
			t.Errorf("orderID = %q", orderID)
		}
		return orderservice.Order{
			ID:           orderID,
			UserID:       "user-1",
			RestaurantID: "00000000-0000-4000-8000-000000000001",
			Status:       orderservice.StatusProcessing,
			TotalPrice:   150000,
			Items: []orderservice.Item{
				{MenuItemID: "20000000-0000-4000-8000-000000000001", Name: "Margherita", UnitPrice: 75000, Quantity: 2},
			},
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}, nil
	}}

	mux := newTestMux(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/orders/30000000-0000-4000-8000-000000000001", nil)
	responseRecorder := httptest.NewRecorder()

	mux.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var body response
	if err := json.NewDecoder(responseRecorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "30000000-0000-4000-8000-000000000001" || body.Status != orderservice.StatusProcessing || body.TotalPrice != 150000 {
		t.Errorf("response = %#v", body)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Margherita" {
		t.Errorf("items = %#v", body.Items)
	}
}

func TestGetOrderHandlerMapsErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantText string
	}{
		{name: "not found", err: orderservice.ErrOrderNotFound, wantCode: http.StatusNotFound, wantText: `"code":"ORDER_NOT_FOUND"`},
		{name: "wrapped not found", err: errors.Join(errors.New("repository"), orderservice.ErrOrderNotFound), wantCode: http.StatusNotFound, wantText: `"code":"ORDER_NOT_FOUND"`},
		{name: "internal error", err: errors.New("database failed"), wantCode: http.StatusInternalServerError, wantText: `"code":"INTERNAL_ERROR"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := newTestMux(serviceStub{get: func(string) (orderservice.Order, error) {
				return orderservice.Order{}, tt.err
			}})
			request := httptest.NewRequest(http.MethodGet, "/api/v1/orders/30000000-0000-4000-8000-000000000001", nil)
			responseRecorder := httptest.NewRecorder()

			mux.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", responseRecorder.Code, tt.wantCode, responseRecorder.Body.String())
			}
			if !strings.Contains(responseRecorder.Body.String(), tt.wantText) {
				t.Errorf("body = %s, want substring %s", responseRecorder.Body.String(), tt.wantText)
			}
		})
	}
}
