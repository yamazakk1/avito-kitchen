package cafe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

func TestClientSubmitOrderAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ACCEPTED","external_order_id":"cafe-order-1"}`))
	}))
	defer server.Close()

	client := NewClient(time.Second)
	order := testDeliveryOrder(server.URL)

	result, err := client.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("SubmitOrder() error = %v", err)
	}
	if result.Status != orderservice.StatusAccepted || result.ExternalOrderID != "cafe-order-1" {
		t.Errorf("result = %#v", result)
	}
}

func TestClientSubmitOrderRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"REJECTED","reason":"OUT_OF_STOCK"}`))
	}))
	defer server.Close()

	client := NewClient(time.Second)
	result, err := client.SubmitOrder(context.Background(), testDeliveryOrder(server.URL))
	if err != nil {
		t.Fatalf("SubmitOrder() error = %v", err)
	}
	if result.Status != orderservice.StatusRejected || result.RejectionReason != orderservice.RejectionReasonOutOfStock {
		t.Errorf("result = %#v", result)
	}
}

func TestClientSubmitOrderTreatsTechnicalResponseAsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(time.Second)
	if _, err := client.SubmitOrder(context.Background(), testDeliveryOrder(server.URL)); err == nil {
		t.Fatal("SubmitOrder() error = nil, want an error")
	}
}

func TestClientSubmitOrderRejectsInvalidCafeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ACCEPTED","external_order_id":""}`))
	}))
	defer server.Close()

	client := NewClient(time.Second)
	if _, err := client.SubmitOrder(context.Background(), testDeliveryOrder(server.URL)); err == nil {
		t.Fatal("SubmitOrder() error = nil, want an error")
	}
}

func testDeliveryOrder(endpoint string) orderservice.DeliveryOrder {
	return orderservice.DeliveryOrder{
		ID:                   "30000000-0000-4000-8000-000000000001",
		RestaurantExternalID: "demo-cafe-1",
		OrderEndpoint:        endpoint,
		MenuSnapshotVersion:  1,
		Items: []orderservice.DeliveryItem{
			{ExternalMenuItemID: "pizza-1", Quantity: 2},
		},
	}
}
