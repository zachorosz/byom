package rpc

import (
	"context"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
	libraryv1 "github.com/zachorosz/byom/proto/library/v1"
	"github.com/zachorosz/byom/proto/library/v1/libraryv1connect"
)

type LibraryReader interface {
	Artist(ctx context.Context, id uuid.UUID) (library.Artist, error)
	Artists(ctx context.Context, token string, limit int) ([]library.Artist, string, error)
	Album(ctx context.Context, id uuid.UUID) (library.Album, error)
	Albums(ctx context.Context, filter library.AlbumFilter, token string, limit int) ([]library.Album, string, error)
	AlbumVersions(ctx context.Context, id uuid.UUID) ([]library.Album, error)
	Track(ctx context.Context, id uuid.UUID) (library.Track, error)
	Tracks(ctx context.Context, albumID uuid.UUID, token string, limit int) ([]library.Track, string, error)
}

type LibraryServer struct {
	libraryv1connect.UnimplementedLibraryServiceHandler
	store LibraryReader
}

func NewLibraryServer(store LibraryReader) *LibraryServer {
	return &LibraryServer{store: store}
}

func (s *LibraryServer) ListArtists(ctx context.Context, req *libraryv1.ListArtistsRequest) (*libraryv1.ListArtistsResponse, error) {
	artists, next, err := s.store.Artists(ctx, req.GetPageToken(), int(req.GetPageSize()))
	if err != nil {
		return nil, rpcError(err)
	}

	res := &libraryv1.ListArtistsResponse{
		Items:         make([]*libraryv1.Artist, 0, len(artists)),
		NextPageToken: next,
	}
	for _, a := range artists {
		res.Items = append(res.Items, artistProto(a))
	}
	return res, nil
}

func (s *LibraryServer) GetArtist(ctx context.Context, req *libraryv1.GetArtistRequest) (*libraryv1.GetArtistResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	artist, err := s.store.Artist(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &libraryv1.GetArtistResponse{Artist: artistProto(artist)}, nil
}

func (s *LibraryServer) ListAlbums(ctx context.Context, req *libraryv1.ListAlbumsRequest) (*libraryv1.ListAlbumsResponse, error) {
	artistID, err := parseOptionalID("artist_id", req.GetArtistId())
	if err != nil {
		return nil, err
	}

	types, err := albumTypeFilter("album_types", req.GetAlbumTypes())
	if err != nil {
		return nil, err
	}
	bootlegs, err := bootlegFilter("bootlegs", req.GetBootlegs())
	if err != nil {
		return nil, err
	}

	filter := library.AlbumFilter{
		ArtistID:           artistID,
		IncludeAllVersions: req.GetIncludeAllVersions(),
		Types:              types,
		Bootlegs:           bootlegs,
	}
	albums, next, err := s.store.Albums(ctx, filter, req.GetPageToken(), int(req.GetPageSize()))
	if err != nil {
		return nil, rpcError(err)
	}

	res := &libraryv1.ListAlbumsResponse{
		Items:         make([]*libraryv1.Album, 0, len(albums)),
		NextPageToken: next,
	}
	for _, al := range albums {
		res.Items = append(res.Items, albumProto(al))
	}
	return res, nil
}

func (s *LibraryServer) GetAlbum(ctx context.Context, req *libraryv1.GetAlbumRequest) (*libraryv1.GetAlbumResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	album, err := s.store.Album(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &libraryv1.GetAlbumResponse{Album: albumProto(album)}, nil
}

func (s *LibraryServer) ListAlbumVersions(ctx context.Context, req *libraryv1.ListAlbumVersionsRequest) (*libraryv1.ListAlbumVersionsResponse, error) {
	id, err := parseID("album_id", req.GetAlbumId())
	if err != nil {
		return nil, err
	}

	albums, err := s.store.AlbumVersions(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}

	res := &libraryv1.ListAlbumVersionsResponse{
		Items: make([]*libraryv1.Album, 0, len(albums)),
	}
	for _, al := range albums {
		res.Items = append(res.Items, albumProto(al))
	}
	return res, nil
}

func (s *LibraryServer) ListTracks(ctx context.Context, req *libraryv1.ListTracksRequest) (*libraryv1.ListTracksResponse, error) {
	albumID, err := parseOptionalID("album_id", req.GetAlbumId())
	if err != nil {
		return nil, err
	}

	tracks, next, err := s.store.Tracks(ctx, albumID, req.GetPageToken(), int(req.GetPageSize()))
	if err != nil {
		return nil, rpcError(err)
	}

	res := &libraryv1.ListTracksResponse{
		Items:         make([]*libraryv1.Track, 0, len(tracks)),
		NextPageToken: next,
	}
	for _, t := range tracks {
		res.Items = append(res.Items, trackProto(t))
	}
	return res, nil
}

func (s *LibraryServer) GetTrack(ctx context.Context, req *libraryv1.GetTrackRequest) (*libraryv1.GetTrackResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	track, err := s.store.Track(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &libraryv1.GetTrackResponse{Track: trackProto(track)}, nil
}
