package metadata

import "github.com/zachorosz/byom/library"

// AlbumMetadata is tag-derived album information.
type AlbumMetadata struct {
	Type                library.AlbumType
	Title               string
	ReleaseDate         string
	OriginalReleaseDate string
	Bootleg             bool
	Compilation         bool
	Live                bool
}

// Credit is a tag-derived artist credit, before artist matching.
type Credit struct {
	CreditedName string
	Role         string
}

// TrackMetadata is tag-derived track information.
type TrackMetadata struct {
	DiscNumber          int
	DiscSubtitle        string
	TrackNumber         int
	Title               string
	ReleaseDate         string
	OriginalReleaseDate string
	Live                bool
}
