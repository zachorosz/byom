package library

import "testing"

func TestNormalizeArtistName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "Base",
			in:   "Thee Oh Sees",
			want: "thee-oh-sees",
		},
		{
			// And and & should match
			name: "Ampersand",
			in:   "King Gizzard & The Lizard Wizard",
			want: "king-gizzard-and-the-lizard-wizard",
		},
		{
			// Forgetting a comma should match
			name: "Comma",
			in:   "Tyler, The Creator",
			want: "tyler-the-creator",
		},
		{
			name: "Accents",
			in:   "Les Rallizes Dénudés",
			want: "les-rallizes-denudes",
		},
		{
			name: "Umlauts",
			in:   "Mötley Crüe",
			want: "motley-crue",
		},
		{
			// Missing a trailing . should match
			name: "Abbreviation",
			in:   "T.S.O.L.",
			want: "t-s-o-l",
		},
		{
			name: "DotWithSpace",
			in:   "Dr. Dre",
			want: "dr-dre",
		},
		{
			name: "Slashes",
			in:   "AC/DC",
			want: "ac-dc",
		},
		{
			name: "MultipleSpaces",
			in:   "Crosby,   Stills,   Nash & Young",
			want: "crosby-stills-nash-and-young",
		},
		{name: "Empty", in: "", want: ""},
	}
	for _, tc := range tests {
		if got := NormalizeArtistName(tc.in); got != tc.want {
			t.Errorf("NormalizeArtistName(%q) = %q, want = %q", tc.in, got, tc.want)
		}
	}
}
