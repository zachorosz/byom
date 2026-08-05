package library

import "errors"

var (
	ErrExists    = errors.New("exists")
	ErrNotExists = errors.New("not exists")
)
