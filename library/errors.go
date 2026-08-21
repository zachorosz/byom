package library

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidPageToken = errors.New("invalid page token")
)
