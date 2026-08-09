package metadata

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zachorosz/byom/library"
)

func TestSplitArtists(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "Single", in: "Black Flag", want: []string{"Black Flag"}},
		{name: "Semicolon", in: "The Faith; Void", want: []string{"The Faith", "Void"}},
		{name: "SemicolonNoSpace", in: "The Faith;Void", want: []string{"The Faith", "Void"}},
		{name: "SlashWithSpaces", in: "The Faith / Void", want: []string{"The Faith", "Void"}},
		{name: "SlashWithoutSpacesKept", in: "AC/DC", want: []string{"AC/DC"}},
		{name: "Multiple", in: "AC/DC; The Faith / Void", want: []string{"AC/DC", "The Faith", "Void"}},
		{name: "Empty", in: "", want: nil},
		{name: "SemicolonOnly", in: ";", want: nil},
		{name: "SlashOnly", in: " / ", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitArtists(tc.in)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("splitArtists(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

func TestMapCredits_SplitsRoleTags(t *testing.T) {
	tags := map[string][]string{
		"COMPOSER": {"Stu Mackenzie; Joey Walker"},
	}

	got := mapCredits(tags)

	want := []Credit{
		{CreditedName: "Stu Mackenzie", Role: "Composer"},
		{CreditedName: "Joey Walker", Role: "Composer"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mapCredits(%v) mismatch (-want +got):\n%s", tags, diff)
	}
}

func TestParseCredit(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   Credit
		wantOK bool
	}{
		{
			name:   "StandardFormat",
			in:     "Eddie Van Halen - Guitar",
			want:   Credit{CreditedName: "Eddie Van Halen", Role: "Guitar"},
			wantOK: true,
		},
		{
			name:   "ExtraWhitespace",
			in:     "  Eddie Van Halen   -   Guitar  ",
			want:   Credit{CreditedName: "Eddie Van Halen", Role: "Guitar"},
			wantOK: true,
		},
		{
			name:   "MultipleSeparators",
			in:     "Eddie Van Halen - Guitar - Solo",
			want:   Credit{CreditedName: "Eddie Van Halen", Role: "Guitar - Solo"},
			wantOK: true,
		},
		{
			name:   "MissingSeparator",
			in:     "Eddie Van Halen Guitar",
			want:   Credit{},
			wantOK: false,
		},
		{
			name:   "SeparatorWithoutSpaces",
			in:     "Eddie Van Halen-Guitar",
			want:   Credit{},
			wantOK: false,
		},
		{name: "Empty", in: "", want: Credit{}, wantOK: false},
		{name: "EmptyName", in: " - Guitar", want: Credit{}, wantOK: false},
		{name: "EmptyRole", in: "Eddie Van Halen - ", want: Credit{}, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotOK := parseCredit(tc.in)
			if gotOK != tc.wantOK {
				t.Errorf("parseCredit(%q) ok = %v, wantOK = %v", tc.in, gotOK, tc.wantOK)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("parseCredit(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

func TestMapAlbumType_Mapping(t *testing.T) {
	tests := []struct {
		value string
		want  library.AlbumType
	}{
		{value: "album", want: library.AlbumMain},
		{value: "Album", want: library.AlbumMain}, // case-insensitive
		{value: "live", want: library.AlbumMain},
		{value: "compilation", want: library.AlbumMain},
		{value: "single", want: library.AlbumSingle},
		{value: "ep", want: library.AlbumEP},
		{value: "extendedplay", want: library.AlbumEP},
		{value: "other", want: library.AlbumOther},
		{value: "demo", want: library.AlbumTypeUnknown},
	}
	for _, tc := range tests {
		tags := map[string][]string{
			"RELEASETYPE": {tc.value},
		}
		if got := mapAlbumType(tags); got != tc.want {
			t.Errorf("%q = %v, want = %v", tc.value, got, tc.want)
		}
	}
}

func TestMapAlbumType_FirstValue(t *testing.T) {
	tags := map[string][]string{
		"RELEASETYPE": {"album", "ep"},
	}
	if got, want := mapAlbumType(tags), library.AlbumMain; got != want {
		t.Errorf("mapAlbumType() = %v, want = %v", got, want)
	}
}
