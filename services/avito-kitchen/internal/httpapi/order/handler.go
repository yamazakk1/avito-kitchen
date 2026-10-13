package order

import (
	"context"
	"log/slog"
	"net/http"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

type Service interface {
	CreateOrder(context.Context, string, orderservice.CreateOrderInput) (orderservice.Order, error)
	GetOrder(context.Context, string) (orderservice.Order, error)
}

type Handler struct {
	logger  *slog.Logger
	service Service
}

func NewHandler(logger *slog.Logger, service Service) *Handler {
	return &Handler{
		logger:  logger,
		service: service,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/orders", h.create)
	mux.HandleFunc("GET /api/v1/orders/{order_id}", h.get)
}
