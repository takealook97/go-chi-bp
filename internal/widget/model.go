// Package widget contains transport- and persistence-independent widget use cases.
package widget

import "time"

// Widget is the module's domain representation.
type Widget struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ListCursor identifies the last widget returned by a previous page.
type ListCursor struct {
	CreatedAt time.Time
	ID        int64
}

// ListOptions controls bounded widget listing.
type ListOptions struct {
	Limit  int32
	Cursor *ListCursor
}

// Page contains one bounded page and an optional continuation cursor.
type Page struct {
	Items []Widget
	// Limit is the page size actually applied, which may differ from a
	// caller-supplied zero value that selects the default.
	Limit      int32
	NextCursor *ListCursor
}
