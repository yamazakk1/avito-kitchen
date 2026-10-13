package menu

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetMenu(r.Context(), r.PathValue("restaurant_id"))
	if err != nil {
		switch {
		case errors.Is(err, menuservice.ErrInvalidMenu):
			common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, menuservice.ErrRestaurantNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "RESTAURANT_NOT_FOUND", "Restaurant not found")
		default:
			h.logger.ErrorContext(r.Context(), "get restaurant menu", "error", err)
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

	common.WriteJSON(r.Context(), h.logger, w, http.StatusOK, menuResponseFromService(result))
}
