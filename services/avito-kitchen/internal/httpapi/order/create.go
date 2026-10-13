package order

import (
	"errors"
	"net/http"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := common.DecodeJSON(w, r, &request); err != nil {
		common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	created, err := h.service.CreateOrder(
		r.Context(),
		r.Header.Get("Idempotency-Key"),
		createInputFromRequest(request),
	)
	if err != nil {
		var priceChangedError *orderservice.PriceChangedError
		switch {
		case errors.Is(err, orderservice.ErrInvalidOrder):
			common.WriteError(r.Context(), h.logger, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, orderservice.ErrRestaurantNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "RESTAURANT_NOT_FOUND", "Restaurant not found")
		case errors.Is(err, orderservice.ErrMenuItemNotFound):
			common.WriteError(r.Context(), h.logger, w, http.StatusNotFound, "ITEM_NOT_FOUND", "Menu item not found")
		case errors.Is(err, orderservice.ErrRestaurantUnavailable):
			common.WriteError(
				r.Context(),
				h.logger,
				w,
				http.StatusConflict,
				"RESTAURANT_UNAVAILABLE",
				"Restaurant is not accepting orders now",
			)
		case errors.Is(err, orderservice.ErrItemUnavailable):
			common.WriteError(r.Context(), h.logger, w, http.StatusConflict, "ITEM_UNAVAILABLE", "Menu item is unavailable")
		case errors.Is(err, orderservice.ErrInsufficientStock):
			common.WriteError(r.Context(), h.logger, w, http.StatusConflict, "INSUFFICIENT_STOCK", "Insufficient item stock")
		case errors.As(err, &priceChangedError):
			common.WriteJSON(
				r.Context(),
				h.logger,
				w,
				http.StatusConflict,
				priceChangedResponseFromError(priceChangedError),
			)
		case errors.Is(err, orderservice.ErrIdempotencyKeyReused):
			common.WriteError(
				r.Context(),
				h.logger,
				w,
				http.StatusConflict,
				"IDEMPOTENCY_KEY_REUSED",
				"Idempotency-Key was already used for another request",
			)
		default:
			h.logger.ErrorContext(r.Context(), "create order", "error", err)
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

	common.WriteJSON(r.Context(), h.logger, w, http.StatusCreated, responseFromService(created))
}
