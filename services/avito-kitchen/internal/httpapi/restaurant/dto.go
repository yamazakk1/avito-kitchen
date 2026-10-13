package restaurant

import (
	"errors"
	"fmt"
	"time"

	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

type listResponse struct {
	Restaurants []summaryResponse `json:"restaurants"`
}

type summaryResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IsAvailable bool   `json:"is_available"`
}

type createRequest struct {
	ExternalID        string                `json:"external_id"`
	Name              string                `json:"name"`
	OrderEndpoint     string                `json:"order_endpoint"`
	Timezone          string                `json:"timezone"`
	IsAcceptingOrders *bool                 `json:"is_accepting_orders"`
	WorkingHours      []workingHoursRequest `json:"working_hours"`
}

type updateRequest struct {
	Name              *string                `json:"name"`
	OrderEndpoint     *string                `json:"order_endpoint"`
	Timezone          *string                `json:"timezone"`
	IsAcceptingOrders *bool                  `json:"is_accepting_orders"`
	WorkingHours      *[]workingHoursRequest `json:"working_hours"`
}

type workingHoursRequest struct {
	DayOfWeek int                                `json:"day_of_week"`
	Mode      restaurantservice.WorkingHoursMode `json:"mode"`
	OpensAt   *string                            `json:"opens_at"`
	ClosesAt  *string                            `json:"closes_at"`
}

type response struct {
	ID                string                 `json:"id"`
	ExternalID        string                 `json:"external_id"`
	Name              string                 `json:"name"`
	OrderEndpoint     string                 `json:"order_endpoint"`
	Timezone          string                 `json:"timezone"`
	IsAcceptingOrders bool                   `json:"is_accepting_orders"`
	WorkingHours      []workingHoursResponse `json:"working_hours"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type workingHoursResponse struct {
	DayOfWeek int                                `json:"day_of_week"`
	Mode      restaurantservice.WorkingHoursMode `json:"mode"`
	OpensAt   *string                            `json:"opens_at"`
	ClosesAt  *string                            `json:"closes_at"`
}

func listResponseFromService(restaurants []restaurantservice.RestaurantSummary) listResponse {
	result := listResponse{
		Restaurants: make([]summaryResponse, 0, len(restaurants)),
	}
	for _, item := range restaurants {
		result.Restaurants = append(result.Restaurants, summaryResponse{
			ID:          item.ID,
			Name:        item.Name,
			IsAvailable: item.IsAvailable,
		})
	}
	return result
}

func createInputFromRequest(request createRequest) (restaurantservice.CreateRestaurantInput, error) {
	isAcceptingOrders := true
	if request.IsAcceptingOrders != nil {
		isAcceptingOrders = *request.IsAcceptingOrders
	}

	workingHours, err := workingHoursInputsFromRequest(request.WorkingHours)
	if err != nil {
		return restaurantservice.CreateRestaurantInput{}, err
	}

	return restaurantservice.CreateRestaurantInput{
		ExternalID:        request.ExternalID,
		Name:              request.Name,
		OrderEndpoint:     request.OrderEndpoint,
		Timezone:          request.Timezone,
		IsAcceptingOrders: isAcceptingOrders,
		WorkingHours:      workingHours,
	}, nil
}

func updateInputFromRequest(request updateRequest) (restaurantservice.UpdateRestaurantInput, error) {
	input := restaurantservice.UpdateRestaurantInput{
		Name:              request.Name,
		OrderEndpoint:     request.OrderEndpoint,
		Timezone:          request.Timezone,
		IsAcceptingOrders: request.IsAcceptingOrders,
	}

	if request.WorkingHours != nil {
		workingHours, err := workingHoursInputsFromRequest(*request.WorkingHours)
		if err != nil {
			return restaurantservice.UpdateRestaurantInput{}, err
		}
		input.WorkingHours = &workingHours
	}

	return input, nil
}

func workingHoursInputsFromRequest(
	request []workingHoursRequest,
) ([]restaurantservice.CreateWorkingHoursInput, error) {
	result := make([]restaurantservice.CreateWorkingHoursInput, 0, len(request))
	for _, hours := range request {
		opensAt, err := parseLocalTime(hours.OpensAt)
		if err != nil {
			return nil, fmt.Errorf(
				"parse opens_at for day %d: %w",
				hours.DayOfWeek,
				err,
			)
		}
		closesAt, err := parseLocalTime(hours.ClosesAt)
		if err != nil {
			return nil, fmt.Errorf(
				"parse closes_at for day %d: %w",
				hours.DayOfWeek,
				err,
			)
		}

		result = append(result, restaurantservice.CreateWorkingHoursInput{
			DayOfWeek: hours.DayOfWeek,
			Mode:      hours.Mode,
			OpensAt:   opensAt,
			ClosesAt:  closesAt,
		})
	}
	return result, nil
}

func responseFromService(restaurant restaurantservice.Restaurant) response {
	result := response{
		ID:                restaurant.ID,
		ExternalID:        restaurant.ExternalID,
		Name:              restaurant.Name,
		OrderEndpoint:     restaurant.OrderEndpoint,
		Timezone:          restaurant.Timezone,
		IsAcceptingOrders: restaurant.IsAcceptingOrders,
		WorkingHours:      make([]workingHoursResponse, 0, len(restaurant.WorkingHours)),
		CreatedAt:         restaurant.CreatedAt,
		UpdatedAt:         restaurant.UpdatedAt,
	}
	for _, hours := range restaurant.WorkingHours {
		item := workingHoursResponse{
			DayOfWeek: hours.DayOfWeek,
			Mode:      hours.Mode,
		}
		if hours.Mode == restaurantservice.WorkingHoursInterval {
			item.OpensAt = formatLocalTime(hours.OpensAt)
			item.ClosesAt = formatLocalTime(hours.ClosesAt)
		}
		result.WorkingHours = append(result.WorkingHours, item)
	}
	return result
}

func parseLocalTime(value *string) (*time.Duration, error) {
	if value == nil {
		return nil, nil
	}
	if len(*value) != len("HH:MM") {
		return nil, errors.New("expected HH:MM")
	}

	parsed, err := time.Parse("15:04", *value)
	if err != nil {
		return nil, fmt.Errorf("expected HH:MM: %w", err)
	}
	duration := time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute
	return &duration, nil
}

func formatLocalTime(value time.Duration) *string {
	formatted := fmt.Sprintf("%02d:%02d", value/time.Hour, value%time.Hour/time.Minute)
	return &formatted
}
