package order

import (
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

type createRequest struct {
	UserID       string              `json:"user_id"`
	RestaurantID string              `json:"restaurant_id"`
	Items        []createItemRequest `json:"items"`
}

type createItemRequest struct {
	MenuItemID    string `json:"menu_item_id"`
	Quantity      int    `json:"quantity"`
	ExpectedPrice int64  `json:"expected_price"`
}

type response struct {
	ID              string                        `json:"id"`
	UserID          string                        `json:"user_id"`
	RestaurantID    string                        `json:"restaurant_id"`
	ExternalOrderID *string                       `json:"external_order_id"`
	Status          orderservice.Status           `json:"status"`
	RejectionReason *orderservice.RejectionReason `json:"rejection_reason"`
	TotalPrice      int64                         `json:"total_price"`
	Items           []itemResponse                `json:"items"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
}

type itemResponse struct {
	MenuItemID string `json:"menu_item_id"`
	Name       string `json:"name"`
	UnitPrice  int64  `json:"unit_price"`
	Quantity   int    `json:"quantity"`
}

type priceChangedResponse struct {
	Error priceChangedErrorBody `json:"error"`
}

type priceChangedErrorBody struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Details priceChangedDetails `json:"details"`
}

type priceChangedDetails struct {
	Items []priceChangedItem `json:"items"`
}

type priceChangedItem struct {
	MenuItemID    string `json:"menu_item_id"`
	ExpectedPrice int64  `json:"expected_price"`
	CurrentPrice  int64  `json:"current_price"`
}

func createInputFromRequest(request createRequest) orderservice.CreateOrderInput {
	items := make([]orderservice.CreateOrderItemInput, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, orderservice.CreateOrderItemInput{
			MenuItemID:    item.MenuItemID,
			Quantity:      item.Quantity,
			ExpectedPrice: item.ExpectedPrice,
		})
	}

	return orderservice.CreateOrderInput{
		UserID:       request.UserID,
		RestaurantID: request.RestaurantID,
		Items:        items,
	}
}

func responseFromService(value orderservice.Order) response {
	result := response{
		ID:              value.ID,
		UserID:          value.UserID,
		RestaurantID:    value.RestaurantID,
		ExternalOrderID: value.ExternalOrderID,
		Status:          value.Status,
		RejectionReason: value.RejectionReason,
		TotalPrice:      value.TotalPrice,
		Items:           make([]itemResponse, 0, len(value.Items)),
		CreatedAt:       value.CreatedAt,
		UpdatedAt:       value.UpdatedAt,
	}
	for _, item := range value.Items {
		result.Items = append(result.Items, itemResponse{
			MenuItemID: item.MenuItemID,
			Name:       item.Name,
			UnitPrice:  item.UnitPrice,
			Quantity:   item.Quantity,
		})
	}
	return result
}

func priceChangedResponseFromError(value *orderservice.PriceChangedError) priceChangedResponse {
	items := make([]priceChangedItem, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, priceChangedItem{
			MenuItemID:    item.MenuItemID,
			ExpectedPrice: item.ExpectedPrice,
			CurrentPrice:  item.CurrentPrice,
		})
	}

	return priceChangedResponse{
		Error: priceChangedErrorBody{
			Code:    "PRICE_CHANGED",
			Message: "One or more item prices have changed",
			Details: priceChangedDetails{Items: items},
		},
	}
}
