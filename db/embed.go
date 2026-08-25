// Package db embeds the versioned schema migrations so that the job applying
// them ships as one artifact. An image carrying the SQL beside the binary can be
// assembled from a different tree than the one that built it; an embedded set
// cannot, so the deployed migrator is the migrations it claims to hold.
package db

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Migrations returns the migrations rooted at the directory Goose reads.
func Migrations() fs.FS {
	// The embedded tree is fixed at build time, so a missing directory is a build
	// that should not have succeeded rather than a condition a caller can handle.
	migrations, err := fs.Sub(embedded, "migrations")
	if err != nil {
		panic("embedded migrations are missing: " + err.Error())
	}

	return migrations
}
