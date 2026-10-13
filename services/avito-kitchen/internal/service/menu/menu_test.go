package menu

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type repositoryStub struct {
	get                func(context.Context, string) (Menu, error)
	replace            func(context.Context, string, Snapshot) (SnapshotResult, error)
	updateAvailability func(context.Context, string, string, int) (AvailabilityResult, error)
}

func (s repositoryStub) GetMenu(ctx context.Context, restaurantID string) (Menu, error) {
	if s.get == nil {
		return Menu{}, nil
	}
	return s.get(ctx, restaurantID)
}

func (s repositoryStub) ReplaceSnapshot(
	ctx context.Context,
	restaurantID string,
	snapshot Snapshot,
) (SnapshotResult, error) {
	if s.replace == nil {
		return SnapshotResult{}, nil
	}
	return s.replace(ctx, restaurantID, snapshot)
}

func (s repositoryStub) UpdateAvailability(
	ctx context.Context,
	restaurantID string,
	externalItemID string,
	availableQuantity int,
) (AvailabilityResult, error) {
	if s.updateAvailability == nil {
		return AvailabilityResult{}, nil
	}
	return s.updateAvailability(ctx, restaurantID, externalItemID, availableQuantity)
}

func TestServiceGetMenu(t *testing.T) {
	const restaurantID = "00000000-0000-4000-8000-000000000001"

	t.Run("success", func(t *testing.T) {
		want := Menu{RestaurantID: restaurantID, SnapshotVersion: 3, Items: []Item{}}
		service := NewService(repositoryStub{get: func(_ context.Context, gotID string) (Menu, error) {
			if gotID != restaurantID {
				t.Fatalf("restaurant ID = %q, want %q", gotID, restaurantID)
			}
			return want, nil
		}})

		got, err := service.GetMenu(context.Background(), restaurantID)
		if err != nil {
			t.Fatalf("GetMenu() error = %v", err)
		}
		if got.RestaurantID != want.RestaurantID || got.SnapshotVersion != want.SnapshotVersion {
			t.Errorf("GetMenu() = %#v, want %#v", got, want)
		}
	})

	t.Run("invalid restaurant ID", func(t *testing.T) {
		service := NewService(repositoryStub{})
		_, err := service.GetMenu(context.Background(), "not-a-uuid")
		if !errors.Is(err, ErrInvalidMenu) {
			t.Fatalf("GetMenu() error = %v, want ErrInvalidMenu", err)
		}
	})
}

func TestServiceReplaceSnapshot(t *testing.T) {
	const restaurantID = "00000000-0000-4000-8000-000000000001"

	t.Run("validates and normalizes snapshot", func(t *testing.T) {
		description := "  tomato and cheese  "
		service := NewService(repositoryStub{replace: func(
			_ context.Context,
			gotRestaurantID string,
			snapshot Snapshot,
		) (SnapshotResult, error) {
			if gotRestaurantID != restaurantID {
				t.Fatalf("restaurant ID = %q", gotRestaurantID)
			}
			if snapshot.Version != 2 || len(snapshot.Items) != 1 {
				t.Fatalf("snapshot = %#v", snapshot)
			}
			item := snapshot.Items[0]
			if item.ExternalID != "pizza-1" || item.Name != "Margherita" {
				t.Errorf("normalized item = %#v", item)
			}
			if item.Description == nil || *item.Description != "tomato and cheese" {
				t.Errorf("description = %#v", item.Description)
			}
			return SnapshotResult{SnapshotVersion: 2, ItemsCount: 1}, nil
		}})

		result, err := service.ReplaceSnapshot(context.Background(), restaurantID, ReplaceSnapshotInput{
			SnapshotVersion: 2,
			Items: []SnapshotItemInput{{
				ExternalID:        " pizza-1 ",
				Name:              " Margherita ",
				Description:       &description,
				Price:             75000,
				AvailableQuantity: 5,
			}},
		})
		if err != nil {
			t.Fatalf("ReplaceSnapshot() error = %v", err)
		}
		if result.SnapshotVersion != 2 || result.ItemsCount != 1 {
			t.Errorf("result = %#v", result)
		}
	})

	tests := []struct {
		name  string
		input ReplaceSnapshotInput
	}{
		{name: "non-positive version", input: ReplaceSnapshotInput{SnapshotVersion: 0}},
		{name: "empty external ID", input: ReplaceSnapshotInput{SnapshotVersion: 1, Items: []SnapshotItemInput{{Name: "Pizza"}}}},
		{name: "empty name", input: ReplaceSnapshotInput{SnapshotVersion: 1, Items: []SnapshotItemInput{{ExternalID: "pizza"}}}},
		{name: "negative price", input: ReplaceSnapshotInput{SnapshotVersion: 1, Items: []SnapshotItemInput{{ExternalID: "pizza", Name: "Pizza", Price: -1}}}},
		{name: "negative quantity", input: ReplaceSnapshotInput{SnapshotVersion: 1, Items: []SnapshotItemInput{{ExternalID: "pizza", Name: "Pizza", AvailableQuantity: -1}}}},
		{
			name: "duplicate external ID",
			input: ReplaceSnapshotInput{SnapshotVersion: 1, Items: []SnapshotItemInput{
				{ExternalID: "pizza", Name: "One"},
				{ExternalID: " pizza ", Name: "Two"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(repositoryStub{})
			_, err := service.ReplaceSnapshot(context.Background(), restaurantID, tt.input)
			if !errors.Is(err, ErrInvalidMenu) {
				t.Fatalf("ReplaceSnapshot() error = %v, want ErrInvalidMenu", err)
			}
		})
	}

	t.Run("empty items is valid and clears menu", func(t *testing.T) {
		called := false
		service := NewService(repositoryStub{replace: func(_ context.Context, _ string, snapshot Snapshot) (SnapshotResult, error) {
			called = true
			if len(snapshot.Items) != 0 {
				t.Fatalf("items = %#v, want empty", snapshot.Items)
			}
			return SnapshotResult{}, nil
		}})
		_, err := service.ReplaceSnapshot(context.Background(), restaurantID, ReplaceSnapshotInput{SnapshotVersion: 1})
		if err != nil {
			t.Fatalf("ReplaceSnapshot() error = %v", err)
		}
		if !called {
			t.Fatal("repository was not called")
		}
	})
}

func TestServiceUpdateAvailability(t *testing.T) {
	const restaurantID = "00000000-0000-4000-8000-000000000001"
	now := time.Now()

	service := NewService(repositoryStub{updateAvailability: func(
		_ context.Context,
		gotRestaurantID string,
		externalItemID string,
		quantity int,
	) (AvailabilityResult, error) {
		if gotRestaurantID != restaurantID || externalItemID != "pizza-1" || quantity != 4 {
			t.Fatalf("arguments = %q, %q, %d", gotRestaurantID, externalItemID, quantity)
		}
		return AvailabilityResult{
			ExternalItemID:    externalItemID,
			AvailableQuantity: quantity,
			UpdatedAt:         now,
		}, nil
	}})

	result, err := service.UpdateAvailability(context.Background(), restaurantID, " pizza-1 ", 4)
	if err != nil {
		t.Fatalf("UpdateAvailability() error = %v", err)
	}
	if result.AvailableQuantity != 4 || result.ExternalItemID != "pizza-1" {
		t.Errorf("result = %#v", result)
	}

	for _, tt := range []struct {
		name       string
		restaurant string
		item       string
		quantity   int
	}{
		{name: "invalid restaurant", restaurant: "bad", item: "pizza", quantity: 1},
		{name: "empty item ID", restaurant: restaurantID, item: "   ", quantity: 1},
		{name: "negative quantity", restaurant: restaurantID, item: "pizza", quantity: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(repositoryStub{})
			_, err := service.UpdateAvailability(context.Background(), tt.restaurant, tt.item, tt.quantity)
			if !errors.Is(err, ErrInvalidMenu) {
				t.Fatalf("UpdateAvailability() error = %v, want ErrInvalidMenu", err)
			}
		})
	}
}

func TestStaleSnapshotErrorSupportsErrorsIsAndAs(t *testing.T) {
	err := fmtWrappedStaleError()
	if !errors.Is(err, ErrStaleSnapshotUpdate) {
		t.Fatalf("errors.Is() = false")
	}
	var stale *StaleSnapshotError
	if !errors.As(err, &stale) {
		t.Fatalf("errors.As() = false")
	}
	if stale.CurrentVersion != 5 || stale.ReceivedVersion != 4 {
		t.Errorf("stale = %#v", stale)
	}
}

func fmtWrappedStaleError() error {
	return fmt.Errorf("wrapped: %w", &StaleSnapshotError{CurrentVersion: 5, ReceivedVersion: 4})
}
