package order

import (
	"context"
	"errors"
	"testing"
)

func TestSubmitOrderAcceptedAndIdempotent(t *testing.T) {
	service := NewService(State{
		RestaurantExternalID: "demo-cafe-1",
		MenuSnapshotVersion:  1,
		IsAcceptingOrders:    true,
		Items: map[string]MenuItem{
			"pizza-1": {AvailableQuantity: 2, IsAvailable: true},
		},
	})

	input := validInput()
	input.Items[0].Quantity = 2

	first, err := service.SubmitOrder(context.Background(), input)
	if err != nil {
		t.Fatalf("SubmitOrder() error = %v", err)
	}
	if first.Status != StatusAccepted {
		t.Fatalf("status = %q, want %q", first.Status, StatusAccepted)
	}
	if first.ExternalOrderID == "" {
		t.Fatal("external order id is empty")
	}

	second, err := service.SubmitOrder(context.Background(), input)
	if err != nil {
		t.Fatalf("retry SubmitOrder() error = %v", err)
	}
	if second != first {
		t.Errorf("retry result = %#v, want %#v", second, first)
	}

	another := input
	another.AvitoOrderID = "00000000-0000-4000-8000-000000000002"
	result, err := service.SubmitOrder(context.Background(), another)
	if err != nil {
		t.Fatalf("second logical order error = %v", err)
	}
	if result.Status != StatusRejected || result.RejectionReason != RejectionReasonOutOfStock {
		t.Errorf("second logical order = %#v, want OUT_OF_STOCK", result)
	}
}

func TestSubmitOrderRejections(t *testing.T) {
	tests := []struct {
		name       string
		state      State
		input      SubmitOrderInput
		wantReason RejectionReason
		wantVer    *int64
	}{
		{
			name: "stale snapshot",
			state: State{
				RestaurantExternalID: "demo-cafe-1",
				MenuSnapshotVersion:  2,
				IsAcceptingOrders:    true,
				Items:                defaultTestItems(),
			},
			input:      validInput(),
			wantReason: RejectionReasonStaleMenuSnapshot,
			wantVer:    int64Ptr(2),
		},
		{
			name: "not accepting orders",
			state: State{
				RestaurantExternalID: "demo-cafe-1",
				MenuSnapshotVersion:  1,
				IsAcceptingOrders:    false,
				Items:                defaultTestItems(),
			},
			input:      validInput(),
			wantReason: RejectionReasonNotAcceptingOrders,
		},
		{
			name: "unknown item",
			state: State{
				RestaurantExternalID: "demo-cafe-1",
				MenuSnapshotVersion:  1,
				IsAcceptingOrders:    true,
				Items:                defaultTestItems(),
			},
			input: func() SubmitOrderInput {
				input := validInput()
				input.Items[0].ExternalItemID = "unknown"
				return input
			}(),
			wantReason: RejectionReasonItemUnavailable,
		},
		{
			name: "unavailable item",
			state: State{
				RestaurantExternalID: "demo-cafe-1",
				MenuSnapshotVersion:  1,
				IsAcceptingOrders:    true,
				Items: map[string]MenuItem{
					"pizza-1": {AvailableQuantity: 10, IsAvailable: false},
				},
			},
			input:      validInput(),
			wantReason: RejectionReasonItemUnavailable,
		},
		{
			name: "out of stock",
			state: State{
				RestaurantExternalID: "demo-cafe-1",
				MenuSnapshotVersion:  1,
				IsAcceptingOrders:    true,
				Items: map[string]MenuItem{
					"pizza-1": {AvailableQuantity: 0, IsAvailable: true},
				},
			},
			input:      validInput(),
			wantReason: RejectionReasonOutOfStock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewService(tt.state).SubmitOrder(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("SubmitOrder() error = %v", err)
			}
			if result.Status != StatusRejected {
				t.Fatalf("status = %q, want %q", result.Status, StatusRejected)
			}
			if result.RejectionReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", result.RejectionReason, tt.wantReason)
			}
			if tt.wantVer == nil {
				if result.CurrentSnapshotVersion != nil {
					t.Errorf("current snapshot version = %v, want nil", *result.CurrentSnapshotVersion)
				}
				return
			}
			if result.CurrentSnapshotVersion == nil || *result.CurrentSnapshotVersion != *tt.wantVer {
				t.Errorf("current snapshot version = %v, want %d", result.CurrentSnapshotVersion, *tt.wantVer)
			}
		})
	}
}

func TestSubmitOrderValidationAndReusedID(t *testing.T) {
	service := NewService(State{
		RestaurantExternalID: "demo-cafe-1",
		MenuSnapshotVersion:  1,
		IsAcceptingOrders:    true,
		Items:                defaultTestItems(),
	})

	invalid := validInput()
	invalid.Items[0].Quantity = 0
	if _, err := service.SubmitOrder(context.Background(), invalid); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("invalid quantity error = %v, want ErrInvalidOrder", err)
	}

	future := validInput()
	future.MenuSnapshotVersion = 2
	if _, err := service.SubmitOrder(context.Background(), future); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("future version error = %v, want ErrInvalidOrder", err)
	}

	first := validInput()
	if _, err := service.SubmitOrder(context.Background(), first); err != nil {
		t.Fatalf("first SubmitOrder() error = %v", err)
	}

	reused := first
	reused.Items = []SubmitOrderItemInput{{ExternalItemID: "pizza-1", Quantity: 2}}
	if _, err := service.SubmitOrder(context.Background(), reused); !errors.Is(err, ErrAvitoOrderIDReused) {
		t.Fatalf("reused avito_order_id error = %v, want ErrAvitoOrderIDReused", err)
	}
}

func validInput() SubmitOrderInput {
	return SubmitOrderInput{
		AvitoOrderID:         "00000000-0000-4000-8000-000000000001",
		RestaurantExternalID: "demo-cafe-1",
		MenuSnapshotVersion:  1,
		Items: []SubmitOrderItemInput{
			{ExternalItemID: "pizza-1", Quantity: 1},
		},
	}
}

func defaultTestItems() map[string]MenuItem {
	return map[string]MenuItem{
		"pizza-1": {AvailableQuantity: 10, IsAvailable: true},
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
