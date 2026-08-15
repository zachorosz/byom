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

// newDiacriticRemover returns a transformer that strips combining
// marks. transform.Chain is stateful, so each caller needs its own.
func newDiacriticRemover() transform.Transformer {
	return transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)
}

// NormalizeArtistName normalizes name for matching against the artist aliases.
func NormalizeArtistName(name string) string {
	name, _, _ = transform.String(newDiacriticRemover(), name)

	var b strings.Builder
	b.Grow(len(name) + 8)

	inSpace := false
	for _, r := range name {
		switch r {
		case ',', '\'', '"', '`', '‘', '’', '“', '”':
			continue
		}
		// Word separators
		if r == '.' || r == '/' || unicode.IsSpace(r) || unicode.Is(unicode.Pd, r) {
			if b.Len() > 0 {
				inSpace = true
			}
			continue
		}
		// Resolve any accumulated separators into a single hyphen
		if inSpace {
			b.WriteByte('-')
			inSpace = false
		}
		// Handle special character transliterations and expansions
		switch r {
		case '&':
			b.WriteString("and")
		case 'Ø', 'ø':
			b.WriteByte('o')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}

	return b.String()
}
