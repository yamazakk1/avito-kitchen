package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var (
	ErrInvalidOrder       = errors.New("invalid order")
	ErrAvitoOrderIDReused = errors.New("avito order id reused")
)

func (s *Service) SubmitOrder(_ context.Context, input SubmitOrderInput) (Result, error) {
	normalized, requestHash, err := validateAndHash(input)
	if err != nil {
		return Result{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if processed, ok := s.processed[normalized.AvitoOrderID]; ok {
		if processed.requestHash != requestHash {
			return Result{}, ErrAvitoOrderIDReused
		}
		return processed.result, nil
	}

	result, err := s.process(normalized)
	if err != nil {
		return Result{}, err
	}

	s.processed[normalized.AvitoOrderID] = processedOrder{
		requestHash: requestHash,
		result:      result,
	}
	return result, nil
}

func (s *Service) process(input SubmitOrderInput) (Result, error) {
	if input.RestaurantExternalID != s.state.RestaurantExternalID {
		return Result{}, invalidOrder("restaurant_external_id is unknown")
	}
	if input.MenuSnapshotVersion > s.state.MenuSnapshotVersion {
		return Result{}, invalidOrder("menu_snapshot_version is newer than Cafe Service state")
	}
	if input.MenuSnapshotVersion < s.state.MenuSnapshotVersion {
		currentVersion := s.state.MenuSnapshotVersion
		return Result{
			Status:                 StatusRejected,
			RejectionReason:        RejectionReasonStaleMenuSnapshot,
			CurrentSnapshotVersion: &currentVersion,
		}, nil
	}
	if !s.state.IsAcceptingOrders {
		return Result{
			Status:          StatusRejected,
			RejectionReason: RejectionReasonNotAcceptingOrders,
		}, nil
	}

	for _, requested := range input.Items {
		item, ok := s.state.Items[requested.ExternalItemID]
		if !ok || !item.IsAvailable {
			return Result{
				Status:          StatusRejected,
				RejectionReason: RejectionReasonItemUnavailable,
			}, nil
		}
		if item.AvailableQuantity < requested.Quantity {
			return Result{
				Status:          StatusRejected,
				RejectionReason: RejectionReasonOutOfStock,
			}, nil
		}
	}

	for _, requested := range input.Items {
		item := s.state.Items[requested.ExternalItemID]
		item.AvailableQuantity -= requested.Quantity
		s.state.Items[requested.ExternalItemID] = item
	}

	return Result{
		Status:          StatusAccepted,
		ExternalOrderID: "cafe-" + input.AvitoOrderID,
	}, nil
}

func validateAndHash(input SubmitOrderInput) (SubmitOrderInput, string, error) {
	input.AvitoOrderID = strings.TrimSpace(input.AvitoOrderID)
	if !isUUID(input.AvitoOrderID) {
		return SubmitOrderInput{}, "", invalidOrder("avito_order_id must be a valid UUID")
	}

	input.RestaurantExternalID = strings.TrimSpace(input.RestaurantExternalID)
	if input.RestaurantExternalID == "" {
		return SubmitOrderInput{}, "", invalidOrder("restaurant_external_id is required")
	}
	if input.MenuSnapshotVersion < 1 {
		return SubmitOrderInput{}, "", invalidOrder("menu_snapshot_version must be positive")
	}
	if len(input.Items) == 0 {
		return SubmitOrderInput{}, "", invalidOrder("items must contain at least one item")
	}

	seen := make(map[string]struct{}, len(input.Items))
	items := make([]SubmitOrderItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		externalID := strings.TrimSpace(item.ExternalItemID)
		if externalID == "" {
			return SubmitOrderInput{}, "", invalidOrder("external_item_id is required")
		}
		if _, exists := seen[externalID]; exists {
			return SubmitOrderInput{}, "", invalidOrder("items contains duplicate external_item_id")
		}
		seen[externalID] = struct{}{}
		if item.Quantity < 1 || item.Quantity > math.MaxInt32 {
			return SubmitOrderInput{}, "", invalidOrder("quantity must be between 1 and 2147483647")
		}
		items = append(items, SubmitOrderItemInput{
			ExternalItemID: externalID,
			Quantity:       item.Quantity,
		})
	}
	input.Items = items

	hash, err := hashRequest(input)
	if err != nil {
		return SubmitOrderInput{}, "", fmt.Errorf("hash order request: %w", err)
	}
	return input, hash, nil
}

func hashRequest(input SubmitOrderInput) (string, error) {
	canonical := input
	canonical.Items = append([]SubmitOrderItemInput(nil), input.Items...)
	sort.Slice(canonical.Items, func(i, j int) bool {
		return canonical.Items[i].ExternalItemID < canonical.Items[j].ExternalItemID
	})

	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal canonical request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func invalidOrder(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidOrder, message)
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}
