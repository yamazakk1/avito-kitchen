package order

import "sync"

type processedOrder struct {
	requestHash string
	result      Result
}

type Service struct {
	mu        sync.Mutex
	state     State
	processed map[string]processedOrder
}

func NewService(state State) *Service {
	items := make(map[string]MenuItem, len(state.Items))
	for externalID, item := range state.Items {
		items[externalID] = item
	}
	state.Items = items

	return &Service{
		state:     state,
		processed: make(map[string]processedOrder),
	}
}

func DefaultState() State {
	return State{
		RestaurantExternalID: "demo-cafe-1",
		MenuSnapshotVersion:  1,
		IsAcceptingOrders:    true,
		Items: map[string]MenuItem{
			"pizza-1": {
				AvailableQuantity: 10,
				IsAvailable:       true,
			},
			"cola-1": {
				AvailableQuantity: 20,
				IsAvailable:       true,
			},
			"unavailable-1": {
				AvailableQuantity: 10,
				IsAvailable:       false,
			},
			"sold-out-1": {
				AvailableQuantity: 0,
				IsAvailable:       true,
			},
		},
	}
}
