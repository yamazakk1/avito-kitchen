package restaurant

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidRestaurant       = errors.New("invalid restaurant")
	ErrRestaurantAlreadyExists = errors.New("restaurant already exists")
	ErrRestaurantNotFound      = errors.New("restaurant not found")
)

func (s *Service) CreateRestaurant(ctx context.Context, input CreateRestaurantInput) (Restaurant, error) {
	restaurant, err := validateRestaurant(input)
	if err != nil {
		return Restaurant{}, err
	}

	restaurant.ID, err = s.newID()
	if err != nil {
		return Restaurant{}, fmt.Errorf("generate restaurant ID: %w", err)
	}

	created, err := s.repository.CreateRestaurant(ctx, restaurant)
	if err != nil {
		return Restaurant{}, fmt.Errorf("create restaurant: %w", err)
	}
	return created, nil
}

func validateRestaurant(input CreateRestaurantInput) (Restaurant, error) {
	externalID := strings.TrimSpace(input.ExternalID)
	if externalID == "" {
		return Restaurant{}, invalidRestaurant("external_id is required")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Restaurant{}, invalidRestaurant("name is required")
	}

	orderEndpoint := strings.TrimSpace(input.OrderEndpoint)
	endpointURL, err := url.ParseRequestURI(orderEndpoint)
	if err != nil || endpointURL.Host == "" || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") {
		return Restaurant{}, invalidRestaurant("order_endpoint must be an absolute HTTP URL")
	}

	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		return Restaurant{}, invalidRestaurant("timezone is required")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Restaurant{}, invalidRestaurant("timezone must be a valid IANA timezone")
	}

	workingHours, err := validateWorkingHoursList(input.WorkingHours)
	if err != nil {
		return Restaurant{}, err
	}

	return Restaurant{
		ExternalID:        externalID,
		Name:              name,
		OrderEndpoint:     orderEndpoint,
		Timezone:          timezone,
		IsAcceptingOrders: input.IsAcceptingOrders,
		WorkingHours:      workingHours,
	}, nil
}

func validateWorkingHoursList(input []CreateWorkingHoursInput) ([]WorkingHours, error) {
	if len(input) == 0 || len(input) > 7 {
		return nil, invalidRestaurant("working_hours must contain from 1 to 7 items")
	}

	workingHours := make([]WorkingHours, 0, len(input))
	days := make(map[int]struct{}, len(input))
	for _, hours := range input {
		if hours.DayOfWeek < 1 || hours.DayOfWeek > 7 {
			return nil, invalidRestaurant("day_of_week must be between 1 and 7")
		}
		if _, exists := days[hours.DayOfWeek]; exists {
			return nil, invalidRestaurant("working_hours contains duplicate day_of_week")
		}
		days[hours.DayOfWeek] = struct{}{}

		validatedHours, err := validateWorkingHours(hours)
		if err != nil {
			return nil, err
		}
		workingHours = append(workingHours, validatedHours)
	}

	return workingHours, nil
}

func validateWorkingHours(input CreateWorkingHoursInput) (WorkingHours, error) {
	hours := WorkingHours{
		DayOfWeek: input.DayOfWeek,
		Mode:      input.Mode,
	}

	switch input.Mode {
	case WorkingHoursClosed, WorkingHoursOpen24H:
		if input.OpensAt != nil || input.ClosesAt != nil {
			return WorkingHours{}, invalidRestaurant("CLOSED and OPEN_24H must not contain opens_at or closes_at")
		}
	case WorkingHoursInterval:
		if input.OpensAt == nil || input.ClosesAt == nil {
			return WorkingHours{}, invalidRestaurant("INTERVAL requires opens_at and closes_at")
		}
		if *input.OpensAt < 0 || *input.OpensAt >= 24*time.Hour ||
			*input.ClosesAt < 0 || *input.ClosesAt >= 24*time.Hour {
			return WorkingHours{}, invalidRestaurant("working hours time must be within one day")
		}
		if *input.OpensAt >= *input.ClosesAt {
			return WorkingHours{}, invalidRestaurant("opens_at must be earlier than closes_at")
		}
		hours.OpensAt = *input.OpensAt
		hours.ClosesAt = *input.ClosesAt
	default:
		return WorkingHours{}, invalidRestaurant("unknown working hours mode")
	}

	return hours, nil
}

func invalidRestaurant(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidRestaurant, message)
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
