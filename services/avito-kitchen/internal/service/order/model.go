package order

import "time"

type Status string

const (
	StatusProcessing Status = "PROCESSING"
	StatusAccepted   Status = "ACCEPTED"
	StatusRejected   Status = "REJECTED"
)

type RejectionReason string

const (
	RejectionReasonOutOfStock         RejectionReason = "OUT_OF_STOCK"
	RejectionReasonItemUnavailable    RejectionReason = "ITEM_UNAVAILABLE"
	RejectionReasonNotAcceptingOrders RejectionReason = "NOT_ACCEPTING_ORDERS"
	RejectionReasonStaleMenuSnapshot  RejectionReason = "STALE_MENU_SNAPSHOT"
)

type Order struct {
	ID                  string
	UserID              string
	RestaurantID        string
	ExternalOrderID     *string
	MenuSnapshotVersion int64
	Status              Status
	RejectionReason     *RejectionReason
	TotalPrice          int64
	Items               []Item
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Item struct {
	MenuItemID         string
	ExternalMenuItemID string
	Name               string
	UnitPrice          int64
	Quantity           int
}

type CreateOrderInput struct {
	UserID       string
	RestaurantID string
	Items        []CreateOrderItemInput
}

type CreateOrderItemInput struct {
	MenuItemID    string
	Quantity      int
	ExpectedPrice int64
}

type CreateOrderCommand struct {
	IdempotencyKey string
	RequestHash    string
	UserID         string
	RestaurantID   string
	Items          []CreateOrderItemInput
	Now            time.Time
}

type RestaurantState struct {
	Timezone          string
	IsAcceptingOrders bool
	WorkingHours      []WorkingHours
}

type WorkingHoursMode string

const (
	WorkingHoursClosed   WorkingHoursMode = "CLOSED"
	WorkingHoursInterval WorkingHoursMode = "INTERVAL"
	WorkingHoursOpen24H  WorkingHoursMode = "OPEN_24H"
)

type WorkingHours struct {
	DayOfWeek int
	Mode      WorkingHoursMode
	OpensAt   time.Duration
	ClosesAt  time.Duration
}

type MenuState struct {
	SnapshotVersion int64
	Items           []MenuItemState
}

type MenuItemState struct {
	ID                string
	ExternalID        string
	Name              string
	Price             int64
	AvailableQuantity int
	IsActive          bool
}

type PriceChangedItem struct {
	MenuItemID    string
	ExpectedPrice int64
	CurrentPrice  int64
}

type DeliveryOrder struct {
	ID                   string
	RestaurantExternalID string
	OrderEndpoint        string
	MenuSnapshotVersion  int64
	Items                []DeliveryItem
}

type DeliveryItem struct {
	ExternalMenuItemID string
	Quantity           int
}

type DeliveryResult struct {
	Status          Status
	ExternalOrderID string
	RejectionReason RejectionReason
}
