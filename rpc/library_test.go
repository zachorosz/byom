package rpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/zachorosz/byom/library"
	libraryv1 "github.com/zachorosz/byom/proto/library/v1"
	"github.com/zachorosz/byom/proto/library/v1/libraryv1connect"
)

// fakeLibrary is a LibraryReader that returns canned results and
// records the arguments of the last call.
type fakeLibrary struct {
	artists []library.Artist
	albums  []library.Album
	tracks  []library.Track
	next    string
	err     error

	gotToken   string
	gotLimit   int
	gotQuery   library.AlbumQuery
	gotAlbumID uuid.UUID
}

func (f *fakeLibrary) Artist(_ context.Context, _ uuid.UUID) (library.Artist, error) {
	if f.err != nil {
		return library.Artist{}, f.err
	}
	return f.artists[0], nil
}

func (f *fakeLibrary) Artists(_ context.Context, token string, limit int) ([]library.Artist, string, error) {
	f.gotToken, f.gotLimit = token, limit
	return f.artists, f.next, f.err
}

func (f *fakeLibrary) Album(_ context.Context, _ uuid.UUID) (library.Album, error) {
	if f.err != nil {
		return library.Album{}, f.err
	}
	return f.albums[0], nil
}

func (f *fakeLibrary) Albums(_ context.Context, query library.AlbumQuery, token string, limit int) ([]library.Album, string, error) {
	f.gotQuery, f.gotToken, f.gotLimit = query, token, limit
	return f.albums, f.next, f.err
}

func (f *fakeLibrary) AlbumVersions(_ context.Context, id uuid.UUID) ([]library.Album, error) {
	f.gotAlbumID = id
	return f.albums, f.err
}

func (f *fakeLibrary) Track(_ context.Context, _ uuid.UUID) (library.Track, error) {
	if f.err != nil {
		return library.Track{}, f.err
	}
	return f.tracks[0], nil
}

func (f *fakeLibrary) Tracks(_ context.Context, albumID uuid.UUID, token string, limit int) ([]library.Track, string, error) {
	f.gotAlbumID, f.gotToken, f.gotLimit = albumID, token, limit
	return f.tracks, f.next, f.err
}

// newTestClient serves store over a local HTTP server, exercising the
// real interceptor stack and codec, and returns a client for it.
func newTestClient(t *testing.T, store LibraryReader) libraryv1connect.LibraryServiceClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(NewHandler(logger, NewLibraryServer(store), NewManagementServer(nil, nil)))
	t.Cleanup(srv.Close)
	return libraryv1connect.NewLibraryServiceClient(srv.Client(), srv.URL)
}

func TestLibraryServer_GetArtist(t *testing.T) {
	artist := library.Artist{ID: uuid.Must(uuid.NewV7()), Name: "Artist A", SortName: "Artist A"}
	client := newTestClient(t, &fakeLibrary{artists: []library.Artist{artist}})

	req := &libraryv1.GetArtistRequest{Id: artist.ID.String()}
	got, err := client.GetArtist(context.Background(), req)
	if err != nil {
		t.Fatalf("GetArtist(ctx, %v) failed: %v", req, err)
	}

	want := &libraryv1.GetArtistResponse{
		Artist: &libraryv1.Artist{Id: artist.ID.String(), Name: "Artist A"},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("GetArtist(ctx, %v) mismatch (-want +got):\n%s", req, diff)
	}
}

func TestLibraryServer_GetArtist_Errors(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		store *fakeLibrary
		want  connect.Code
	}{
		{
			name:  "missingID",
			id:    "",
			store: &fakeLibrary{},
			want:  connect.CodeInvalidArgument,
		},
		{
			name:  "malformedID",
			id:    "not-a-uuid",
			store: &fakeLibrary{},
			want:  connect.CodeInvalidArgument,
		},
		{
			name:  "unknownArtist",
			id:    uuid.Must(uuid.NewV7()).String(),
			store: &fakeLibrary{err: library.ErrNotFound},
			want:  connect.CodeNotFound,
		},
		{
			name:  "storeFailure",
			id:    uuid.Must(uuid.NewV7()).String(),
			store: &fakeLibrary{err: errors.New("disk on fire")},
			want:  connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.store)

			req := &libraryv1.GetArtistRequest{Id: tt.id}
			_, err := client.GetArtist(context.Background(), req)
			if got := connect.CodeOf(err); got != tt.want {
				t.Errorf("GetArtist(ctx, %v) code = %v, want %v (err: %v)", req, got, tt.want, err)
			}
		})
	}
}

func TestLibraryServer_ListArtists_PassesPage(t *testing.T) {
	store := &fakeLibrary{next: "next-token"}
	client := newTestClient(t, store)

	req := &libraryv1.ListArtistsRequest{PageSize: 7, PageToken: "page-token"}
	got, err := client.ListArtists(context.Background(), req)
	if err != nil {
		t.Fatalf("ListArtists(ctx, %v) failed: %v", req, err)
	}
	if got.GetNextPageToken() != "next-token" {
		t.Errorf("ListArtists(ctx, %v) next page token = %q, want %q", req, got.GetNextPageToken(), "next-token")
	}
	if store.gotToken != "page-token" || store.gotLimit != 7 {
		t.Errorf("Artists(%q, %d), want Artists(%q, %d)",
			store.gotToken, store.gotLimit, "page-token", 7)
	}
}

func TestLibraryServer_ListAlbums_PassesArtistFilter(t *testing.T) {
	artistID := uuid.Must(uuid.NewV7())
	album := library.Album{
		ID:    uuid.Must(uuid.NewV7()),
		Title: "Album One",
		Type:  library.AlbumMain,
		Artists: []library.AlbumArtist{
			{ArtistID: artistID, CreditedName: "Artist A"},
		},
	}
	store := &fakeLibrary{albums: []library.Album{album}}
	client := newTestClient(t, store)

	req := &libraryv1.ListAlbumsRequest{ArtistId: artistID.String()}
	got, err := client.ListAlbums(context.Background(), req)
	if err != nil {
		t.Fatalf("ListAlbums(ctx, %v) failed: %v", req, err)
	}
	if store.gotQuery.ArtistID != artistID {
		t.Errorf("Albums() artist filter = %v, want %v", store.gotQuery.ArtistID, artistID)
	}

	want := &libraryv1.ListAlbumsResponse{
		Items: []*libraryv1.Album{{
			Id:        album.ID.String(),
			Title:     "Album One",
			AlbumType: libraryv1.AlbumType_ALBUM_TYPE_MAIN,
			Artists: []*libraryv1.AlbumArtist{
				{ArtistId: artistID.String(), CreditedName: "Artist A"},
			},
		}},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("ListAlbums(ctx, %v) mismatch (-want +got):\n%s", req, diff)
	}
}

func TestLibraryServer_ListAlbums_OmittedArtistFilter(t *testing.T) {
	store := &fakeLibrary{}
	client := newTestClient(t, store)

	req := &libraryv1.ListAlbumsRequest{}
	if _, err := client.ListAlbums(context.Background(), req); err != nil {
		t.Fatalf("ListAlbums(ctx, %v) failed: %v", req, err)
	}
	if store.gotQuery.ArtistID != uuid.Nil {
		t.Errorf("Albums() artist filter = %v, want uuid.Nil", store.gotQuery.ArtistID)
	}
}

func TestLibraryServer_ListAlbums_VersionFilter(t *testing.T) {
	tests := []struct {
		name               string
		includeAllVersions bool
	}{
		{name: "omitted", includeAllVersions: false},
		{name: "requested", includeAllVersions: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeLibrary{}
			client := newTestClient(t, store)

			req := &libraryv1.ListAlbumsRequest{IncludeAllVersions: tt.includeAllVersions}
			if _, err := client.ListAlbums(context.Background(), req); err != nil {
				t.Fatalf("ListAlbums(include_all_versions=%v) failed: %v", tt.includeAllVersions, err)
			}
			got := store.gotQuery.IncludeAllVersions
			if got != tt.includeAllVersions {
				t.Errorf("ListAlbums(include_all_versions=%v) filter = %v, want %v",
					tt.includeAllVersions, got, tt.includeAllVersions)
			}
		})
	}
}

func TestLibraryServer_ListAlbums_TypeAndBootlegFilters(t *testing.T) {
	tests := []struct {
		name string
		req  *libraryv1.ListAlbumsRequest
		want library.AlbumQuery
	}{
		{
			name: "noFilters",
			req:  &libraryv1.ListAlbumsRequest{},
			want: library.AlbumQuery{},
		},
		{
			name: "singleType",
			req: &libraryv1.ListAlbumsRequest{
				AlbumTypes: []libraryv1.AlbumType{libraryv1.AlbumType_ALBUM_TYPE_MAIN},
			},
			want: library.AlbumQuery{Types: []library.AlbumType{library.AlbumMain}},
		},
		{
			name: "untaggedType",
			req: &libraryv1.ListAlbumsRequest{
				AlbumTypes: []libraryv1.AlbumType{libraryv1.AlbumType_ALBUM_TYPE_UNSPECIFIED},
			},
			want: library.AlbumQuery{Types: []library.AlbumType{library.AlbumTypeUnknown}},
		},
		{
			// Sorted and deduplicated so that two requests for the same
			// set of types share one page token.
			name: "unorderedDuplicateTypes",
			req: &libraryv1.ListAlbumsRequest{
				AlbumTypes: []libraryv1.AlbumType{
					libraryv1.AlbumType_ALBUM_TYPE_SINGLE,
					libraryv1.AlbumType_ALBUM_TYPE_EP,
					libraryv1.AlbumType_ALBUM_TYPE_SINGLE,
				},
			},
			want: library.AlbumQuery{Types: []library.AlbumType{library.AlbumEP, library.AlbumSingle}},
		},
		{
			name: "excludeBootlegs",
			req:  &libraryv1.ListAlbumsRequest{Bootlegs: libraryv1.BootlegFilter_BOOTLEG_FILTER_EXCLUDE},
			want: library.AlbumQuery{Bootlegs: library.BootlegsExclude},
		},
		{
			name: "onlyBootlegs",
			req:  &libraryv1.ListAlbumsRequest{Bootlegs: libraryv1.BootlegFilter_BOOTLEG_FILTER_ONLY},
			want: library.AlbumQuery{Bootlegs: library.BootlegsOnly},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeLibrary{}
			client := newTestClient(t, store)

			if _, err := client.ListAlbums(context.Background(), tt.req); err != nil {
				t.Fatalf("ListAlbums() failed: %v", err)
			}

			if diff := cmp.Diff(tt.want, store.gotQuery); diff != "" {
				t.Errorf("ListAlbums() filter mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLibraryServer_ListAlbums_Ordering(t *testing.T) {
	tests := []struct {
		name string
		req  *libraryv1.ListAlbumsRequest
		want library.AlbumQuery
	}{
		{
			name: "unspecifiedIsTitle",
			req:  &libraryv1.ListAlbumsRequest{},
			want: library.AlbumQuery{Order: library.AlbumOrderTitle},
		},
		{
			name: "explicitTitle",
			req:  &libraryv1.ListAlbumsRequest{Order: libraryv1.AlbumOrder_ALBUM_ORDER_TITLE},
			want: library.AlbumQuery{Order: library.AlbumOrderTitle},
		},
		{
			name: "artist",
			req:  &libraryv1.ListAlbumsRequest{Order: libraryv1.AlbumOrder_ALBUM_ORDER_ARTIST},
			want: library.AlbumQuery{Order: library.AlbumOrderArtist},
		},
		{
			name: "recentlyAddedDescending",
			req: &libraryv1.ListAlbumsRequest{
				Order:      libraryv1.AlbumOrder_ALBUM_ORDER_RECENTLY_ADDED,
				Descending: true,
			},
			want: library.AlbumQuery{Order: library.AlbumOrderRecentlyAdded, Descending: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeLibrary{}
			client := newTestClient(t, store)

			if _, err := client.ListAlbums(context.Background(), tt.req); err != nil {
				t.Fatalf("ListAlbums() failed: %v", err)
			}
			if diff := cmp.Diff(tt.want, store.gotQuery); diff != "" {
				t.Errorf("ListAlbums() query mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLibraryServer_ListAlbums_UnknownAlbumOrder(t *testing.T) {
	client := newTestClient(t, &fakeLibrary{})

	req := &libraryv1.ListAlbumsRequest{Order: libraryv1.AlbumOrder(99)}
	_, err := client.ListAlbums(context.Background(), req)
	if got, want := connect.CodeOf(err), connect.CodeInvalidArgument; got != want {
		t.Errorf("ListAlbums(order=99) code = %v, want %v (err: %v)", got, want, err)
	}
}

func TestLibraryServer_ListAlbums_UnknownAlbumType(t *testing.T) {
	client := newTestClient(t, &fakeLibrary{})

	// An enum value this server does not know must not silently widen
	// the listing to every type.
	req := &libraryv1.ListAlbumsRequest{AlbumTypes: []libraryv1.AlbumType{99}}
	_, err := client.ListAlbums(context.Background(), req)
	if got, want := connect.CodeOf(err), connect.CodeInvalidArgument; got != want {
		t.Errorf("ListAlbums(album_types=[99]) code = %v, want %v (err: %v)", got, want, err)
	}
}

func TestLibraryServer_ListAlbumVersions(t *testing.T) {
	artistID := uuid.Must(uuid.NewV7())
	primary := library.Album{
		ID:             uuid.Must(uuid.NewV7()),
		Title:          "Album One",
		Type:           library.AlbumMain,
		PrimaryVersion: true,
		Artists: []library.AlbumArtist{
			{ArtistID: artistID, CreditedName: "Artist A"},
		},
	}
	remaster := library.Album{
		ID:      uuid.Must(uuid.NewV7()),
		Title:   "Album One",
		Type:    library.AlbumMain,
		Version: "2011 Remaster",
		Artists: []library.AlbumArtist{
			{ArtistID: artistID, CreditedName: "Artist A"},
		},
	}
	store := &fakeLibrary{albums: []library.Album{primary, remaster}}
	client := newTestClient(t, store)

	got, err := client.ListAlbumVersions(context.Background(),
		&libraryv1.ListAlbumVersionsRequest{AlbumId: remaster.ID.String()})
	if err != nil {
		t.Fatalf("ListAlbumVersions(%v) failed: %v", remaster.ID, err)
	}
	if store.gotAlbumID != remaster.ID {
		t.Errorf("ListAlbumVersions(%v) album = %v, want %v", remaster.ID, store.gotAlbumID, remaster.ID)
	}

	artists := []*libraryv1.AlbumArtist{
		{ArtistId: artistID.String(), CreditedName: "Artist A"},
	}
	want := &libraryv1.ListAlbumVersionsResponse{
		Items: []*libraryv1.Album{
			{
				Id:             primary.ID.String(),
				Title:          "Album One",
				AlbumType:      libraryv1.AlbumType_ALBUM_TYPE_MAIN,
				PrimaryVersion: true,
				Artists:        artists,
			},
			{
				Id:        remaster.ID.String(),
				Title:     "Album One",
				AlbumType: libraryv1.AlbumType_ALBUM_TYPE_MAIN,
				Version:   "2011 Remaster",
				Artists:   artists,
			},
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("ListAlbumVersions(%v) mismatch (-want +got):\n%s", remaster.ID, diff)
	}
}

func TestLibraryServer_ListAlbumVersions_Errors(t *testing.T) {
	tests := []struct {
		name    string
		albumID string
		store   *fakeLibrary
		want    connect.Code
	}{
		{
			name:    "missingID",
			albumID: "",
			store:   &fakeLibrary{},
			want:    connect.CodeInvalidArgument,
		},
		{
			name:    "malformedID",
			albumID: "not-a-uuid",
			store:   &fakeLibrary{},
			want:    connect.CodeInvalidArgument,
		},
		{
			name:    "unknownAlbum",
			albumID: uuid.Must(uuid.NewV7()).String(),
			store:   &fakeLibrary{err: library.ErrNotFound},
			want:    connect.CodeNotFound,
		},
		{
			name:    "storeFailure",
			albumID: uuid.Must(uuid.NewV7()).String(),
			store:   &fakeLibrary{err: errors.New("disk on fire")},
			want:    connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.store)

			_, err := client.ListAlbumVersions(context.Background(),
				&libraryv1.ListAlbumVersionsRequest{AlbumId: tt.albumID})
			if got := connect.CodeOf(err); got != tt.want {
				t.Errorf("ListAlbumVersions(%q) code = %v, want %v (err: %v)", tt.albumID, got, tt.want, err)
			}
		})
	}
}

func TestLibraryServer_ListTracks(t *testing.T) {
	albumID := uuid.Must(uuid.NewV7())
	artistID := uuid.Must(uuid.NewV7())
	track := library.Track{
		ID:          uuid.Must(uuid.NewV7()),
		AlbumID:     albumID,
		Title:       "Track One",
		DiscNumber:  1,
		TrackNumber: 2,
		Duration:    3*time.Minute + 30*time.Second,
		Audio: library.AudioProperties{
			Codec: "flac", SampleRate: 44100, BitDepth: 16, Channels: 2, Bitrate: 1000,
		},
		Credits: []library.TrackCredit{
			{ArtistID: artistID, CreditedName: "Artist A", Role: "Performer"},
		},
	}
	store := &fakeLibrary{tracks: []library.Track{track}}
	client := newTestClient(t, store)

	req := &libraryv1.ListTracksRequest{AlbumId: albumID.String()}
	got, err := client.ListTracks(context.Background(), req)
	if err != nil {
		t.Fatalf("ListTracks(ctx, %v) failed: %v", req, err)
	}
	if store.gotAlbumID != albumID {
		t.Errorf("Tracks() album filter = %v, want %v", store.gotAlbumID, albumID)
	}

	want := &libraryv1.ListTracksResponse{
		Items: []*libraryv1.Track{{
			Id:          track.ID.String(),
			AlbumId:     albumID.String(),
			Title:       "Track One",
			DiscNumber:  1,
			TrackNumber: 2,
			Duration:    durationpb.New(3*time.Minute + 30*time.Second),
			Audio: &libraryv1.AudioProperties{
				Codec: "flac", SampleRate: 44100, BitDepth: 16, Channels: 2, Bitrate: 1000,
			},
			Credits: []*libraryv1.TrackCredit{
				{ArtistId: artistID.String(), CreditedName: "Artist A", Role: "Performer"},
			},
		}},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("ListTracks(ctx, %v) mismatch (-want +got):\n%s", req, diff)
	}
}
