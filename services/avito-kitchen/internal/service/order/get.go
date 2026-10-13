package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrOrderNotFound = errors.New("order not found")

func (s *Service) GetOrder(ctx context.Context, orderID string) (Order, error) {
	orderID = strings.TrimSpace(orderID)
	if !isUUID(orderID) {
		return Order{}, ErrOrderNotFound
	}

	order, err := s.repository.GetOrder(ctx, orderID)
	if err != nil {
		return Order{}, fmt.Errorf("get order: %w", err)
	}
	return order, nil
}
