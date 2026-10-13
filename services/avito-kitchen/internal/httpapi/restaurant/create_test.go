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

func TestCreateRestaurant(t *testing.T) {
	createdAt := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	service := restaurantServiceStub{create: func(
		_ context.Context,
		input restaurantservice.CreateRestaurantInput,
	) (restaurantservice.Restaurant, error) {
		if !input.IsAcceptingOrders {
			t.Error("is_accepting_orders default = false, want true")
		}
		if len(input.WorkingHours) != 2 {
			t.Fatalf("working hours count = %d, want 2", len(input.WorkingHours))
		}
		if input.WorkingHours[0].OpensAt == nil || *input.WorkingHours[0].OpensAt != 9*time.Hour {
			t.Errorf("opens_at = %#v", input.WorkingHours[0].OpensAt)
		}

		return restaurantservice.Restaurant{
			ID:                "00000000-0000-4000-8000-000000000001",
			ExternalID:        input.ExternalID,
			Name:              input.Name,
			OrderEndpoint:     input.OrderEndpoint,
			Timezone:          input.Timezone,
			IsAcceptingOrders: input.IsAcceptingOrders,
			WorkingHours: []restaurantservice.WorkingHours{
				{DayOfWeek: 1, Mode: restaurantservice.WorkingHoursInterval, OpensAt: 9 * time.Hour, ClosesAt: 18 * time.Hour},
				{DayOfWeek: 2, Mode: restaurantservice.WorkingHoursOpen24H},
			},
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}, nil
	}}
	h := newRestaurantsTestHandler(service)
	body := `{
		"external_id":"cafe-1",
		"name":"Demo Cafe",
		"order_endpoint":"http://demo-cafe:8081/api/v1/orders",
		"timezone":"Europe/Moscow",
		"working_hours":[
			{"day_of_week":1,"mode":"INTERVAL","opens_at":"09:00","closes_at":"18:00"},
			{"day_of_week":2,"mode":"OPEN_24H"}
		]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/partner/restaurants", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	h.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	wantBody := `{"id":"00000000-0000-4000-8000-000000000001","external_id":"cafe-1","name":"Demo Cafe","order_endpoint":"http://demo-cafe:8081/api/v1/orders","timezone":"Europe/Moscow","is_accepting_orders":true,"working_hours":[{"day_of_week":1,"mode":"INTERVAL","opens_at":"09:00","closes_at":"18:00"},{"day_of_week":2,"mode":"OPEN_24H","opens_at":null,"closes_at":null}],"created_at":"2026-08-28T12:00:00Z","updated_at":"2026-08-28T12:00:00Z"}`
	if strings.TrimSpace(recorder.Body.String()) != wantBody {
		t.Errorf("body = %s, want %s", recorder.Body.String(), wantBody)
	}
}

func TestCreateRestaurantErrors(t *testing.T) {
	validBody := `{
		"external_id":"cafe-1",
		"name":"Demo Cafe",
		"order_endpoint":"http://demo-cafe:8081/api/v1/orders",
		"timezone":"Europe/Moscow",
		"working_hours":[{"day_of_week":1,"mode":"OPEN_24H"}]
	}`

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
				"external_id":"cafe-1",
				"name":"Demo Cafe",
				"order_endpoint":"http://demo-cafe:8081/api/v1/orders",
				"timezone":"Europe/Moscow",
				"working_hours":[{"day_of_week":1,"mode":"INTERVAL","opens_at":"9:00","closes_at":"18:00"}]
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
			name:       "duplicate external ID",
			body:       validBody,
			serviceErr: restaurantservice.ErrRestaurantAlreadyExists,
			wantStatus: http.StatusConflict,
			wantBody:   `{"error":{"code":"RESTAURANT_ALREADY_EXISTS","message":"Restaurant already exists"}}`,
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
			service := restaurantServiceStub{create: func(
				context.Context,
				restaurantservice.CreateRestaurantInput,
			) (restaurantservice.Restaurant, error) {
				return restaurantservice.Restaurant{}, tt.serviceErr
			}}
			h := newRestaurantsTestHandler(service)
			recorder := httptest.NewRecorder()

			h.ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodPost, "/api/v1/partner/restaurants", strings.NewReader(tt.body)),
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
