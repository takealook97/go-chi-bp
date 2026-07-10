package widget

import "time"

// Widget is the module's domain representation.
type Widget struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ListOptions controls bounded widget listing.
type ListOptions struct {
	Limit  int32
	Offset int32
}
