package restaurant

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

type Service interface {
	ListRestaurants(context.Context) ([]restaurantservice.RestaurantSummary, error)
	CreateRestaurant(
		context.Context,
		restaurantservice.CreateRestaurantInput,
	) (restaurantservice.Restaurant, error)
	UpdateRestaurant(
		context.Context,
		string,
		restaurantservice.UpdateRestaurantInput,
	) (restaurantservice.Restaurant, error)
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
	mux.HandleFunc("GET /api/v1/restaurants", h.list)
	mux.HandleFunc("POST /api/v1/partner/restaurants", h.create)
	mux.HandleFunc("PATCH /api/v1/partner/restaurants/{restaurant_id}", h.update)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	restaurants, err := h.service.ListRestaurants(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list restaurants", "error", err)
		common.WriteError(
			r.Context(),
			h.logger,
			w,
			http.StatusInternalServerError,
			"INTERNAL_ERROR",
			"Internal server error",
		)
		return
	}

	common.WriteJSON(
		r.Context(),
		h.logger,
		w,
		http.StatusOK,
		listResponseFromService(restaurants),
	)
}
