package restaurant

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (s *Service) UpdateRestaurant(
	ctx context.Context,
	restaurantID string,
	input UpdateRestaurantInput,
) (Restaurant, error) {
	restaurantID = strings.TrimSpace(restaurantID)
	if !isUUID(restaurantID) {
		return Restaurant{}, invalidRestaurant("restaurant_id must be a valid UUID")
	}

	update, err := validateRestaurantUpdate(input)
	if err != nil {
		return Restaurant{}, err
	}

	updated, err := s.repository.UpdateRestaurant(ctx, restaurantID, update)
	if err != nil {
		return Restaurant{}, fmt.Errorf("update restaurant: %w", err)
	}

	return updated, nil
}

func validateRestaurantUpdate(input UpdateRestaurantInput) (RestaurantUpdate, error) {
	if input.Name == nil &&
		input.OrderEndpoint == nil &&
		input.Timezone == nil &&
		input.IsAcceptingOrders == nil &&
		input.WorkingHours == nil {
		return RestaurantUpdate{}, invalidRestaurant("at least one field must be provided")
	}

	update := RestaurantUpdate{
		IsAcceptingOrders: input.IsAcceptingOrders,
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return RestaurantUpdate{}, invalidRestaurant("name must not be empty")
		}
		update.Name = &name
	}

	if input.OrderEndpoint != nil {
		orderEndpoint := strings.TrimSpace(*input.OrderEndpoint)
		endpointURL, err := url.ParseRequestURI(orderEndpoint)
		if err != nil || endpointURL.Host == "" || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") {
			return RestaurantUpdate{}, invalidRestaurant("order_endpoint must be an absolute HTTP URL")
		}
		update.OrderEndpoint = &orderEndpoint
	}

	if input.Timezone != nil {
		timezone := strings.TrimSpace(*input.Timezone)
		if timezone == "" {
			return RestaurantUpdate{}, invalidRestaurant("timezone must not be empty")
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return RestaurantUpdate{}, invalidRestaurant("timezone must be a valid IANA timezone")
		}
		update.Timezone = &timezone
	}

	if input.WorkingHours != nil {
		workingHours, err := validateWorkingHoursList(*input.WorkingHours)
		if err != nil {
			return RestaurantUpdate{}, err
		}
		update.WorkingHours = &workingHours
	}

	return update, nil
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}

	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}

	_, err := hex.DecodeString(compact)
	return err == nil
}
