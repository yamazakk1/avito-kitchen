package menu

import "context"

type Repository interface {
	GetMenu(context.Context, string) (Menu, error)
	ReplaceSnapshot(context.Context, string, Snapshot) (SnapshotResult, error)
	UpdateAvailability(context.Context, string, string, int) (AvailabilityResult, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}
