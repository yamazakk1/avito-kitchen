package order

import (
	"context"
	"time"
)

type Repository interface {
	CreateOrder(context.Context, CreateOrderCommand) (Order, error)
	GetOrder(context.Context, string) (Order, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		now:        time.Now,
	}
}
