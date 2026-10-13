package menu

import (
	"time"

	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

type menuResponse struct {
	RestaurantID    string             `json:"restaurant_id"`
	SnapshotVersion int64              `json:"snapshot_version"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Items           []menuItemResponse `json:"items"`
}

type menuItemResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	Price             int64   `json:"price"`
	AvailableQuantity int     `json:"available_quantity"`
}

type snapshotRequest struct {
	SnapshotVersion int64                 `json:"snapshot_version"`
	Items           []snapshotItemRequest `json:"items"`
}

type snapshotItemRequest struct {
	ExternalID        string  `json:"external_id"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	Price             int64   `json:"price"`
	AvailableQuantity int     `json:"available_quantity"`
}

type snapshotResponse struct {
	SnapshotVersion int64     `json:"snapshot_version"`
	UpdatedAt       time.Time `json:"updated_at"`
	ItemsCount      int       `json:"items_count"`
}

type availabilityRequest struct {
	AvailableQuantity int `json:"available_quantity"`
}

type availabilityResponse struct {
	ExternalItemID    string    `json:"external_item_id"`
	AvailableQuantity int       `json:"available_quantity"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type staleSnapshotResponse struct {
	Error staleSnapshotErrorBody `json:"error"`
}

type staleSnapshotErrorBody struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Details staleSnapshotDetails `json:"details"`
}

type staleSnapshotDetails struct {
	CurrentSnapshotVersion  int64 `json:"current_snapshot_version"`
	ReceivedSnapshotVersion int64 `json:"received_snapshot_version"`
}

func menuResponseFromService(value menuservice.Menu) menuResponse {
	response := menuResponse{
		RestaurantID:    value.RestaurantID,
		SnapshotVersion: value.SnapshotVersion,
		UpdatedAt:       value.UpdatedAt,
		Items:           make([]menuItemResponse, 0, len(value.Items)),
	}
	for _, item := range value.Items {
		response.Items = append(response.Items, menuItemResponse{
			ID:                item.ID,
			Name:              item.Name,
			Description:       item.Description,
			Price:             item.Price,
			AvailableQuantity: item.AvailableQuantity,
		})
	}
	return response
}

func snapshotInputFromRequest(request snapshotRequest) menuservice.ReplaceSnapshotInput {
	items := make([]menuservice.SnapshotItemInput, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, menuservice.SnapshotItemInput{
			ExternalID:        item.ExternalID,
			Name:              item.Name,
			Description:       item.Description,
			Price:             item.Price,
			AvailableQuantity: item.AvailableQuantity,
		})
	}
	return menuservice.ReplaceSnapshotInput{
		SnapshotVersion: request.SnapshotVersion,
		Items:           items,
	}
}

func snapshotResponseFromService(value menuservice.SnapshotResult) snapshotResponse {
	return snapshotResponse{
		SnapshotVersion: value.SnapshotVersion,
		UpdatedAt:       value.UpdatedAt,
		ItemsCount:      value.ItemsCount,
	}
}

func availabilityResponseFromService(value menuservice.AvailabilityResult) availabilityResponse {
	return availabilityResponse{
		ExternalItemID:    value.ExternalItemID,
		AvailableQuantity: value.AvailableQuantity,
		UpdatedAt:         value.UpdatedAt,
	}
}
