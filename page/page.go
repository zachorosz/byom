// Package page provides the keyset pagination primitives shared by
// byom's stores and its RPC services: page size clamping and opaque
// cursor tokens.
package page

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

// Encode encodes a keyset cursor into an opaque token.
//
// Cursors carry the parameters their listing was started under, so a
// listing that resumes with different parameters can be rejected
// rather than silently mixing two result sets.
func Encode(cursor any) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode page token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Decode decodes a token into dst, which must be a non-nil pointer to
// a cursor.
//
// It returns ErrInvalidToken if the token is malformed or does not
// match dst's shape, which is how a token minted for another listing
// is caught.
func Decode(token string, dst any) error {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return nil
}
