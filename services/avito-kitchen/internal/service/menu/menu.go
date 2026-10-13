package menu

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidMenu         = errors.New("invalid menu")
	ErrRestaurantNotFound  = errors.New("restaurant not found")
	ErrMenuItemNotFound    = errors.New("menu item not found")
	ErrStaleSnapshotUpdate = errors.New("stale menu snapshot update")
)

type StaleSnapshotError struct {
	CurrentVersion  int64
	ReceivedVersion int64
}

func (e *StaleSnapshotError) Error() string {
	return fmt.Sprintf(
		"%v: current version %d, received version %d",
		ErrStaleSnapshotUpdate,
		e.CurrentVersion,
		e.ReceivedVersion,
	)
}

func (e *StaleSnapshotError) Unwrap() error {
	return ErrStaleSnapshotUpdate
}

func (s *Service) GetMenu(ctx context.Context, restaurantID string) (Menu, error) {
	restaurantID = strings.TrimSpace(restaurantID)
	if !isUUID(restaurantID) {
		return Menu{}, invalidMenu("restaurant_id must be a valid UUID")
	}

	result, err := s.repository.GetMenu(ctx, restaurantID)
	if err != nil {
		return Menu{}, fmt.Errorf("get menu: %w", err)
	}
	return result, nil
}

func (s *Service) ReplaceSnapshot(
	ctx context.Context,
	restaurantID string,
	input ReplaceSnapshotInput,
) (SnapshotResult, error) {
	restaurantID = strings.TrimSpace(restaurantID)
	if !isUUID(restaurantID) {
		return SnapshotResult{}, invalidMenu("restaurant_id must be a valid UUID")
	}

	snapshot, err := validateSnapshot(input)
	if err != nil {
		return SnapshotResult{}, err
	}

	result, err := s.repository.ReplaceSnapshot(ctx, restaurantID, snapshot)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("replace menu snapshot: %w", err)
	}
	return result, nil
}

func (s *Service) UpdateAvailability(
	ctx context.Context,
	restaurantID string,
	externalItemID string,
	availableQuantity int,
) (AvailabilityResult, error) {
	restaurantID = strings.TrimSpace(restaurantID)
	if !isUUID(restaurantID) {
		return AvailabilityResult{}, invalidMenu("restaurant_id must be a valid UUID")
	}

	externalItemID = strings.TrimSpace(externalItemID)
	if externalItemID == "" {
		return AvailabilityResult{}, invalidMenu("external_item_id is required")
	}
	if availableQuantity < 0 {
		return AvailabilityResult{}, invalidMenu("available_quantity must not be negative")
	}

	result, err := s.repository.UpdateAvailability(
		ctx,
		restaurantID,
		externalItemID,
		availableQuantity,
	)
	if err != nil {
		return AvailabilityResult{}, fmt.Errorf("update menu item availability: %w", err)
	}
	return result, nil
}

func validateSnapshot(input ReplaceSnapshotInput) (Snapshot, error) {
	if input.SnapshotVersion < 1 {
		return Snapshot{}, invalidMenu("snapshot_version must be positive")
	}

	items := make([]SnapshotItem, 0, len(input.Items))
	externalIDs := make(map[string]struct{}, len(input.Items))
	for _, item := range input.Items {
		externalID := strings.TrimSpace(item.ExternalID)
		if externalID == "" {
			return Snapshot{}, invalidMenu("menu item external_id is required")
		}
		if _, exists := externalIDs[externalID]; exists {
			return Snapshot{}, invalidMenu("menu snapshot contains duplicate external_id")
		}
		externalIDs[externalID] = struct{}{}

		name := strings.TrimSpace(item.Name)
		if name == "" {
			return Snapshot{}, invalidMenu("menu item name is required")
		}
		if item.Price < 0 {
			return Snapshot{}, invalidMenu("menu item price must not be negative")
		}
		if item.AvailableQuantity < 0 {
			return Snapshot{}, invalidMenu("available_quantity must not be negative")
		}

		var description *string
		if item.Description != nil {
			trimmed := strings.TrimSpace(*item.Description)
			description = &trimmed
		}

		items = append(items, SnapshotItem{
			ExternalID:        externalID,
			Name:              name,
			Description:       description,
			Price:             item.Price,
			AvailableQuantity: item.AvailableQuantity,
		})
	}

	return Snapshot{
		Version: input.SnapshotVersion,
		Items:   items,
	}, nil
}

func invalidMenu(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidMenu, message)
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
