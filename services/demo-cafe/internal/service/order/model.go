package order

type Status string

const (
	StatusAccepted Status = "ACCEPTED"
	StatusRejected Status = "REJECTED"
)

type RejectionReason string

const (
	RejectionReasonOutOfStock         RejectionReason = "OUT_OF_STOCK"
	RejectionReasonItemUnavailable    RejectionReason = "ITEM_UNAVAILABLE"
	RejectionReasonNotAcceptingOrders RejectionReason = "NOT_ACCEPTING_ORDERS"
	RejectionReasonStaleMenuSnapshot  RejectionReason = "STALE_MENU_SNAPSHOT"
)

type SubmitOrderInput struct {
	AvitoOrderID         string
	RestaurantExternalID string
	MenuSnapshotVersion  int64
	Items                []SubmitOrderItemInput
}

type SubmitOrderItemInput struct {
	ExternalItemID string
	Quantity       int
}

type Result struct {
	Status                 Status
	ExternalOrderID        string
	RejectionReason        RejectionReason
	CurrentSnapshotVersion *int64
}

type State struct {
	RestaurantExternalID string
	MenuSnapshotVersion  int64
	IsAcceptingOrders    bool
	Items                map[string]MenuItem
}

type MenuItem struct {
	AvailableQuantity int
	IsAvailable       bool
}
