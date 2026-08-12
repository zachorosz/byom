package library

import "testing"

func TestNormalizeArtistName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "StandardLowercase", in: "Thee Oh Sees", want: "thee-oh-sees"},
		{name: "AlreadyNormalized", in: "thee-oh-sees", want: "thee-oh-sees"},
		{name: "QuestionMark", in: "? and the Mysterians", want: "?-and-the-mysterians"},
		{name: "ExclamationMark", in: "Wham!", want: "wham!"},
		{name: "Empty", in: "", want: ""},
		// ignored punctuation
		{name: "Comma", in: "Crosby, Stills, Nash, and Young", want: "crosby-stills-nash-and-young"},
		{name: "Quotes", in: `Nat "King" Cole`, want: "nat-king-cole"},
		{name: "SmartApostrophe", in: "My Wife’s An Angel", want: "my-wifes-an-angel"},
		// Separator collapsing
		{name: "Slashes", in: "AC/DC", want: "ac-dc"},
		{name: "DotWithSpace", in: "T . S . O . L .", want: "t-s-o-l"},
		{name: "TrailingDashAndSpaces", in: "Van Halen - ", want: "van-halen"},
		{name: "MultipleDashes", in: "blink--182", want: "blink-182"},
		// transliterations and expansions
		{name: "Ampersand", in: "King Gizzard & The Lizard Wizard", want: "king-gizzard-and-the-lizard-wizard"},
		{name: "NonDecomposingLetter", in: "GØGGS", want: "goggs"},
		// Diacritics/accents
		{name: "Accents", in: "Les Rallizes Dénudés", want: "les-rallizes-denudes"},
		{name: "Umlauts", in: "Mötley Crüe", want: "motley-crue"},
	}
	for _, tc := range tests {
		if got := NormalizeArtistName(tc.in); got != tc.want {
			t.Errorf("NormalizeArtistName(%q) = %q, want = %q", tc.in, got, tc.want)
		}
	}
}
