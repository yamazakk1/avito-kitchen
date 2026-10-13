package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	orderservice "github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/service/order"
)

type submitOrderRequest struct {
	AvitoOrderID         string                   `json:"avito_order_id"`
	RestaurantExternalID string                   `json:"restaurant_external_id"`
	MenuSnapshotVersion  int64                    `json:"menu_snapshot_version"`
	Items                []submitOrderItemRequest `json:"items"`
}

type submitOrderItemRequest struct {
	ExternalItemID string `json:"external_item_id"`
	Quantity       int    `json:"quantity"`
}

type acceptedOrderResponse struct {
	Status          orderservice.Status `json:"status"`
	ExternalOrderID string              `json:"external_order_id"`
}

type rejectedOrderResponse struct {
	Status                 orderservice.Status          `json:"status"`
	Reason                 orderservice.RejectionReason `json:"reason"`
	CurrentSnapshotVersion *int64                       `json:"current_snapshot_version,omitempty"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h handler) submitOrder(w http.ResponseWriter, r *http.Request) {
	request, err := decodeSubmitOrderRequest(r)
	if err != nil {
		h.writeError(r, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		return
	}

	result, err := h.orderService.SubmitOrder(r.Context(), requestToService(request))
	if err != nil {
		switch {
		case errors.Is(err, orderservice.ErrInvalidOrder):
			h.writeError(r, w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request")
		case errors.Is(err, orderservice.ErrAvitoOrderIDReused):
			h.writeError(r, w, http.StatusBadRequest, "AVITO_ORDER_ID_REUSED", "avito_order_id was already used for another request")
		default:
			h.logger.ErrorContext(r.Context(), "submit order", "error", err)
			h.writeError(r, w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}

	if result.Status == orderservice.StatusAccepted {
		h.writeJSON(r.Context(), w, http.StatusOK, acceptedOrderResponse{
			Status:          result.Status,
			ExternalOrderID: result.ExternalOrderID,
		})
		return
	}

	h.writeJSON(r.Context(), w, http.StatusOK, rejectedOrderResponse{
		Status:                 result.Status,
		Reason:                 result.RejectionReason,
		CurrentSnapshotVersion: result.CurrentSnapshotVersion,
	})
}

func decodeSubmitOrderRequest(r *http.Request) (submitOrderRequest, error) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request submitOrderRequest
	if err := decoder.Decode(&request); err != nil {
		return submitOrderRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return submitOrderRequest{}, errors.New("request body must contain one JSON object")
	}
	return request, nil
}

func requestToService(request submitOrderRequest) orderservice.SubmitOrderInput {
	items := make([]orderservice.SubmitOrderItemInput, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, orderservice.SubmitOrderItemInput{
			ExternalItemID: item.ExternalItemID,
			Quantity:       item.Quantity,
		})
	}
	return orderservice.SubmitOrderInput{
		AvitoOrderID:         request.AvitoOrderID,
		RestaurantExternalID: request.RestaurantExternalID,
		MenuSnapshotVersion:  request.MenuSnapshotVersion,
		Items:                items,
	}
}

func (h handler) writeError(r *http.Request, w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(r.Context(), w, status, errorResponse{Error: errorBody{
		Code:    code,
		Message: message,
	}})
}
