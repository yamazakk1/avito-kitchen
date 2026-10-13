package menu

import (
	"context"
	"log/slog"
	"net/http"

	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

type Service interface {
	GetMenu(context.Context, string) (menuservice.Menu, error)
	ReplaceSnapshot(context.Context, string, menuservice.ReplaceSnapshotInput) (menuservice.SnapshotResult, error)
	UpdateAvailability(context.Context, string, string, int) (menuservice.AvailabilityResult, error)
}

type Handler struct {
	logger  *slog.Logger
	service Service
}

func NewHandler(logger *slog.Logger, service Service) *Handler {
	return &Handler{logger: logger, service: service}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/restaurants/{restaurant_id}/menu", h.get)
	mux.HandleFunc("PUT /api/v1/partner/restaurants/{restaurant_id}/menu", h.replaceSnapshot)
	mux.HandleFunc(
		"PATCH /api/v1/partner/restaurants/{restaurant_id}/menu-items/{external_item_id}/availability",
		h.updateAvailability,
	)
}
