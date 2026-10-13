package restaurant

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUpdateRestaurant(t *testing.T) {
	name := " Updated Cafe "
	endpoint := " http://demo-cafe:8081/api/v1/new-orders "
	timezone := " UTC "
	isAccepting := false
	opensAt := 10 * time.Hour
	closesAt := 19 * time.Hour
	workingHours := []CreateWorkingHoursInput{
		{
			DayOfWeek: 1,
			Mode:      WorkingHoursInterval,
			OpensAt:   &opensAt,
			ClosesAt:  &closesAt,
		},
		{DayOfWeek: 2, Mode: WorkingHoursOpen24H},
	}

	var (
		savedID     string
		savedUpdate RestaurantUpdate
	)
	service := NewService(repositoryStub{update: func(
		restaurantID string,
		update RestaurantUpdate,
	) (Restaurant, error) {
		savedID = restaurantID
		savedUpdate = update
		return Restaurant{
			ID:                restaurantID,
			ExternalID:        "cafe-1",
			Name:              *update.Name,
			OrderEndpoint:     *update.OrderEndpoint,
			Timezone:          *update.Timezone,
			IsAcceptingOrders: *update.IsAcceptingOrders,
			WorkingHours:      *update.WorkingHours,
		}, nil
	}})

	updated, err := service.UpdateRestaurant(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		UpdateRestaurantInput{
			Name:              &name,
			OrderEndpoint:     &endpoint,
			Timezone:          &timezone,
			IsAcceptingOrders: &isAccepting,
			WorkingHours:      &workingHours,
		},
	)
	if err != nil {
		t.Fatalf("UpdateRestaurant() error = %v", err)
	}

	if savedID != "00000000-0000-4000-8000-000000000001" {
		t.Errorf("restaurant ID = %q", savedID)
	}
	if savedUpdate.Name == nil || *savedUpdate.Name != "Updated Cafe" {
		t.Errorf("name = %#v", savedUpdate.Name)
	}
	if savedUpdate.OrderEndpoint == nil || *savedUpdate.OrderEndpoint != "http://demo-cafe:8081/api/v1/new-orders" {
		t.Errorf("order endpoint = %#v", savedUpdate.OrderEndpoint)
	}
	if savedUpdate.Timezone == nil || *savedUpdate.Timezone != "UTC" {
		t.Errorf("timezone = %#v", savedUpdate.Timezone)
	}
	if savedUpdate.IsAcceptingOrders == nil || *savedUpdate.IsAcceptingOrders {
		t.Errorf("is_accepting_orders = %#v", savedUpdate.IsAcceptingOrders)
	}
	if savedUpdate.WorkingHours == nil || len(*savedUpdate.WorkingHours) != 2 {
		t.Fatalf("working hours = %#v", savedUpdate.WorkingHours)
	}
	if (*savedUpdate.WorkingHours)[0].OpensAt != 10*time.Hour ||
		(*savedUpdate.WorkingHours)[0].ClosesAt != 19*time.Hour {
		t.Errorf("working hours = %#v", (*savedUpdate.WorkingHours)[0])
	}
	if updated.Name != "Updated Cafe" || updated.IsAcceptingOrders {
		t.Errorf("updated restaurant = %#v", updated)
	}
}

func TestUpdateRestaurantPartialPatchPreservesOmittedFields(t *testing.T) {
	isAccepting := false
	service := NewService(repositoryStub{update: func(
		_ string,
		update RestaurantUpdate,
	) (Restaurant, error) {
		if update.Name != nil || update.OrderEndpoint != nil || update.Timezone != nil || update.WorkingHours != nil {
			t.Fatalf("unexpected fields in update = %#v", update)
		}
		if update.IsAcceptingOrders == nil || *update.IsAcceptingOrders {
			t.Fatalf("is_accepting_orders = %#v", update.IsAcceptingOrders)
		}
		return Restaurant{ID: "00000000-0000-4000-8000-000000000001"}, nil
	}})

	_, err := service.UpdateRestaurant(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		UpdateRestaurantInput{IsAcceptingOrders: &isAccepting},
	)
	if err != nil {
		t.Fatalf("UpdateRestaurant() error = %v", err)
	}
}

func TestUpdateRestaurantValidation(t *testing.T) {
	validID := "00000000-0000-4000-8000-000000000001"
	nine := 9 * time.Hour
	eighteen := 18 * time.Hour

	tests := []struct {
		name  string
		id    string
		input UpdateRestaurantInput
	}{
		{name: "invalid restaurant ID", id: "not-a-uuid", input: updateWithName("Cafe")},
		{name: "empty patch", id: validID, input: UpdateRestaurantInput{}},
		{name: "empty name", id: validID, input: updateWithName(" ")},
		{name: "invalid endpoint", id: validID, input: updateWithEndpoint("ftp://cafe/orders")},
		{name: "invalid timezone", id: validID, input: updateWithTimezone("invalid/timezone")},
		{name: "empty working hours", id: validID, input: updateWithWorkingHours([]CreateWorkingHoursInput{})},
		{
			name: "duplicate weekday",
			id:   validID,
			input: updateWithWorkingHours([]CreateWorkingHoursInput{
				{DayOfWeek: 1, Mode: WorkingHoursOpen24H},
				{DayOfWeek: 1, Mode: WorkingHoursClosed},
			}),
		},
		{
			name: "invalid interval",
			id:   validID,
			input: updateWithWorkingHours([]CreateWorkingHoursInput{
				{DayOfWeek: 1, Mode: WorkingHoursInterval, OpensAt: &eighteen, ClosesAt: &nine},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repositoryCalled := false
			service := NewService(repositoryStub{update: func(string, RestaurantUpdate) (Restaurant, error) {
				repositoryCalled = true
				return Restaurant{}, nil
			}})

			_, err := service.UpdateRestaurant(context.Background(), tt.id, tt.input)
			if !errors.Is(err, ErrInvalidRestaurant) {
				t.Fatalf("error = %v, want ErrInvalidRestaurant", err)
			}
			if repositoryCalled {
				t.Error("repository was called for invalid input")
			}
		})
	}
}

func TestUpdateRestaurantWrapsRepositoryNotFound(t *testing.T) {
	service := NewService(repositoryStub{update: func(string, RestaurantUpdate) (Restaurant, error) {
		return Restaurant{}, ErrRestaurantNotFound
	}})

	_, err := service.UpdateRestaurant(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		updateWithName("Cafe"),
	)
	if !errors.Is(err, ErrRestaurantNotFound) {
		t.Fatalf("error = %v, want ErrRestaurantNotFound", err)
	}
}

func updateWithName(value string) UpdateRestaurantInput {
	return UpdateRestaurantInput{Name: &value}
}

func updateWithEndpoint(value string) UpdateRestaurantInput {
	return UpdateRestaurantInput{OrderEndpoint: &value}
}

func updateWithTimezone(value string) UpdateRestaurantInput {
	return UpdateRestaurantInput{Timezone: &value}
}

func updateWithWorkingHours(value []CreateWorkingHoursInput) UpdateRestaurantInput {
	return UpdateRestaurantInput{WorkingHours: &value}
}
