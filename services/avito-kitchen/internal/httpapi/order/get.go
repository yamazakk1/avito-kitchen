package order

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.GetOrder(r.Context(), r.PathValue("order_id"))
	if err != nil {
		switch {
		case errors.Is(err, orderservice.ErrOrderNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found")
		default:
			h.logger.ErrorContext(r.Context(), "get order", "error", err)
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

	common.WriteJSON(r.Context(), h.logger, w, http.StatusOK, responseFromService(order))
}
