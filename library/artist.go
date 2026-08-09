package library

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/google/uuid"
)

type Artist struct {
	ID       uuid.UUID
	Name     string
	SortName string
}

var artistNameReplacer = strings.NewReplacer(
	"&", "and",
	// separators; spaces will collapse
	".", " ",
	"/", " ",
	// not important or structural
	",", "",
)

// NormalizeArtistName normalizes name for matching against the artist
// aliases.
func NormalizeArtistName(name string) string {
	// TODO: make more efficient

	name = artistNameReplacer.Replace(name)

	// remove diacritics
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	name, _, _ = transform.String(t, name)

	name = strings.ToLower(name)

	words := strings.Fields(name)
	return strings.Join(words, "-")
}
