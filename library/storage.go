package library

import (
	"time"

	"github.com/google/uuid"
)

type FileKind string

const (
	FileAudio FileKind = "audio"
	FileImage FileKind = "image"
)

type File struct {
	ID uuid.UUID
	// Name is the basename of the file, prefixed with its parent for disc dirs.
	Name    string
	Kind    FileKind
	Size    int64
	ModTime time.Time
	Missing bool
}

type Storage struct {
	ID        uuid.UUID
	URI       string
	Available bool
}

type SyncPayload struct {
	StorageID  uuid.UUID
	RelPath    string
	Generation int64
	Changed    []File
	Missing    []uuid.UUID
	// When true, the change set will be queued for parsing.
	Dirty bool
}

type ClaimedDir struct {
	ID               uuid.UUID
	LockedGeneration int64
}
