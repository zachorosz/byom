package library

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	// DefaultPageSize is the page size used when a caller asks for none.
	DefaultPageSize = 50
	// MaxPageSize caps how many items a single page may return.
	MaxPageSize = 200
)

// PageSize returns the effective page size for a requested size,
// substituting DefaultPageSize for values <= 0 and capping at
// MaxPageSize.
func PageSize(n int) int {
	if n <= 0 {
		return DefaultPageSize
	}
	return min(n, MaxPageSize)
}

// EncodePageToken encodes keyset cursor fields into an opaque token.
// The token is a store implementation detail; callers pass it back
// unread.
func EncodePageToken(fields ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, "\x00")))
}

// DecodePageToken decodes token into exactly want cursor fields. It
// returns ErrInvalidPageToken if token is malformed or holds a
// different number of fields, which callers should surface as a client
// error rather than a server fault.
func DecodePageToken(token string, want int) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPageToken, err)
	}
	fields := strings.Split(string(raw), "\x00")
	if len(fields) != want {
		return nil, fmt.Errorf("%w: got %d fields, want %d", ErrInvalidPageToken, len(fields), want)
	}
	return fields, nil
}
