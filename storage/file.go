package storage

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
