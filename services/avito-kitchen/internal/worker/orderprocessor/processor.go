package orderprocessor

import (
	"context"
	"log/slog"
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

type Repository interface {
	ListProcessingOrders(context.Context) ([]orderservice.DeliveryOrder, error)
	MarkAccepted(context.Context, string, string) error
	MarkRejected(context.Context, string, orderservice.RejectionReason) error
}

type CafeClient interface {
	SubmitOrder(context.Context, orderservice.DeliveryOrder) (orderservice.DeliveryResult, error)
}

type Processor struct {
	logger       *slog.Logger
	repository   Repository
	cafeClient   CafeClient
	pollInterval time.Duration
}

func NewProcessor(
	logger *slog.Logger,
	repository Repository,
	cafeClient CafeClient,
	pollInterval time.Duration,
) *Processor {
	return &Processor{
		logger:       logger,
		repository:   repository,
		cafeClient:   cafeClient,
		pollInterval: pollInterval,
	}
}

func (p *Processor) Run(ctx context.Context) {
	p.processAvailable(ctx)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processAvailable(ctx)
		}
	}
}

func (p *Processor) processAvailable(ctx context.Context) {
	orders, err := p.repository.ListProcessingOrders(ctx)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.ErrorContext(ctx, "list processing orders", "error", err)
		}
		return
	}

	for _, order := range orders {
		if ctx.Err() != nil {
			return
		}

		result, err := p.cafeClient.SubmitOrder(ctx, order)
		if err != nil {
			if ctx.Err() == nil {
				p.logger.WarnContext(
					ctx,
					"deliver order to Cafe failed; order remains PROCESSING",
					"order_id", order.ID,
					"error", err,
				)
			}
			continue
		}

		switch result.Status {
		case orderservice.StatusAccepted:
			if err := p.repository.MarkAccepted(ctx, order.ID, result.ExternalOrderID); err != nil {
				if ctx.Err() == nil {
					p.logger.ErrorContext(ctx, "mark order accepted", "order_id", order.ID, "error", err)
				}
				continue
			}
			p.logger.InfoContext(ctx, "order accepted by Cafe", "order_id", order.ID, "external_order_id", result.ExternalOrderID)

		case orderservice.StatusRejected:
			if err := p.repository.MarkRejected(ctx, order.ID, result.RejectionReason); err != nil {
				if ctx.Err() == nil {
					p.logger.ErrorContext(ctx, "mark order rejected", "order_id", order.ID, "error", err)
				}
				continue
			}
			p.logger.InfoContext(ctx, "order rejected by Cafe", "order_id", order.ID, "reason", result.RejectionReason)
		}
	}
}
