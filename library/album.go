package library

import (
	"errors"
	"fmt"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

type AlbumType string

const (
	// AlbumTypeUnknown is the zero value, used when a release type is
	// absent or unrecognized in the source tags.
	AlbumTypeUnknown AlbumType = ""
	AlbumMain        AlbumType = "main"
	AlbumSingle      AlbumType = "single"
	AlbumEP          AlbumType = "ep"
	AlbumOther       AlbumType = "other"
)

type Album struct {
	ID    uuid.UUID
	DirID uuid.UUID

	Type                AlbumType
	Title               string
	ReleaseDate         string
	OriginalReleaseDate string
	ReleaseCountry      string
	Bootleg             bool
	Compilation         bool
	Live                bool

	// Artists holds the album's credited artists in credited order; an
	// artist's position is its slice index.
	Artists []AlbumArtist
	Images  []AlbumImage

	// GroupKey groups alternate versions (remasters, reissues) of the
	// same album; exactly one album per group has PrimaryVersion set.
	GroupKey       string
	Version        string
	PrimaryVersion bool
}

type AlbumArtist struct {
	ArtistID     uuid.UUID
	CreditedName string
}

// AlbumGroupKey generates a key for clustering album versions (dirs) to a
// single album group/release. An error is returned if Album has missing
// data (e.g. ArtistID is uuid.Nil).
func AlbumGroupKey(al Album) (string, error) {
	buf := make([]byte, 0, 128)

	first := true
	for _, a := range al.Artists {
		if a.ArtistID == uuid.Nil {
			return "", errors.New("album contains nil artist ID")
		}
		if !first {
			buf = append(buf, '/')
		}
		buf = append(buf, a.ArtistID.String()...)
		first = false
	}

	buf = append(buf, 0) // null byte separator

	if al.Title != "" {
		buf = append(buf, al.Title...)
	} else {
		buf = append(buf, "[Unknown Album]"...)
	}

	sum := xxhash.Sum64(buf)
	return fmt.Sprintf("%x", sum), nil
}
