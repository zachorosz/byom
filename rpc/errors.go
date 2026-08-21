package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
)

var errPanic = errors.New("internal error")

// rpcError maps a domain error to the Connect code its cause implies,
// defaulting to CodeInternal.
func rpcError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, library.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, library.ErrInvalidPageToken):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

// parseID parses a request's resource ID, reporting a malformed value
// as CodeInvalidArgument.
func parseID(field, s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s: %w", field, err))
	}
	return id, nil
}

// parseOptionalID parses a filter ID, returning uuid.Nil when unset.
func parseOptionalID(field, s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return parseID(field, s)
}
