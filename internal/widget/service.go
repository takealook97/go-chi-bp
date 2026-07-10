package widget

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	defaultListLimit  = 20
	maximumListLimit  = 100
	maximumNameLength = 120
)

var (
	// ErrNotFound indicates that a widget does not exist.
	ErrNotFound = errors.New("widget not found")
	// ErrInvalidName indicates that a widget name violates domain rules.
	ErrInvalidName = errors.New("invalid widget name")
	// ErrInvalidPagination indicates that list bounds are invalid.
	ErrInvalidPagination = errors.New("invalid pagination")
)

// Repository is the persistence capability consumed by Service.
type Repository interface {
	Create(ctx context.Context, name string) (Widget, error)
	Get(ctx context.Context, id int64) (Widget, error)
	List(ctx context.Context, options ListOptions) ([]Widget, error)
	Delete(ctx context.Context, id int64) error
}

// Service implements widget use cases.
type Service struct {
	repository Repository
}

// NewService constructs a widget service.
func NewService(repository Repository) *Service {
	if repository == nil {
		panic("widget repository must not be nil")
	}

	return &Service{repository: repository}
}

// Create validates and persists a widget.
func (service *Service) Create(ctx context.Context, name string) (Widget, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maximumNameLength {
		return Widget{}, ErrInvalidName
	}

	result, err := service.repository.Create(ctx, name)
	if err != nil {
		return Widget{}, fmt.Errorf("create widget: %w", err)
	}

	return result, nil
}

// Get returns one widget by identifier.
func (service *Service) Get(ctx context.Context, id int64) (Widget, error) {
	if id < 1 {
		return Widget{}, ErrNotFound
	}

	result, err := service.repository.Get(ctx, id)
	if err != nil {
		return Widget{}, fmt.Errorf("get widget: %w", err)
	}

	return result, nil
}

// List returns a bounded page of widgets.
func (service *Service) List(ctx context.Context, options ListOptions) ([]Widget, error) {
	if options.Limit == 0 {
		options.Limit = defaultListLimit
	}
	if options.Limit < 1 || options.Limit > maximumListLimit || options.Offset < 0 {
		return nil, ErrInvalidPagination
	}

	results, err := service.repository.List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list widgets: %w", err)
	}

	return results, nil
}

// Delete removes one widget.
func (service *Service) Delete(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrNotFound
	}
	if err := service.repository.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete widget: %w", err)
	}

	return nil
}
