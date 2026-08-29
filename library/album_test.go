package library

import (
	"fmt"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

func TestAlbumGroupKey(t *testing.T) {
	uuid1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uuid2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	uuidNil := uuid.Nil

	expectedHash := func(raw string) string {
		sum := xxhash.Sum64String(raw)
		return fmt.Sprintf("%x", sum)
	}

	tests := []struct {
		name    string
		album   Album
		want    string
		wantErr bool
	}{
		{
			name: "ArtistWithTitle",
			album: Album{
				Artists: []AlbumArtist{{ArtistID: uuid1}},
				Title:   "In Your Mind Fuzz",
			},
			want: expectedHash(uuid1.String() + "\x00" + "In Your Mind Fuzz"),
		},
		{
			name: "MultipleArtists",
			album: Album{
				Artists: []AlbumArtist{{ArtistID: uuid1}, {ArtistID: uuid2}},
				Title:   "Split EP",
			},
			want: expectedHash(uuid1.String() + "/" + uuid2.String() + "\x00" + "Split EP"),
		},
		{
			name: "EmptyTitle",
			album: Album{
				Artists: []AlbumArtist{{ArtistID: uuid1}},
				Title:   "",
			},
			want: expectedHash(uuid1.String() + "\x00" + "[Unknown Album]"),
		},
		{
			name: "NilArtistUUID",
			album: Album{
				Artists: []AlbumArtist{{ArtistID: uuidNil}},
				Title:   "In Your Mind Fuzz",
			},
			wantErr: true,
		},
		{
			name: "EmptyArtists",
			album: Album{
				Artists: []AlbumArtist{},
				Title:   "Ghost Album",
			},
			want: expectedHash("\x00" + "Ghost Album"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AlbumGroupKey(tc.album)
			if (err != nil) != tc.wantErr {
				t.Fatalf("AlbumGroupKey() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("AlbumGroupKey() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAlbumGroupKey_Matching(t *testing.T) {
	uuid1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uuid2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name        string
		albumA      Album
		albumB      Album
		shouldMatch bool
	}{
		{
			name:        "ExactMatches",
			albumA:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: "In Your Mind Fuzz"},
			albumB:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: "In Your Mind Fuzz"},
			shouldMatch: true,
		},
		{
			name:        "DifferentTitles",
			albumA:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: "In Your Mind Fuzz"},
			albumB:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: "Nonagon Infinity"},
			shouldMatch: false,
		},
		{
			name:        "DifferentArtists",
			albumA:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: "Greatest Hits"},
			albumB:      Album{Artists: []AlbumArtist{{ArtistID: uuid2}}, Title: "Greatest Hits"},
			shouldMatch: false,
		},
		{
			name:        "EmptyTitle",
			albumA:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: ""},
			albumB:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}}, Title: "[Unknown Album]"},
			shouldMatch: true,
		},
		{
			name:        "DifferentArtistOrder",
			albumA:      Album{Artists: []AlbumArtist{{ArtistID: uuid1}, {ArtistID: uuid2}}, Title: "Split EP"},
			albumB:      Album{Artists: []AlbumArtist{{ArtistID: uuid2}, {ArtistID: uuid1}}, Title: "Split EP"},
			shouldMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyA, err := AlbumGroupKey(tc.albumA)
			if err != nil {
				t.Fatalf("AlbumGroupKey(albumA) failed: %v", err)
			}
			keyB, err := AlbumGroupKey(tc.albumB)
			if err != nil {
				t.Fatalf("AlbumGroupKey(albumB) failed: %v", err)
			}

			matched := keyA == keyB

			if matched != tc.shouldMatch {
				t.Errorf("AlbumGroupKey(albumA) == AlbumGroupKey(albumB) is %v, want %v", matched, tc.shouldMatch)
			}
		})
	}
}

func TestAlbumQuery_Equal(t *testing.T) {
	artistA := uuid.Must(uuid.NewV7())
	artistB := uuid.Must(uuid.NewV7())

	tests := []struct {
		name string
		a    AlbumQuery
		b    AlbumQuery
		want bool
	}{
		{
			name: "zeroValues",
			a:    AlbumQuery{},
			b:    AlbumQuery{},
			want: true,
		},
		{
			name: "sameEveryField",
			a:    AlbumQuery{ArtistID: artistA, IncludeAllVersions: true, Types: []AlbumType{AlbumMain, AlbumEP}, Bootlegs: BootlegsOnly},
			b:    AlbumQuery{ArtistID: artistA, IncludeAllVersions: true, Types: []AlbumType{AlbumMain, AlbumEP}, Bootlegs: BootlegsOnly},
			want: true,
		},
		{
			name: "differentArtist",
			a:    AlbumQuery{ArtistID: artistA},
			b:    AlbumQuery{ArtistID: artistB},
			want: false,
		},
		{
			name: "differentVersions",
			a:    AlbumQuery{IncludeAllVersions: true},
			b:    AlbumQuery{},
			want: false,
		},
		{
			name: "differentBootlegs",
			a:    AlbumQuery{Bootlegs: BootlegsExclude},
			b:    AlbumQuery{Bootlegs: BootlegsOnly},
			want: false,
		},
		{
			name: "differentTypes",
			a:    AlbumQuery{Types: []AlbumType{AlbumMain}},
			b:    AlbumQuery{Types: []AlbumType{AlbumSingle}},
			want: false,
		},
		{
			name: "extraType",
			a:    AlbumQuery{Types: []AlbumType{AlbumMain}},
			b:    AlbumQuery{Types: []AlbumType{AlbumMain, AlbumEP}},
			want: false,
		},
		{
			name: "noTypesVersusSomeTypes",
			a:    AlbumQuery{},
			b:    AlbumQuery{Types: []AlbumType{AlbumMain}},
			want: false,
		},
		{
			// Equal is strict: callers normalize before comparing, so a
			// reordered list is a different filter here.
			name: "reorderedTypes",
			a:    AlbumQuery{Types: []AlbumType{AlbumMain, AlbumEP}},
			b:    AlbumQuery{Types: []AlbumType{AlbumEP, AlbumMain}},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("AlbumQuery.Equal(%+v, %+v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
