package restaurant

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

type restaurantServiceStub struct {
	list   func(context.Context) ([]restaurantservice.RestaurantSummary, error)
	create func(context.Context, restaurantservice.CreateRestaurantInput) (restaurantservice.Restaurant, error)
	update func(context.Context, string, restaurantservice.UpdateRestaurantInput) (restaurantservice.Restaurant, error)
}

func (s restaurantServiceStub) ListRestaurants(ctx context.Context) ([]restaurantservice.RestaurantSummary, error) {
	if s.list == nil {
		return []restaurantservice.RestaurantSummary{}, nil
	}
	return s.list(ctx)
}

func (s restaurantServiceStub) CreateRestaurant(
	ctx context.Context,
	input restaurantservice.CreateRestaurantInput,
) (restaurantservice.Restaurant, error) {
	if s.create == nil {
		return restaurantservice.Restaurant{}, nil
	}
	return s.create(ctx, input)
}

func (s restaurantServiceStub) UpdateRestaurant(
	ctx context.Context,
	restaurantID string,
	input restaurantservice.UpdateRestaurantInput,
) (restaurantservice.Restaurant, error) {
	if s.update == nil {
		return restaurantservice.Restaurant{}, nil
	}
	return s.update(ctx, restaurantID, input)
}

func TestListRestaurants(t *testing.T) {
	tests := []struct {
		name       string
		service    Service
		wantStatus int
		wantBody   string
	}{
		{
			name: "success",
			service: restaurantServiceStub{list: func(context.Context) ([]restaurantservice.RestaurantSummary, error) {
				return []restaurantservice.RestaurantSummary{
					{ID: "00000000-0000-4000-8000-000000000001", Name: "Coffee Place", IsAvailable: true},
				}, nil
			}},
			wantStatus: http.StatusOK,
			wantBody:   `{"restaurants":[{"id":"00000000-0000-4000-8000-000000000001","name":"Coffee Place","is_available":true}]}`,
		},
		{
			name: "empty result is an empty JSON array",
			service: restaurantServiceStub{list: func(context.Context) ([]restaurantservice.RestaurantSummary, error) {
				return nil, nil
			}},
			wantStatus: http.StatusOK,
			wantBody:   `{"restaurants":[]}`,
		},
		{
			name: "service failure",
			service: restaurantServiceStub{list: func(context.Context) ([]restaurantservice.RestaurantSummary, error) {
				return nil, errors.New("database unavailable")
			}},
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newRestaurantsTestHandler(tt.service)
			recorder := httptest.NewRecorder()

			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/restaurants", nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if recorder.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", recorder.Header().Get("Content-Type"))
			}
			if strings.TrimSpace(recorder.Body.String()) != tt.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestListRestaurantsRejectsUnsupportedMethod(t *testing.T) {
	h := newRestaurantsTestHandler(restaurantServiceStub{list: func(context.Context) ([]restaurantservice.RestaurantSummary, error) {
		return nil, nil
	}})
	recorder := httptest.NewRecorder()

	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/restaurants", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func newRestaurantsTestHandler(service Service) http.Handler {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), service)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}
