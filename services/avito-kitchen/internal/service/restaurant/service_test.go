package restaurant

import (
	"context"
	"errors"
	"testing"
	"time"
)

type repositoryStub struct {
	restaurants []Restaurant
	err         error
	create      func(Restaurant) (Restaurant, error)
	update      func(string, RestaurantUpdate) (Restaurant, error)
}

func (s repositoryStub) ListRestaurants(context.Context) ([]Restaurant, error) {
	return s.restaurants, s.err
}

func (s repositoryStub) CreateRestaurant(_ context.Context, restaurant Restaurant) (Restaurant, error) {
	if s.create != nil {
		return s.create(restaurant)
	}
	return restaurant, nil
}

func (s repositoryStub) UpdateRestaurant(
	_ context.Context,
	restaurantID string,
	update RestaurantUpdate,
) (Restaurant, error) {
	if s.update != nil {
		return s.update(restaurantID, update)
	}
	return Restaurant{ID: restaurantID}, nil
}

func TestListRestaurantsReturnsOnlyAvailableRestaurants(t *testing.T) {
	now := time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC)
	service := NewService(repositoryStub{restaurants: []Restaurant{
		{
			ID:                "available",
			Name:              "Available Cafe",
			Timezone:          "UTC",
			IsAcceptingOrders: true,
			WorkingHours: []WorkingHours{
				{DayOfWeek: 1, Mode: WorkingHoursOpen24H},
			},
		},
		{
			ID:                "closed",
			Name:              "Closed Cafe",
			Timezone:          "UTC",
			IsAcceptingOrders: true,
			WorkingHours: []WorkingHours{
				{DayOfWeek: 1, Mode: WorkingHoursClosed},
			},
		},
		{
			ID:                "not-accepting",
			Name:              "Paused Cafe",
			Timezone:          "UTC",
			IsAcceptingOrders: false,
			WorkingHours: []WorkingHours{
				{DayOfWeek: 1, Mode: WorkingHoursOpen24H},
			},
		},
	}})
	service.now = func() time.Time { return now }

	restaurants, err := service.ListRestaurants(context.Background())
	if err != nil {
		t.Fatalf("ListRestaurants() error = %v", err)
	}

	if len(restaurants) != 1 {
		t.Fatalf("restaurant count = %d, want 1", len(restaurants))
	}
	if restaurants[0] != (RestaurantSummary{
		ID:          "available",
		Name:        "Available Cafe",
		IsAvailable: true,
	}) {
		t.Errorf("restaurant = %#v", restaurants[0])
	}
}

func TestRestaurantAvailability(t *testing.T) {
	mondayAtTenUTC := time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		restaurant Restaurant
		now        time.Time
		want       bool
		wantError  bool
	}{
		{
			name: "open at interval start",
			restaurant: availableRestaurant(WorkingHours{
				DayOfWeek: 1,
				Mode:      WorkingHoursInterval,
				OpensAt:   10 * time.Hour,
				ClosesAt:  18 * time.Hour,
			}),
			now:  mondayAtTenUTC,
			want: true,
		},
		{
			name: "closed at interval end",
			restaurant: availableRestaurant(WorkingHours{
				DayOfWeek: 1,
				Mode:      WorkingHoursInterval,
				OpensAt:   9 * time.Hour,
				ClosesAt:  10 * time.Hour,
			}),
			now: mondayAtTenUTC,
		},
		{
			name: "uses restaurant local weekday",
			restaurant: Restaurant{
				ID:                "restaurant-id",
				Timezone:          "America/New_York",
				IsAcceptingOrders: true,
				WorkingHours: []WorkingHours{
					{DayOfWeek: 7, Mode: WorkingHoursOpen24H},
				},
			},
			now:  time.Date(2026, time.August, 31, 0, 30, 0, 0, time.UTC),
			want: true,
		},
		{
			name:       "missing current weekday is unavailable",
			restaurant: availableRestaurant(WorkingHours{DayOfWeek: 2, Mode: WorkingHoursOpen24H}),
			now:        mondayAtTenUTC,
		},
		{
			name: "invalid timezone is an error",
			restaurant: Restaurant{
				ID:                "restaurant-id",
				Timezone:          "invalid/timezone",
				IsAcceptingOrders: true,
			},
			now:       mondayAtTenUTC,
			wantError: true,
		},
		{
			name:       "unknown schedule mode is an error",
			restaurant: availableRestaurant(WorkingHours{DayOfWeek: 1, Mode: "UNKNOWN"}),
			now:        mondayAtTenUTC,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isRestaurantAvailable(tt.restaurant, tt.now)
			if tt.wantError {
				if err == nil {
					t.Fatal("error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got != tt.want {
				t.Errorf("available = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestListRestaurantsWrapsRepositoryError(t *testing.T) {
	repositoryError := errors.New("database unavailable")
	service := NewService(repositoryStub{err: repositoryError})

	_, err := service.ListRestaurants(context.Background())
	if !errors.Is(err, repositoryError) {
		t.Fatalf("error = %v, want wrapped repository error", err)
	}
}

func availableRestaurant(hours WorkingHours) Restaurant {
	return Restaurant{
		ID:                "restaurant-id",
		Timezone:          "UTC",
		IsAcceptingOrders: true,
		WorkingHours:      []WorkingHours{hours},
	}
}
