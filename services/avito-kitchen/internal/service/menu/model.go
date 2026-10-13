package menu

import "time"

type Menu struct {
	RestaurantID    string
	SnapshotVersion int64
	UpdatedAt       time.Time
	Items           []Item
}

type Item struct {
	ID                string
	ExternalID        string
	Name              string
	Description       *string
	Price             int64
	AvailableQuantity int
}

type ReplaceSnapshotInput struct {
	SnapshotVersion int64
	Items           []SnapshotItemInput
}

type SnapshotItemInput struct {
	ExternalID        string
	Name              string
	Description       *string
	Price             int64
	AvailableQuantity int
}

type Snapshot struct {
	Version int64
	Items   []SnapshotItem
}

type SnapshotItem struct {
	ExternalID        string
	Name              string
	Description       *string
	Price             int64
	AvailableQuantity int
}

type SnapshotResult struct {
	SnapshotVersion int64
	UpdatedAt       time.Time
	ItemsCount      int
}

type AvailabilityResult struct {
	ExternalItemID    string
	AvailableQuantity int
	UpdatedAt         time.Time
}
