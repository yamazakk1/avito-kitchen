package restaurant

import "time"

type WorkingHoursMode string

const (
	WorkingHoursClosed   WorkingHoursMode = "CLOSED"
	WorkingHoursInterval WorkingHoursMode = "INTERVAL"
	WorkingHoursOpen24H  WorkingHoursMode = "OPEN_24H"
)

type Restaurant struct {
	ID                string
	ExternalID        string
	Name              string
	OrderEndpoint     string
	Timezone          string
	IsAcceptingOrders bool
	WorkingHours      []WorkingHours
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type WorkingHours struct {
	DayOfWeek int
	Mode      WorkingHoursMode
	OpensAt   time.Duration
	ClosesAt  time.Duration
}

type RestaurantSummary struct {
	ID          string
	Name        string
	IsAvailable bool
}

type CreateRestaurantInput struct {
	ExternalID        string
	Name              string
	OrderEndpoint     string
	Timezone          string
	IsAcceptingOrders bool
	WorkingHours      []CreateWorkingHoursInput
}

type CreateWorkingHoursInput struct {
	DayOfWeek int
	Mode      WorkingHoursMode
	OpensAt   *time.Duration
	ClosesAt  *time.Duration
}

type UpdateRestaurantInput struct {
	Name              *string
	OrderEndpoint     *string
	Timezone          *string
	IsAcceptingOrders *bool
	WorkingHours      *[]CreateWorkingHoursInput
}

type RestaurantUpdate struct {
	Name              *string
	OrderEndpoint     *string
	Timezone          *string
	IsAcceptingOrders *bool
	WorkingHours      *[]WorkingHours
}
