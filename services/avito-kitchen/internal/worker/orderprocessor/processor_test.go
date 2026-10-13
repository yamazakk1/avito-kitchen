package orderprocessor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

type repositoryStub struct {
	orders            []orderservice.DeliveryOrder
	listErr           error
	accepted          map[string]string
	rejected          map[string]orderservice.RejectionReason
	markAcceptedErr   error
	markRejectedError error
}

func (r *repositoryStub) ListProcessingOrders(context.Context) ([]orderservice.DeliveryOrder, error) {
	return r.orders, r.listErr
}

func (r *repositoryStub) MarkAccepted(_ context.Context, orderID, externalOrderID string) error {
	if r.markAcceptedErr != nil {
		return r.markAcceptedErr
	}
	if r.accepted == nil {
		r.accepted = make(map[string]string)
	}
	r.accepted[orderID] = externalOrderID
	return nil
}

func (r *repositoryStub) MarkRejected(_ context.Context, orderID string, reason orderservice.RejectionReason) error {
	if r.markRejectedError != nil {
		return r.markRejectedError
	}
	if r.rejected == nil {
		r.rejected = make(map[string]orderservice.RejectionReason)
	}
	r.rejected[orderID] = reason
	return nil
}

type cafeClientStub struct {
	results map[string]orderservice.DeliveryResult
	errors  map[string]error
}

func (c cafeClientStub) SubmitOrder(_ context.Context, order orderservice.DeliveryOrder) (orderservice.DeliveryResult, error) {
	if err := c.errors[order.ID]; err != nil {
		return orderservice.DeliveryResult{}, err
	}
	return c.results[order.ID], nil
}

func TestProcessorFinalizesAcceptedAndRejectedOrders(t *testing.T) {
	repository := &repositoryStub{
		orders: []orderservice.DeliveryOrder{
			{ID: "accepted-order"},
			{ID: "rejected-order"},
		},
	}
	client := cafeClientStub{results: map[string]orderservice.DeliveryResult{
		"accepted-order": {
			Status:          orderservice.StatusAccepted,
			ExternalOrderID: "cafe-accepted-order",
		},
		"rejected-order": {
			Status:          orderservice.StatusRejected,
			RejectionReason: orderservice.RejectionReasonOutOfStock,
		},
	}}

	processor := NewProcessor(discardLogger(), repository, client, time.Hour)
	processor.processAvailable(context.Background())

	if repository.accepted["accepted-order"] != "cafe-accepted-order" {
		t.Errorf("accepted = %#v", repository.accepted)
	}
	if repository.rejected["rejected-order"] != orderservice.RejectionReasonOutOfStock {
		t.Errorf("rejected = %#v", repository.rejected)
	}
}

func TestProcessorLeavesTechnicalFailureProcessingAndContinues(t *testing.T) {
	repository := &repositoryStub{
		orders: []orderservice.DeliveryOrder{
			{ID: "failed-order"},
			{ID: "accepted-order"},
		},
	}
	client := cafeClientStub{
		errors: map[string]error{"failed-order": errors.New("connection refused")},
		results: map[string]orderservice.DeliveryResult{
			"accepted-order": {
				Status:          orderservice.StatusAccepted,
				ExternalOrderID: "cafe-order",
			},
		},
	}

	processor := NewProcessor(discardLogger(), repository, client, time.Hour)
	processor.processAvailable(context.Background())

	if _, ok := repository.accepted["failed-order"]; ok {
		t.Fatal("technical failure was marked accepted")
	}
	if _, ok := repository.rejected["failed-order"]; ok {
		t.Fatal("technical failure was marked rejected")
	}
	if repository.accepted["accepted-order"] != "cafe-order" {
		t.Errorf("later order was not processed: %#v", repository.accepted)
	}
}

func TestProcessorRunStopsWithContext(t *testing.T) {
	repository := &repositoryStub{}
	processor := NewProcessor(discardLogger(), repository, cafeClientStub{}, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		processor.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
