package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	orderservice "github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/service/order"
)

type OrderService interface {
	SubmitOrder(context.Context, orderservice.SubmitOrderInput) (orderservice.Result, error)
}

type handler struct {
	logger       *slog.Logger
	orderService OrderService
}

func NewHandler(logger *slog.Logger, orderService OrderService) http.Handler {
	h := handler{
		logger:       logger,
		orderService: orderService,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", h.live)
	mux.HandleFunc("POST /api/v1/orders", h.submitOrder)
	return mux
}

func (h handler) writeJSON(ctx context.Context, w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.ErrorContext(ctx, "write JSON response", "error", err)
	}
}
