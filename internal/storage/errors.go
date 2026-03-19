package storage

import "errors"

// Sentinel errors for storage operations.
var (
	ErrNotFound = errors.New("not found")
)
