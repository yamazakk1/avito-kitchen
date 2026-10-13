package order

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

type serviceStub struct {
	create func(string, orderservice.CreateOrderInput) (orderservice.Order, error)
	get    func(string) (orderservice.Order, error)
}

func (s serviceStub) CreateOrder(
	_ context.Context,
	idempotencyKey string,
	input orderservice.CreateOrderInput,
) (orderservice.Order, error) {
	if s.create != nil {
		return s.create(idempotencyKey, input)
	}
	return orderservice.Order{}, nil
}

func (s serviceStub) GetOrder(_ context.Context, orderID string) (orderservice.Order, error) {
	if s.get != nil {
		return s.get(orderID)
	}
	return orderservice.Order{}, nil
}

func TestCreateOrderHandler(t *testing.T) {
	createdAt := time.Date(2026, time.August, 29, 10, 0, 0, 0, time.UTC)
	service := serviceStub{create: func(key string, input orderservice.CreateOrderInput) (orderservice.Order, error) {
		if key != "postman-key" {
			t.Errorf("Idempotency-Key = %q", key)
		}
		if input.UserID != "user-1" || input.RestaurantID != "00000000-0000-4000-8000-000000000001" {
			t.Errorf("input = %#v", input)
		}
		if len(input.Items) != 1 || input.Items[0].Quantity != 2 || input.Items[0].ExpectedPrice != 75000 {
			t.Errorf("items = %#v", input.Items)
		}
		return orderservice.Order{
			ID:           "30000000-0000-4000-8000-000000000001",
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
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{
		"user_id":"user-1",
		"restaurant_id":"00000000-0000-4000-8000-000000000001",
		"items":[{"menu_item_id":"20000000-0000-4000-8000-000000000001","quantity":2,"expected_price":75000}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "postman-key")
	responseRecorder := httptest.NewRecorder()

	mux.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusCreated {
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

func TestCreateOrderHandlerMapsErrors(t *testing.T) {
	priceError := &orderservice.PriceChangedError{Items: []orderservice.PriceChangedItem{
		{MenuItemID: "20000000-0000-4000-8000-000000000001", ExpectedPrice: 70000, CurrentPrice: 75000},
	}}

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantText string
	}{
		{name: "invalid request", err: orderservice.ErrInvalidOrder, wantCode: http.StatusBadRequest, wantText: `"code":"INVALID_REQUEST"`},
		{name: "restaurant not found", err: orderservice.ErrRestaurantNotFound, wantCode: http.StatusNotFound, wantText: `"code":"RESTAURANT_NOT_FOUND"`},
		{name: "item not found", err: orderservice.ErrMenuItemNotFound, wantCode: http.StatusNotFound, wantText: `"code":"ITEM_NOT_FOUND"`},
		{name: "restaurant unavailable", err: orderservice.ErrRestaurantUnavailable, wantCode: http.StatusConflict, wantText: `"code":"RESTAURANT_UNAVAILABLE"`},
		{name: "item unavailable", err: orderservice.ErrItemUnavailable, wantCode: http.StatusConflict, wantText: `"code":"ITEM_UNAVAILABLE"`},
		{name: "insufficient stock", err: orderservice.ErrInsufficientStock, wantCode: http.StatusConflict, wantText: `"code":"INSUFFICIENT_STOCK"`},
		{name: "price changed", err: priceError, wantCode: http.StatusConflict, wantText: `"current_price":75000`},
		{name: "idempotency reused", err: orderservice.ErrIdempotencyKeyReused, wantCode: http.StatusConflict, wantText: `"code":"IDEMPOTENCY_KEY_REUSED"`},
		{name: "internal error", err: errors.New("database failed"), wantCode: http.StatusInternalServerError, wantText: `"code":"INTERNAL_ERROR"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := newTestMux(serviceStub{create: func(string, orderservice.CreateOrderInput) (orderservice.Order, error) {
				return orderservice.Order{}, tt.err
			}})
			request := validCreateOrderHTTPRequest()
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

func TestCreateOrderHandlerRejectsMalformedJSON(t *testing.T) {
	mux := newTestMux(serviceStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{"user_id":`))
	request.Header.Set("Idempotency-Key", "key")
	responseRecorder := httptest.NewRecorder()

	mux.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", responseRecorder.Code, responseRecorder.Body.String())
	}
}

func newTestMux(service Service) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	NewHandler(logger, service).RegisterRoutes(mux)
	return mux
}

func validCreateOrderHTTPRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{
		"user_id":"user-1",
		"restaurant_id":"00000000-0000-4000-8000-000000000001",
		"items":[{"menu_item_id":"20000000-0000-4000-8000-000000000001","quantity":1,"expected_price":75000}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "key")
	return request
}
