package library

import (
	"time"

	"github.com/google/uuid"
)

type Track struct {
	ID      uuid.UUID
	AlbumID uuid.UUID

	DiscNumber          int
	DiscSubtitle        string
	TrackNumber         int
	Title               string
	ReleaseDate         string
	OriginalReleaseDate string

	Credits []TrackCredit

	FileID      uuid.UUID
	Audio       AudioProperties
	Duration    time.Duration
	StartOffset time.Duration
}

type AudioProperties struct {
	Codec      string
	SampleRate int
	BitDepth   int
	Channels   int
	Bitrate    int
	Duration   time.Duration
}

type TrackCredit struct {
	ArtistID     uuid.UUID
	CreditedName string
	Role         string
}
