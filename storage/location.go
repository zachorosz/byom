package storage

import (
	"fmt"
	"net/url"

	"github.com/google/uuid"
)

type Location struct {
	ID        uuid.UUID
	URI       string
	Available bool
}

// Root resolves the location's URI to a local filesystem path.
func (s Location) Root() (string, error) {
	u, err := url.Parse(s.URI)
	if err != nil {
		return "", fmt.Errorf("parse location uri %q: %w", s.URI, err)
	}
	switch u.Scheme {
	case "":
		return s.URI, nil
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return "", fmt.Errorf("location uri %q: remote host %q not supported", s.URI, u.Host)
		}
		if u.Path == "" {
			return "", fmt.Errorf("location uri %q: empty path", s.URI)
		}
		return u.Path, nil
	default:
		return "", fmt.Errorf("location uri %q: unsupported scheme %q", s.URI, u.Scheme)
	}
}
