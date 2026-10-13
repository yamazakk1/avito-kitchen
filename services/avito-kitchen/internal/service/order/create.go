package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
)

var (
	ErrInvalidOrder          = errors.New("invalid order")
	ErrRestaurantNotFound    = errors.New("restaurant not found")
	ErrRestaurantUnavailable = errors.New("restaurant unavailable")
	ErrMenuItemNotFound      = errors.New("menu item not found")
	ErrItemUnavailable       = errors.New("item unavailable")
	ErrInsufficientStock     = errors.New("insufficient stock")
	ErrPriceChanged          = errors.New("price changed")
	ErrIdempotencyKeyReused  = errors.New("idempotency key reused")
)

type PriceChangedError struct {
	Items []PriceChangedItem
}

func (e *PriceChangedError) Error() string {
	return fmt.Sprintf("%v: %d item prices changed", ErrPriceChanged, len(e.Items))
}

func (e *PriceChangedError) Unwrap() error {
	return ErrPriceChanged
}

func (s *Service) CreateOrder(
	ctx context.Context,
	idempotencyKey string,
	input CreateOrderInput,
) (Order, error) {
	command, err := validateCreateOrder(idempotencyKey, input)
	if err != nil {
		return Order{}, err
	}

	command.RequestHash, err = hashCreateOrderRequest(command)
	if err != nil {
		return Order{}, fmt.Errorf("hash create order request: %w", err)
	}
	command.Now = s.now()

	created, err := s.repository.CreateOrder(ctx, command)
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	return created, nil
}

func validateCreateOrder(idempotencyKey string, input CreateOrderInput) (CreateOrderCommand, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		return CreateOrderCommand{}, invalidOrder("Idempotency-Key must contain from 1 to 255 characters")
	}

	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return CreateOrderCommand{}, invalidOrder("user_id is required")
	}

	restaurantID := strings.TrimSpace(input.RestaurantID)
	if !isUUID(restaurantID) {
		return CreateOrderCommand{}, invalidOrder("restaurant_id must be a valid UUID")
	}

	if len(input.Items) == 0 {
		return CreateOrderCommand{}, invalidOrder("items must contain at least one item")
	}

	items := make([]CreateOrderItemInput, 0, len(input.Items))
	seenMenuItems := make(map[string]struct{}, len(input.Items))
	for _, item := range input.Items {
		menuItemID := strings.TrimSpace(item.MenuItemID)
		if !isUUID(menuItemID) {
			return CreateOrderCommand{}, invalidOrder("menu_item_id must be a valid UUID")
		}
		if _, exists := seenMenuItems[menuItemID]; exists {
			return CreateOrderCommand{}, invalidOrder("items contains duplicate menu_item_id")
		}
		seenMenuItems[menuItemID] = struct{}{}

		if item.Quantity < 1 || item.Quantity > math.MaxInt32 {
			return CreateOrderCommand{}, invalidOrder("quantity must be between 1 and 2147483647")
		}
		if item.ExpectedPrice < 0 {
			return CreateOrderCommand{}, invalidOrder("expected_price must be non-negative")
		}

		items = append(items, CreateOrderItemInput{
			MenuItemID:    menuItemID,
			Quantity:      item.Quantity,
			ExpectedPrice: item.ExpectedPrice,
		})
	}

	return CreateOrderCommand{
		IdempotencyKey: idempotencyKey,
		UserID:         userID,
		RestaurantID:   restaurantID,
		Items:          items,
	}, nil
}

func hashCreateOrderRequest(command CreateOrderCommand) (string, error) {
	type hashItem struct {
		MenuItemID    string `json:"menu_item_id"`
		Quantity      int    `json:"quantity"`
		ExpectedPrice int64  `json:"expected_price"`
	}
	type hashRequest struct {
		UserID       string     `json:"user_id"`
		RestaurantID string     `json:"restaurant_id"`
		Items        []hashItem `json:"items"`
	}

	items := make([]hashItem, 0, len(command.Items))
	for _, item := range command.Items {
		items = append(items, hashItem{
			MenuItemID:    item.MenuItemID,
			Quantity:      item.Quantity,
			ExpectedPrice: item.ExpectedPrice,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].MenuItemID < items[j].MenuItemID
	})

	payload, err := json.Marshal(hashRequest{
		UserID:       command.UserID,
		RestaurantID: command.RestaurantID,
		Items:        items,
	})
	if err != nil {
		return "", fmt.Errorf("marshal canonical request: %w", err)
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func BuildProcessingOrder(
	command CreateOrderCommand,
	restaurant RestaurantState,
	menu MenuState,
) (Order, error) {
	available, err := isRestaurantAvailableAt(restaurant, command.Now)
	if err != nil {
		return Order{}, fmt.Errorf("determine restaurant availability: %w", err)
	}
	if !available {
		return Order{}, ErrRestaurantUnavailable
	}

	menuItems := make(map[string]MenuItemState, len(menu.Items))
	for _, item := range menu.Items {
		menuItems[item.ID] = item
	}

	orderItems := make([]Item, 0, len(command.Items))
	priceChanges := make([]PriceChangedItem, 0)
	var totalPrice int64

	for _, requestedItem := range command.Items {
		currentItem, exists := menuItems[requestedItem.MenuItemID]
		if !exists {
			return Order{}, ErrMenuItemNotFound
		}
		if !currentItem.IsActive {
			return Order{}, ErrItemUnavailable
		}
		if currentItem.AvailableQuantity < requestedItem.Quantity {
			return Order{}, ErrInsufficientStock
		}
		if currentItem.Price != requestedItem.ExpectedPrice {
			priceChanges = append(priceChanges, PriceChangedItem{
				MenuItemID:    requestedItem.MenuItemID,
				ExpectedPrice: requestedItem.ExpectedPrice,
				CurrentPrice:  currentItem.Price,
			})
			continue
		}

		quantity := int64(requestedItem.Quantity)
		if currentItem.Price > 0 && quantity > math.MaxInt64/currentItem.Price {
			return Order{}, invalidOrder("total_price exceeds int64 range")
		}
		lineTotal := currentItem.Price * quantity
		if lineTotal > math.MaxInt64-totalPrice {
			return Order{}, invalidOrder("total_price exceeds int64 range")
		}
		totalPrice += lineTotal

		orderItems = append(orderItems, Item{
			MenuItemID:         currentItem.ID,
			ExternalMenuItemID: currentItem.ExternalID,
			Name:               currentItem.Name,
			UnitPrice:          currentItem.Price,
			Quantity:           requestedItem.Quantity,
		})
	}

	if len(priceChanges) > 0 {
		return Order{}, &PriceChangedError{Items: priceChanges}
	}

	return Order{
		UserID:              command.UserID,
		RestaurantID:        command.RestaurantID,
		MenuSnapshotVersion: menu.SnapshotVersion,
		Status:              StatusProcessing,
		TotalPrice:          totalPrice,
		Items:               orderItems,
	}, nil
}

func isRestaurantAvailableAt(restaurant RestaurantState, now time.Time) (bool, error) {
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

func invalidOrder(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidOrder, message)
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
