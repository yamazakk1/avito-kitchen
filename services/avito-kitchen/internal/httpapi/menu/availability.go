package menu

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

func (h *Handler) updateAvailability(w http.ResponseWriter, r *http.Request) {
	var request availabilityRequest
	if err := common.DecodeJSON(w, r, &request); err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	result, err := h.service.UpdateAvailability(
		r.Context(),
		r.PathValue("restaurant_id"),
		r.PathValue("external_item_id"),
		request.AvailableQuantity,
	)
	if err != nil {
		switch {
		case errors.Is(err, menuservice.ErrInvalidMenu):
			common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, menuservice.ErrRestaurantNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "RESTAURANT_NOT_FOUND", "Restaurant not found")
		case errors.Is(err, menuservice.ErrMenuItemNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "ITEM_NOT_FOUND", "Menu item not found")
		default:
			h.logger.ErrorContext(r.Context(), "update menu item availability", "error", err)
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

	common.WriteJSON(r.Context(), h.logger, w, http.StatusOK, availabilityResponseFromService(result))
}
