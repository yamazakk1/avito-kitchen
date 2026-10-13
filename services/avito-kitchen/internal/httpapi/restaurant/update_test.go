package restaurant

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

func TestUpdateRestaurant(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 28, 18, 0, 0, 0, time.UTC)
	createdAt := updatedAt.Add(-time.Hour)
	service := restaurantServiceStub{update: func(
		_ context.Context,
		restaurantID string,
		input restaurantservice.UpdateRestaurantInput,
	) (restaurantservice.Restaurant, error) {
		if restaurantID != "00000000-0000-4000-8000-000000000001" {
			t.Fatalf("restaurant ID = %q", restaurantID)
		}
		if input.Name == nil || *input.Name != "Updated Cafe" {
			t.Errorf("name = %#v", input.Name)
		}
		if input.IsAcceptingOrders == nil || *input.IsAcceptingOrders {
			t.Errorf("is_accepting_orders = %#v", input.IsAcceptingOrders)
		}
		if input.WorkingHours == nil || len(*input.WorkingHours) != 2 {
			t.Fatalf("working hours = %#v", input.WorkingHours)
		}
		if (*input.WorkingHours)[0].OpensAt == nil || *(*input.WorkingHours)[0].OpensAt != 10*time.Hour {
			t.Errorf("opens_at = %#v", (*input.WorkingHours)[0].OpensAt)
		}

		return restaurantservice.Restaurant{
			ID:                restaurantID,
			ExternalID:        "cafe-1",
			Name:              "Updated Cafe",
			OrderEndpoint:     "http://demo-cafe:8081/api/v1/orders",
			Timezone:          "Europe/Moscow",
			IsAcceptingOrders: false,
			WorkingHours: []restaurantservice.WorkingHours{
				{DayOfWeek: 1, Mode: restaurantservice.WorkingHoursInterval, OpensAt: 10 * time.Hour, ClosesAt: 20 * time.Hour},
				{DayOfWeek: 7, Mode: restaurantservice.WorkingHoursClosed},
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}, nil
	}}

	h := newRestaurantsTestHandler(service)
	body := `{
		"name":"Updated Cafe",
		"is_accepting_orders":false,
		"working_hours":[
			{"day_of_week":1,"mode":"INTERVAL","opens_at":"10:00","closes_at":"20:00"},
			{"day_of_week":7,"mode":"CLOSED"}
		]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/partner/restaurants/00000000-0000-4000-8000-000000000001",
		strings.NewReader(body),
	)

	h.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	wantBody := `{"id":"00000000-0000-4000-8000-000000000001","external_id":"cafe-1","name":"Updated Cafe","order_endpoint":"http://demo-cafe:8081/api/v1/orders","timezone":"Europe/Moscow","is_accepting_orders":false,"working_hours":[{"day_of_week":1,"mode":"INTERVAL","opens_at":"10:00","closes_at":"20:00"},{"day_of_week":7,"mode":"CLOSED","opens_at":null,"closes_at":null}],"created_at":"2026-08-28T17:00:00Z","updated_at":"2026-08-28T18:00:00Z"}`
	if strings.TrimSpace(recorder.Body.String()) != wantBody {
		t.Errorf("body = %s, want %s", recorder.Body.String(), wantBody)
	}
}

func TestUpdateRestaurantErrors(t *testing.T) {
	validBody := `{"name":"Updated Cafe"}`

	tests := []struct {
		name       string
		body       string
		serviceErr error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "malformed JSON",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":{"code":"INVALID_REQUEST","message":"Invalid request"}}`,
		},
		{
			name:       "unknown field",
			body:       `{"unknown":true}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":{"code":"INVALID_REQUEST","message":"Invalid request"}}`,
		},
		{
			name: "invalid local time",
			body: `{
				"working_hours":[
					{"day_of_week":1,"mode":"INTERVAL","opens_at":"9:00","closes_at":"18:00"}
				]
			}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":{"code":"INVALID_REQUEST","message":"Invalid request"}}`,
		},
		{
			name:       "service validation",
			body:       validBody,
			serviceErr: restaurantservice.ErrInvalidRestaurant,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":{"code":"INVALID_REQUEST","message":"Invalid request"}}`,
		},
		{
			name:       "restaurant not found",
			body:       validBody,
			serviceErr: restaurantservice.ErrRestaurantNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":{"code":"RESTAURANT_NOT_FOUND","message":"Restaurant not found"}}`,
		},
		{
			name:       "technical failure",
			body:       validBody,
			serviceErr: errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := restaurantServiceStub{update: func(
				context.Context,
				string,
				restaurantservice.UpdateRestaurantInput,
			) (restaurantservice.Restaurant, error) {
				return restaurantservice.Restaurant{}, tt.serviceErr
			}}
			h := newRestaurantsTestHandler(service)
			recorder := httptest.NewRecorder()

			h.ServeHTTP(
				recorder,
				httptest.NewRequest(
					http.MethodPatch,
					"/api/v1/partner/restaurants/00000000-0000-4000-8000-000000000001",
					strings.NewReader(tt.body),
				),
			)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if strings.TrimSpace(recorder.Body.String()) != tt.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}
