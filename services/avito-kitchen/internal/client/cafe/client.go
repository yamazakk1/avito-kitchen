package cafe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

const maxResponseBodySize = 1 << 20

type Client struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
	}
}

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

type submitOrderResponse struct {
	Status                 orderservice.Status          `json:"status"`
	ExternalOrderID        string                       `json:"external_order_id"`
	Reason                 orderservice.RejectionReason `json:"reason"`
	CurrentSnapshotVersion *int64                       `json:"current_snapshot_version"`
}

func (c *Client) SubmitOrder(
	ctx context.Context,
	order orderservice.DeliveryOrder,
) (orderservice.DeliveryResult, error) {
	requestBody := submitOrderRequest{
		AvitoOrderID:         order.ID,
		RestaurantExternalID: order.RestaurantExternalID,
		MenuSnapshotVersion:  order.MenuSnapshotVersion,
		Items:                make([]submitOrderItemRequest, 0, len(order.Items)),
	}
	for _, item := range order.Items {
		requestBody.Items = append(requestBody.Items, submitOrderItemRequest{
			ExternalItemID: item.ExternalMenuItemID,
			Quantity:       item.Quantity,
		})
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return orderservice.DeliveryResult{}, fmt.Errorf("marshal Cafe order request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		order.OrderEndpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return orderservice.DeliveryResult{}, fmt.Errorf("create Cafe order request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return orderservice.DeliveryResult{}, fmt.Errorf("send order to Cafe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodySize))
		return orderservice.DeliveryResult{}, fmt.Errorf("Cafe returned HTTP status %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodySize))
	decoder.DisallowUnknownFields()
	var response submitOrderResponse
	if err := decoder.Decode(&response); err != nil {
		return orderservice.DeliveryResult{}, fmt.Errorf("decode Cafe order response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return orderservice.DeliveryResult{}, fmt.Errorf("decode Cafe order response: response must contain one JSON object")
	}

	switch response.Status {
	case orderservice.StatusAccepted:
		externalOrderID := strings.TrimSpace(response.ExternalOrderID)
		if externalOrderID == "" {
			return orderservice.DeliveryResult{}, fmt.Errorf("decode Cafe order response: external_order_id is required for ACCEPTED")
		}
		return orderservice.DeliveryResult{
			Status:          orderservice.StatusAccepted,
			ExternalOrderID: externalOrderID,
		}, nil

	case orderservice.StatusRejected:
		if !isKnownRejectionReason(response.Reason) {
			return orderservice.DeliveryResult{}, fmt.Errorf("decode Cafe order response: unknown rejection reason %q", response.Reason)
		}
		return orderservice.DeliveryResult{
			Status:          orderservice.StatusRejected,
			RejectionReason: response.Reason,
		}, nil

	default:
		return orderservice.DeliveryResult{}, fmt.Errorf("decode Cafe order response: unknown status %q", response.Status)
	}
}

func isKnownRejectionReason(reason orderservice.RejectionReason) bool {
	switch reason {
	case orderservice.RejectionReasonOutOfStock,
		orderservice.RejectionReasonItemUnavailable,
		orderservice.RejectionReasonNotAcceptingOrders,
		orderservice.RejectionReasonStaleMenuSnapshot:
		return true
	default:
		return false
	}
}
