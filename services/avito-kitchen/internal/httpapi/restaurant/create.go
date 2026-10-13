package restaurant

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := common.DecodeJSON(w, r, &request); err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	input, err := createInputFromRequest(request)
	if err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	created, err := h.service.CreateRestaurant(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, restaurantservice.ErrInvalidRestaurant):
			common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, restaurantservice.ErrRestaurantAlreadyExists):
			common.WriteError(
				r.Context(),
				h.logger,
				w,
				http.StatusConflict,
				"RESTAURANT_ALREADY_EXISTS",
				"Restaurant already exists",
			)
		default:
			h.logger.ErrorContext(r.Context(), "create restaurant", "error", err)
			common.WriteError(
				r.Context(),
				h.logger,
				w,
				http.StatusInternalServerError,
				"INTERNAL_ERROR",
				"Internal server error",
			)
		}
		return
	}

	common.WriteJSON(
		r.Context(),
		h.logger,
		w,
		http.StatusCreated,
		responseFromService(created),
	)
}
