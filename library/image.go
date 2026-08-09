package library

import "github.com/google/uuid"

// Image is content-addressed image metadata. The bytes live on disk,
// stored and addressed by the image store via ContentHash.
type Image struct {
	ID          uuid.UUID
	ContentHash string
	MimeType    string
	Width       int
	Height      int
}

type ImageKind string

const (
	ImageCover  ImageKind = "cover"
	ImageBack   ImageKind = "back"
	ImageDisc   ImageKind = "disc"
	ImageArtist ImageKind = "artist"
	ImageOther  ImageKind = "other"
)

// AlbumImage links an image to an album. SetCover marks the album's
// canonical cover; at most one image per album has it set.
type AlbumImage struct {
	ImageID  uuid.UUID
	Kind     ImageKind
	SetCover bool
}
