package restaurant

import (
	"context"
	"fmt"
	"time"
	_ "time/tzdata"
)

type Repository interface {
	ListRestaurants(context.Context) ([]Restaurant, error)
	CreateRestaurant(context.Context, Restaurant) (Restaurant, error)
	UpdateRestaurant(context.Context, string, RestaurantUpdate) (Restaurant, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func() (string, error)
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		now:        time.Now,
		newID:      newUUID,
	}
}

func (s *Service) ListRestaurants(ctx context.Context) ([]RestaurantSummary, error) {
	restaurants, err := s.repository.ListRestaurants(ctx)
	if err != nil {
		return nil, fmt.Errorf("list restaurants: %w", err)
	}

	now := s.now()
	result := make([]RestaurantSummary, 0, len(restaurants))
	for _, restaurant := range restaurants {
		available, err := isRestaurantAvailable(restaurant, now)
		if err != nil {
			return nil, fmt.Errorf("determine availability of restaurant %s: %w", restaurant.ID, err)
		}
		if !available {
			continue
		}

		result = append(result, RestaurantSummary{
			ID:          restaurant.ID,
			Name:        restaurant.Name,
			IsAvailable: true,
		})
	}

	return result, nil
}

func isRestaurantAvailable(restaurant Restaurant, now time.Time) (bool, error) {
	if !restaurant.IsAcceptingOrders {
		return false, nil
	}

	location, err := time.LoadLocation(restaurant.Timezone)
	if err != nil {
		return false, fmt.Errorf("load timezone %q: %w", restaurant.Timezone, err)
	}

	localTime := now.In(location)
	dayOfWeek := isoWeekday(localTime.Weekday())
	for _, hours := range restaurant.WorkingHours {
		if hours.DayOfWeek != dayOfWeek {
			continue
		}

		switch hours.Mode {
		case WorkingHoursClosed:
			return false, nil
		case WorkingHoursOpen24H:
			return true, nil
		case WorkingHoursInterval:
			currentTime := time.Duration(localTime.Hour())*time.Hour +
				time.Duration(localTime.Minute())*time.Minute +
				time.Duration(localTime.Second())*time.Second +
				time.Duration(localTime.Nanosecond())
			return hours.OpensAt <= currentTime && currentTime < hours.ClosesAt, nil
		default:
			return false, fmt.Errorf("unknown working hours mode %q", hours.Mode)
		}
	}

	return false, nil
}

func isoWeekday(weekday time.Weekday) int {
	if weekday == time.Sunday {
		return 7
	}
	return int(weekday)
}
