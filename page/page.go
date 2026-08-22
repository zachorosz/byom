// Package page provides the keyset pagination primitives shared by
// byom's stores and its RPC services: page size clamping and opaque
// cursor tokens.
package page

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	// DefaultSize is the page size used when a caller asks for none.
	DefaultSize = 50
	// MaxSize caps how many items a single page may return.
	MaxSize = 200
)

var ErrInvalidToken = errors.New("invalid page token")

// Size returns the effective page size for a requested size.
//
// It substitutes DefaultSize for values <= 0 and caps the size at MaxSize.
func Size(n int) int {
	if n <= 0 {
		return DefaultSize
	}
	return min(n, MaxSize)
}

// EncodeToken encodes keyset cursor fields into an opaque token.
func EncodeToken(fields ...string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, "\x00")))
}

// DecodeToken decodes a token into exactly the requested number of cursor fields.
//
// It returns ErrInvalidToken if the token is malformed or holds a different
// number of fields.
func DecodeToken(token string, want int) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	fields := strings.Split(string(raw), "\x00")
	if len(fields) != want {
		return nil, fmt.Errorf("%w: got %d fields, want %d", ErrInvalidToken, len(fields), want)
	}
	return fields, nil
}
