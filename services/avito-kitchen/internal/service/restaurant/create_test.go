package restaurant

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateRestaurant(t *testing.T) {
	createdAt := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	var saved Restaurant
	repository := repositoryStub{create: func(restaurant Restaurant) (Restaurant, error) {
		saved = restaurant
		restaurant.CreatedAt = createdAt
		restaurant.UpdatedAt = createdAt
		return restaurant, nil
	}}
	service := NewService(repository)
	service.newID = func() (string, error) {
		return "00000000-0000-4000-8000-000000000001", nil
	}

	opensAt := 9 * time.Hour
	closesAt := 18 * time.Hour
	created, err := service.CreateRestaurant(context.Background(), CreateRestaurantInput{
		ExternalID:        " cafe-1 ",
		Name:              " Demo Cafe ",
		OrderEndpoint:     " http://demo-cafe:8081/api/v1/orders ",
		Timezone:          " Europe/Moscow ",
		IsAcceptingOrders: true,
		WorkingHours: []CreateWorkingHoursInput{
			{
				DayOfWeek: 1,
				Mode:      WorkingHoursInterval,
				OpensAt:   &opensAt,
				ClosesAt:  &closesAt,
			},
			{DayOfWeek: 2, Mode: WorkingHoursOpen24H},
		},
	})
	if err != nil {
		t.Fatalf("CreateRestaurant() error = %v", err)
	}

	if saved.ID != "00000000-0000-4000-8000-000000000001" {
		t.Errorf("saved ID = %q", saved.ID)
	}
	if saved.ExternalID != "cafe-1" || saved.Name != "Demo Cafe" {
		t.Errorf("saved restaurant = %#v", saved)
	}
	if saved.OrderEndpoint != "http://demo-cafe:8081/api/v1/orders" || saved.Timezone != "Europe/Moscow" {
		t.Errorf("saved restaurant endpoints = %#v", saved)
	}
	if len(saved.WorkingHours) != 2 {
		t.Fatalf("saved working hours count = %d, want 2", len(saved.WorkingHours))
	}
	if created.CreatedAt != createdAt || created.UpdatedAt != createdAt {
		t.Errorf("created timestamps = %s, %s", created.CreatedAt, created.UpdatedAt)
	}
}

func TestCreateRestaurantValidation(t *testing.T) {
	nine := 9 * time.Hour
	eighteen := 18 * time.Hour
	twentyFour := 24 * time.Hour

	tests := []struct {
		name   string
		change func(*CreateRestaurantInput)
	}{
		{name: "external ID is required", change: func(input *CreateRestaurantInput) { input.ExternalID = " " }},
		{name: "name is required", change: func(input *CreateRestaurantInput) { input.Name = "" }},
		{name: "order endpoint must be HTTP", change: func(input *CreateRestaurantInput) { input.OrderEndpoint = "ftp://cafe/orders" }},
		{name: "order endpoint must be absolute", change: func(input *CreateRestaurantInput) { input.OrderEndpoint = "/orders" }},
		{name: "timezone must exist", change: func(input *CreateRestaurantInput) { input.Timezone = "invalid/timezone" }},
		{name: "working hours are required", change: func(input *CreateRestaurantInput) { input.WorkingHours = nil }},
		{name: "at most seven working days", change: func(input *CreateRestaurantInput) {
			input.WorkingHours = make([]CreateWorkingHoursInput, 8)
		}},
		{name: "weekday is in ISO range", change: func(input *CreateRestaurantInput) { input.WorkingHours[0].DayOfWeek = 0 }},
		{name: "weekday is unique", change: func(input *CreateRestaurantInput) {
			input.WorkingHours = append(input.WorkingHours, input.WorkingHours[0])
		}},
		{name: "mode is known", change: func(input *CreateRestaurantInput) { input.WorkingHours[0].Mode = "UNKNOWN" }},
		{name: "closed day has no time", change: func(input *CreateRestaurantInput) {
			input.WorkingHours[0].Mode = WorkingHoursClosed
			input.WorkingHours[0].OpensAt = &nine
		}},
		{name: "interval requires both times", change: func(input *CreateRestaurantInput) {
			input.WorkingHours[0] = CreateWorkingHoursInput{DayOfWeek: 1, Mode: WorkingHoursInterval, OpensAt: &nine}
		}},
		{name: "interval stays within day", change: func(input *CreateRestaurantInput) {
			input.WorkingHours[0] = CreateWorkingHoursInput{
				DayOfWeek: 1,
				Mode:      WorkingHoursInterval,
				OpensAt:   &nine,
				ClosesAt:  &twentyFour,
			}
		}},
		{name: "interval cannot cross midnight", change: func(input *CreateRestaurantInput) {
			input.WorkingHours[0] = CreateWorkingHoursInput{
				DayOfWeek: 1,
				Mode:      WorkingHoursInterval,
				OpensAt:   &eighteen,
				ClosesAt:  &nine,
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateRestaurantInput()
			tt.change(&input)
			repositoryCalled := false
			service := NewService(repositoryStub{create: func(restaurant Restaurant) (Restaurant, error) {
				repositoryCalled = true
				return restaurant, nil
			}})

			_, err := service.CreateRestaurant(context.Background(), input)
			if !errors.Is(err, ErrInvalidRestaurant) {
				t.Fatalf("error = %v, want ErrInvalidRestaurant", err)
			}
			if repositoryCalled {
				t.Error("repository was called for invalid input")
			}
		})
	}
}

func TestCreateRestaurantWrapsRepositoryConflict(t *testing.T) {
	service := NewService(repositoryStub{create: func(Restaurant) (Restaurant, error) {
		return Restaurant{}, ErrRestaurantAlreadyExists
	}})
	service.newID = func() (string, error) {
		return "00000000-0000-4000-8000-000000000001", nil
	}

	_, err := service.CreateRestaurant(context.Background(), validCreateRestaurantInput())
	if !errors.Is(err, ErrRestaurantAlreadyExists) {
		t.Fatalf("error = %v, want ErrRestaurantAlreadyExists", err)
	}
}

func validCreateRestaurantInput() CreateRestaurantInput {
	return CreateRestaurantInput{
		ExternalID:        "cafe-1",
		Name:              "Demo Cafe",
		OrderEndpoint:     "http://demo-cafe:8081/api/v1/orders",
		Timezone:          "Europe/Moscow",
		IsAcceptingOrders: true,
		WorkingHours: []CreateWorkingHoursInput{
			{DayOfWeek: 1, Mode: WorkingHoursOpen24H},
		},
	}
}
