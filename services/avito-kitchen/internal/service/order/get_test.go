package order

import (
	"context"
	"errors"
	"testing"
)

const testOrderID = "30000000-0000-4000-8000-000000000001"

func TestGetOrderValidatesAndNormalizesID(t *testing.T) {
	var capturedID string
	service := NewService(repositoryStub{get: func(orderID string) (Order, error) {
		capturedID = orderID
		return Order{ID: orderID}, nil
	}})

	got, err := service.GetOrder(context.Background(), "  "+testOrderID+"  ")
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if capturedID != testOrderID {
		t.Errorf("repository orderID = %q, want %q", capturedID, testOrderID)
	}
	if got.ID != testOrderID {
		t.Errorf("order ID = %q, want %q", got.ID, testOrderID)
	}
}

func TestGetOrderRejectsInvalidIDAsNotFound(t *testing.T) {
	called := false
	service := NewService(repositoryStub{get: func(string) (Order, error) {
		called = true
		return Order{}, nil
	}})

	_, err := service.GetOrder(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("error = %v, want ErrOrderNotFound", err)
	}
	if called {
		t.Fatal("repository must not be called for invalid order id")
	}
}

func TestGetOrderPreservesRepositoryError(t *testing.T) {
	service := NewService(repositoryStub{get: func(string) (Order, error) {
		return Order{}, ErrOrderNotFound
	}})

	_, err := service.GetOrder(context.Background(), testOrderID)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("error = %v, want ErrOrderNotFound", err)
	}
}
