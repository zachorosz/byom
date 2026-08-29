package metadata

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zachorosz/byom/library"
)

func TestNormTags(t *testing.T) {
	tests := []struct {
		name string
		in   map[string][]string
		want map[string][]string
	}{
		{
			name: "SplittableTagSplits",
			in:   map[string][]string{"artist": {"The Faith; Void"}},
			want: map[string][]string{"ARTIST": {"The Faith", "Void"}},
		},
		{
			name: "NonSplittableTagKeepsDelimiters",
			in:   map[string][]string{"album": {"Kid A; Amnesiac"}},
			want: map[string][]string{"ALBUM": {"Kid A; Amnesiac"}},
		},
		{
			name: "UnknownTagDefaultsToNoSplit",
			in:   map[string][]string{"mood": {"Dark; Moody"}},
			want: map[string][]string{"MOOD": {"Dark; Moody"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normTags(tc.in)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("normTags(%v) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
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

func TestMapCredits(t *testing.T) {
	tests := []struct {
		name string
		tags map[string][]string
		want []Credit
	}{
		{
			name: "ArtistSplitOnSlash",
			tags: map[string][]string{"ARTIST": {"Artist A / Artist B"}},
			want: []Credit{
				{CreditedName: "Artist A", Role: "Performer"},
				{CreditedName: "Artist B", Role: "Performer"},
			},
		},
		{
			name: "ArtistSlashInNameKept",
			tags: map[string][]string{"ARTIST": {"AC/DC"}},
			want: []Credit{{CreditedName: "AC/DC", Role: "Performer"}},
		},
		{
			name: "RoleTagNotSplitOnSlash",
			tags: map[string][]string{"COMPOSER": {"Foo / Bar"}},
			want: []Credit{{CreditedName: "Foo / Bar", Role: "Composer"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapCredits(tc.tags)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mapCredits(%v) mismatch (-want +got):\n%s", tc.tags, diff)
			}
		})
	}
}

func TestMapAlbum(t *testing.T) {
	tests := []struct {
		name string
		tags map[string][]string
		want AlbumMetadata
	}{
		{
			name: "Bootleg",
			tags: map[string][]string{
				"RELEASESTATUS": {"bootleg"},
			},
			want: AlbumMetadata{Bootleg: true},
		},
		{
			name: "Compilation",
			tags: map[string][]string{
				"RELEASETYPE": {"Compilation"},
			},
			want: AlbumMetadata{Type: library.AlbumMain, Compilation: true},
		},
		{
			name: "CompilationFlag",
			tags: map[string][]string{
				"COMPILATION": {"1"},
			},
			want: AlbumMetadata{Type: library.AlbumMain, Compilation: true},
		},
		{
			name: "Live",
			tags: map[string][]string{
				"RELEASETYPE": {"LiVe"},
			},
			want: AlbumMetadata{Type: library.AlbumMain, Live: true},
		},
		{
			name: "LiveCompilation",
			tags: map[string][]string{
				"RELEASETYPE": {"live", "compilation"},
			},
			want: AlbumMetadata{Type: library.AlbumMain, Live: true, Compilation: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapAlbum(tc.tags)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mapAlbum(%v) mismatch (-want +got):\n%s", tc.tags, diff)
			}
		})
	}
}

func TestMapAlbumArtists(t *testing.T) {
	tests := []struct {
		name string
		tags map[string][]string
		want []Credit
	}{
		{
			name: "AlbumArtistSplitOnSlash",
			tags: map[string][]string{"ALBUMARTIST": {"Artist A / Artist B"}},
			want: []Credit{{CreditedName: "Artist A"}, {CreditedName: "Artist B"}},
		},
		{
			name: "FallsBackToSingleArtist",
			tags: map[string][]string{"ARTIST": {"Black Flag"}},
			want: []Credit{{CreditedName: "Black Flag"}},
		},
		{
			name: "MultipleArtistsFallbackReturnsEmpty",
			tags: map[string][]string{"ARTIST": {"Artist A", "Artist B"}},
			want: []Credit{},
		},
		{
			name: "SplitArtistsFallbackReturnsEmpty",
			tags: map[string][]string{"ARTIST": {"Artist A / Artist B"}},
			want: []Credit{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapAlbumArtists(tc.tags)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mapAlbumArtists(%v) mismatch (-want +got):\n%s", tc.tags, diff)
			}
		})
	}
}

func TestMapAlbumType(t *testing.T) {
	tests := []struct {
		name string
		tags map[string][]string
		want library.AlbumType
	}{
		{
			name: "EmptyTags",
			tags: map[string][]string{},
			want: library.AlbumTypeUnknown,
		},
		{
			name: "Album",
			tags: map[string][]string{"RELEASETYPE": {"album"}},
			want: library.AlbumMain,
		},
		{
			name: "EP_CaseInsensitive",
			tags: map[string][]string{"RELEASETYPE": {"eXtendedPlay"}},
			want: library.AlbumEP,
		},
		{
			name: "Single",
			tags: map[string][]string{"RELEASETYPE": {"single"}},
			want: library.AlbumSingle,
		},
		{
			name: "Other",
			tags: map[string][]string{"RELEASETYPE": {"other"}},
			want: library.AlbumOther,
		},
		{
			name: "Unrecognized",
			tags: map[string][]string{"RELEASETYPE": {"notrecognized"}},
			want: library.AlbumTypeUnknown,
		},
		{
			name: "CompilationFallback",
			tags: map[string][]string{"RELEASETYPE": {"compilation"}},
			want: library.AlbumMain,
		},
		{
			name: "LiveFallback",
			tags: map[string][]string{"RELEASETYPE": {"live"}},
			want: library.AlbumMain,
		},
		{
			name: "LiveOverridden",
			tags: map[string][]string{"RELEASETYPE": {"live", "single"}},
			want: library.AlbumSingle,
		},
		{
			name: "CompilationTruthy",
			tags: map[string][]string{"COMPILATION": {"1"}},
			want: library.AlbumMain,
		},
		{
			name: "CompilationFalsy",
			tags: map[string][]string{"COMPILATION": {"0"}},
			want: library.AlbumTypeUnknown,
		},
		{
			name: "ReleaseTypePriority",
			tags: map[string][]string{
				"RELEASETYPE": {"ep"},
				"COMPILATION": {"1"},
			},
			want: library.AlbumEP,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapAlbumType(tc.tags)
			if got != tc.want {
				t.Errorf("mapAlbumType(%v) = %v, want %v", tc.tags, got, tc.want)
			}
		})
	}
}
