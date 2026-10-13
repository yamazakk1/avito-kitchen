package menu

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

type menuServiceStub struct {
	get                func(context.Context, string) (menuservice.Menu, error)
	replace            func(context.Context, string, menuservice.ReplaceSnapshotInput) (menuservice.SnapshotResult, error)
	updateAvailability func(context.Context, string, string, int) (menuservice.AvailabilityResult, error)
}

func (s menuServiceStub) GetMenu(ctx context.Context, restaurantID string) (menuservice.Menu, error) {
	if s.get == nil {
		return menuservice.Menu{}, nil
	}
	return s.get(ctx, restaurantID)
}

func (s menuServiceStub) ReplaceSnapshot(
	ctx context.Context,
	restaurantID string,
	input menuservice.ReplaceSnapshotInput,
) (menuservice.SnapshotResult, error) {
	if s.replace == nil {
		return menuservice.SnapshotResult{}, nil
	}
	return s.replace(ctx, restaurantID, input)
}

func (s menuServiceStub) UpdateAvailability(
	ctx context.Context,
	restaurantID string,
	externalItemID string,
	availableQuantity int,
) (menuservice.AvailabilityResult, error) {
	if s.updateAvailability == nil {
		return menuservice.AvailabilityResult{}, nil
	}
	return s.updateAvailability(ctx, restaurantID, externalItemID, availableQuantity)
}

func TestGetMenu(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	description := "Tomato and cheese"

	tests := []struct {
		name       string
		service    Service
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name: "success",
			service: menuServiceStub{get: func(context.Context, string) (menuservice.Menu, error) {
				return menuservice.Menu{
					RestaurantID:    "00000000-0000-4000-8000-000000000001",
					SnapshotVersion: 2,
					UpdatedAt:       updatedAt,
					Items: []menuservice.Item{{
						ID:                "10000000-0000-4000-8000-000000000001",
						Name:              "Margherita",
						Description:       &description,
						Price:             75000,
						AvailableQuantity: 3,
					}},
				}, nil
			}},
			path:       "/api/v1/restaurants/00000000-0000-4000-8000-000000000001/menu",
			wantStatus: http.StatusOK,
			wantBody:   `{"restaurant_id":"00000000-0000-4000-8000-000000000001","snapshot_version":2,"updated_at":"2026-08-28T12:00:00Z","items":[{"id":"10000000-0000-4000-8000-000000000001","name":"Margherita","description":"Tomato and cheese","price":75000,"available_quantity":3}]}`,
		},
		{
			name: "invalid restaurant ID",
			service: menuServiceStub{get: func(context.Context, string) (menuservice.Menu, error) {
				return menuservice.Menu{}, menuservice.ErrInvalidMenu
			}},
			path:       "/api/v1/restaurants/bad/menu",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":{"code":"INVALID_REQUEST","message":"Invalid request"}}`,
		},
		{
			name: "restaurant not found",
			service: menuServiceStub{get: func(context.Context, string) (menuservice.Menu, error) {
				return menuservice.Menu{}, menuservice.ErrRestaurantNotFound
			}},
			path:       "/api/v1/restaurants/00000000-0000-4000-8000-000000000001/menu",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":{"code":"RESTAURANT_NOT_FOUND","message":"Restaurant not found"}}`,
		},
		{
			name: "service failure",
			service: menuServiceStub{get: func(context.Context, string) (menuservice.Menu, error) {
				return menuservice.Menu{}, errors.New("db down")
			}},
			path:       "/api/v1/restaurants/00000000-0000-4000-8000-000000000001/menu",
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newMenuTestHandler(tt.service)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if strings.TrimSpace(recorder.Body.String()) != tt.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestReplaceMenuSnapshot(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 28, 13, 0, 0, 0, time.UTC)
	path := "/api/v1/partner/restaurants/00000000-0000-4000-8000-000000000001/menu"

	t.Run("success", func(t *testing.T) {
		service := menuServiceStub{replace: func(
			_ context.Context,
			restaurantID string,
			input menuservice.ReplaceSnapshotInput,
		) (menuservice.SnapshotResult, error) {
			if restaurantID != "00000000-0000-4000-8000-000000000001" {
				t.Fatalf("restaurant ID = %q", restaurantID)
			}
			if input.SnapshotVersion != 4 || len(input.Items) != 1 || input.Items[0].ExternalID != "pizza-1" {
				t.Fatalf("input = %#v", input)
			}
			return menuservice.SnapshotResult{SnapshotVersion: 4, UpdatedAt: updatedAt, ItemsCount: 1}, nil
		}}
		h := newMenuTestHandler(service)
		recorder := httptest.NewRecorder()
		body := `{"snapshot_version":4,"items":[{"external_id":"pizza-1","name":"Margherita","price":75000,"available_quantity":3}]}`
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		want := `{"snapshot_version":4,"updated_at":"2026-08-28T13:00:00Z","items_count":1}`
		if strings.TrimSpace(recorder.Body.String()) != want {
			t.Errorf("body = %q, want %q", recorder.Body.String(), want)
		}
	})

	t.Run("stale snapshot", func(t *testing.T) {
		service := menuServiceStub{replace: func(context.Context, string, menuservice.ReplaceSnapshotInput) (menuservice.SnapshotResult, error) {
			return menuservice.SnapshotResult{}, &menuservice.StaleSnapshotError{CurrentVersion: 5, ReceivedVersion: 4}
		}}
		h := newMenuTestHandler(service)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"snapshot_version":4,"items":[]}`)))
		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d", recorder.Code)
		}
		want := `{"error":{"code":"STALE_SNAPSHOT_UPDATE","message":"Menu snapshot version is older than current version","details":{"current_snapshot_version":5,"received_snapshot_version":4}}}`
		if strings.TrimSpace(recorder.Body.String()) != want {
			t.Errorf("body = %q, want %q", recorder.Body.String(), want)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		h := newMenuTestHandler(menuServiceStub{})
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"snapshot_version":`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", recorder.Code)
		}
	})
}

func TestUpdateMenuItemAvailability(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 28, 14, 0, 0, 0, time.UTC)
	path := "/api/v1/partner/restaurants/00000000-0000-4000-8000-000000000001/menu-items/pizza-1/availability"

	t.Run("success", func(t *testing.T) {
		service := menuServiceStub{updateAvailability: func(
			_ context.Context,
			restaurantID string,
			externalItemID string,
			quantity int,
		) (menuservice.AvailabilityResult, error) {
			if restaurantID == "" || externalItemID != "pizza-1" || quantity != 7 {
				t.Fatalf("args = %q, %q, %d", restaurantID, externalItemID, quantity)
			}
			return menuservice.AvailabilityResult{ExternalItemID: externalItemID, AvailableQuantity: 7, UpdatedAt: updatedAt}, nil
		}}
		h := newMenuTestHandler(service)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"available_quantity":7}`)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		want := `{"external_item_id":"pizza-1","available_quantity":7,"updated_at":"2026-08-28T14:00:00Z"}`
		if strings.TrimSpace(recorder.Body.String()) != want {
			t.Errorf("body = %q, want %q", recorder.Body.String(), want)
		}
	})

	t.Run("item not found", func(t *testing.T) {
		service := menuServiceStub{updateAvailability: func(context.Context, string, string, int) (menuservice.AvailabilityResult, error) {
			return menuservice.AvailabilityResult{}, menuservice.ErrMenuItemNotFound
		}}
		h := newMenuTestHandler(service)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"available_quantity":7}`)))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d", recorder.Code)
		}
		want := `{"error":{"code":"ITEM_NOT_FOUND","message":"Menu item not found"}}`
		if strings.TrimSpace(recorder.Body.String()) != want {
			t.Errorf("body = %q, want %q", recorder.Body.String(), want)
		}
	})
}

func newMenuTestHandler(service Service) http.Handler {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), service)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}
