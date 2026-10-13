package order

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

const (
	testRestaurantID = "00000000-0000-4000-8000-000000000001"
	testMenuItemID1  = "20000000-0000-4000-8000-000000000001"
	testMenuItemID2  = "20000000-0000-4000-8000-000000000002"
)

type repositoryStub struct {
	create func(CreateOrderCommand) (Order, error)
	get    func(string) (Order, error)
}

func (s repositoryStub) CreateOrder(_ context.Context, command CreateOrderCommand) (Order, error) {
	if s.create != nil {
		return s.create(command)
	}
	return Order{}, nil
}

func (s repositoryStub) GetOrder(_ context.Context, orderID string) (Order, error) {
	if s.get != nil {
		return s.get(orderID)
	}
	return Order{}, nil
}

func TestCreateOrderValidatesAndNormalizesInput(t *testing.T) {
	var captured CreateOrderCommand
	service := NewService(repositoryStub{create: func(command CreateOrderCommand) (Order, error) {
		captured = command
		return Order{ID: "order-id"}, nil
	}})
	now := time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	_, err := service.CreateOrder(context.Background(), "  retry-key  ", CreateOrderInput{
		UserID:       "  user-42  ",
		RestaurantID: "  " + testRestaurantID + "  ",
		Items: []CreateOrderItemInput{
			{MenuItemID: "  " + testMenuItemID1 + "  ", Quantity: 2, ExpectedPrice: 75000},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	if captured.IdempotencyKey != "retry-key" {
		t.Errorf("IdempotencyKey = %q", captured.IdempotencyKey)
	}
	if captured.UserID != "user-42" {
		t.Errorf("UserID = %q", captured.UserID)
	}
	if captured.RestaurantID != testRestaurantID {
		t.Errorf("RestaurantID = %q", captured.RestaurantID)
	}
	if captured.Items[0].MenuItemID != testMenuItemID1 {
		t.Errorf("MenuItemID = %q", captured.Items[0].MenuItemID)
	}
	if len(captured.RequestHash) != 64 {
		t.Errorf("RequestHash length = %d, want 64", len(captured.RequestHash))
	}
	if !captured.Now.Equal(now) {
		t.Errorf("Now = %v, want %v", captured.Now, now)
	}
}

func TestCreateOrderHashDoesNotDependOnItemOrder(t *testing.T) {
	first, err := validateCreateOrder("key", CreateOrderInput{
		UserID:       "user",
		RestaurantID: testRestaurantID,
		Items: []CreateOrderItemInput{
			{MenuItemID: testMenuItemID1, Quantity: 1, ExpectedPrice: 10},
			{MenuItemID: testMenuItemID2, Quantity: 2, ExpectedPrice: 20},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Items = []CreateOrderItemInput{first.Items[1], first.Items[0]}

	firstHash, err := hashCreateOrderRequest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := hashCreateOrderRequest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Errorf("hashes differ: %q != %q", firstHash, secondHash)
	}
}

func TestCreateOrderRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		key  string
		in   CreateOrderInput
	}{
		{name: "missing idempotency key", in: validCreateOrderInput()},
		{name: "empty user", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.UserID = " "; return v }()},
		{name: "invalid restaurant UUID", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.RestaurantID = "bad"; return v }()},
		{name: "empty items", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.Items = nil; return v }()},
		{name: "invalid item UUID", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.Items[0].MenuItemID = "bad"; return v }()},
		{name: "zero quantity", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.Items[0].Quantity = 0; return v }()},
		{name: "quantity over int32", key: "key", in: func() CreateOrderInput {
			v := validCreateOrderInput()
			v.Items[0].Quantity = math.MaxInt32 + 1
			return v
		}()},
		{name: "negative expected price", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.Items[0].ExpectedPrice = -1; return v }()},
		{name: "duplicate item", key: "key", in: func() CreateOrderInput { v := validCreateOrderInput(); v.Items = append(v.Items, v.Items[0]); return v }()},
	}

	service := NewService(repositoryStub{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateOrder(context.Background(), tt.key, tt.in)
			if !errors.Is(err, ErrInvalidOrder) {
				t.Fatalf("error = %v, want ErrInvalidOrder", err)
			}
		})
	}
}

func TestBuildProcessingOrderCreatesImmutableSnapshot(t *testing.T) {
	command := validCreateOrderCommand()
	command.Items = []CreateOrderItemInput{
		{MenuItemID: testMenuItemID1, Quantity: 2, ExpectedPrice: 75000},
		{MenuItemID: testMenuItemID2, Quantity: 1, ExpectedPrice: 15000},
	}
	menu := MenuState{
		SnapshotVersion: 7,
		Items: []MenuItemState{
			{ID: testMenuItemID1, ExternalID: "pizza-1", Name: "Margherita", Price: 75000, AvailableQuantity: 5, IsActive: true},
			{ID: testMenuItemID2, ExternalID: "cola-1", Name: "Cola", Price: 15000, AvailableQuantity: 3, IsActive: true},
		},
	}

	created, err := BuildProcessingOrder(command, openRestaurant(), menu)
	if err != nil {
		t.Fatalf("BuildProcessingOrder() error = %v", err)
	}
	if created.Status != StatusProcessing {
		t.Errorf("Status = %q", created.Status)
	}
	if created.TotalPrice != 165000 {
		t.Errorf("TotalPrice = %d, want 165000", created.TotalPrice)
	}
	if created.MenuSnapshotVersion != 7 {
		t.Errorf("MenuSnapshotVersion = %d", created.MenuSnapshotVersion)
	}
	if len(created.Items) != 2 {
		t.Fatalf("items count = %d", len(created.Items))
	}
	if created.Items[0].ExternalMenuItemID != "pizza-1" || created.Items[0].Name != "Margherita" || created.Items[0].UnitPrice != 75000 {
		t.Errorf("first order item = %#v", created.Items[0])
	}
}

func TestBuildProcessingOrderBusinessConflicts(t *testing.T) {
	baseMenu := MenuState{
		SnapshotVersion: 1,
		Items: []MenuItemState{
			{ID: testMenuItemID1, ExternalID: "pizza-1", Name: "Pizza", Price: 75000, AvailableQuantity: 2, IsActive: true},
		},
	}

	tests := []struct {
		name       string
		command    CreateOrderCommand
		restaurant RestaurantState
		menu       MenuState
		want       error
	}{
		{name: "restaurant unavailable", command: validCreateOrderCommand(), restaurant: RestaurantState{Timezone: "UTC", IsAcceptingOrders: false}, menu: baseMenu, want: ErrRestaurantUnavailable},
		{name: "item missing", command: validCreateOrderCommand(), restaurant: openRestaurant(), menu: MenuState{SnapshotVersion: 1}, want: ErrMenuItemNotFound},
		{name: "item inactive", command: validCreateOrderCommand(), restaurant: openRestaurant(), menu: MenuState{SnapshotVersion: 1, Items: []MenuItemState{{ID: testMenuItemID1, Price: 75000, AvailableQuantity: 2}}}, want: ErrItemUnavailable},
		{name: "insufficient stock", command: func() CreateOrderCommand { v := validCreateOrderCommand(); v.Items[0].Quantity = 3; return v }(), restaurant: openRestaurant(), menu: baseMenu, want: ErrInsufficientStock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildProcessingOrder(tt.command, tt.restaurant, tt.menu)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBuildProcessingOrderReportsPriceChange(t *testing.T) {
	command := validCreateOrderCommand()
	command.Items[0].ExpectedPrice = 70000

	_, err := BuildProcessingOrder(command, openRestaurant(), MenuState{
		SnapshotVersion: 3,
		Items: []MenuItemState{
			{ID: testMenuItemID1, ExternalID: "pizza-1", Name: "Pizza", Price: 75000, AvailableQuantity: 2, IsActive: true},
		},
	})
	var priceError *PriceChangedError
	if !errors.As(err, &priceError) {
		t.Fatalf("error = %v, want *PriceChangedError", err)
	}
	if len(priceError.Items) != 1 || priceError.Items[0].CurrentPrice != 75000 || priceError.Items[0].ExpectedPrice != 70000 {
		t.Errorf("price error = %#v", priceError)
	}
}

func validCreateOrderInput() CreateOrderInput {
	return CreateOrderInput{
		UserID:       "user-1",
		RestaurantID: testRestaurantID,
		Items: []CreateOrderItemInput{
			{MenuItemID: testMenuItemID1, Quantity: 1, ExpectedPrice: 75000},
		},
	}
}

func validCreateOrderCommand() CreateOrderCommand {
	return CreateOrderCommand{
		IdempotencyKey: "key",
		RequestHash:    "hash",
		UserID:         "user-1",
		RestaurantID:   testRestaurantID,
		Items: []CreateOrderItemInput{
			{MenuItemID: testMenuItemID1, Quantity: 1, ExpectedPrice: 75000},
		},
		Now: time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC),
	}
}

func openRestaurant() RestaurantState {
	return RestaurantState{
		Timezone:          "UTC",
		IsAcceptingOrders: true,
		WorkingHours: []WorkingHours{
			{DayOfWeek: 1, Mode: WorkingHoursOpen24H},
		},
	}
}
