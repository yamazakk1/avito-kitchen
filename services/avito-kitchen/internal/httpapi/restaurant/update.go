package restaurant

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var request updateRequest
	if err := common.DecodeJSON(w, r, &request); err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	input, err := updateInputFromRequest(request)
	if err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	updated, err := h.service.UpdateRestaurant(r.Context(), r.PathValue("restaurant_id"), input)
	if err != nil {
		switch {
		case errors.Is(err, restaurantservice.ErrInvalidRestaurant):
			common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, restaurantservice.ErrRestaurantNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "RESTAURANT_NOT_FOUND", "Restaurant not found")
		default:
			h.logger.ErrorContext(r.Context(), "update restaurant", "error", err)
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
		http.StatusOK,
		responseFromService(updated),
	)
}
