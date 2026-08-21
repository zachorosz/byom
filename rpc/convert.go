package rpc

import (
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/zachorosz/byom/library"
	libraryv1 "github.com/zachorosz/byom/proto/library/v1"
)

var albumTypes = map[library.AlbumType]libraryv1.AlbumType{
	library.AlbumTypeUnknown: libraryv1.AlbumType_ALBUM_TYPE_UNSPECIFIED,
	library.AlbumMain:        libraryv1.AlbumType_ALBUM_TYPE_MAIN,
	library.AlbumSingle:      libraryv1.AlbumType_ALBUM_TYPE_SINGLE,
	library.AlbumEP:          libraryv1.AlbumType_ALBUM_TYPE_EP,
	library.AlbumOther:       libraryv1.AlbumType_ALBUM_TYPE_OTHER,
}

func artistProto(a library.Artist) *libraryv1.Artist {
	return &libraryv1.Artist{
		Id:   a.ID.String(),
		Name: a.Name,
	}
}

func albumProto(al library.Album) *libraryv1.Album {
	pb := &libraryv1.Album{
		Id:                  al.ID.String(),
		Title:               al.Title,
		AlbumType:           albumTypes[al.Type],
		ReleaseDate:         al.ReleaseDate,
		OriginalReleaseDate: al.OriginalReleaseDate,
		ReleaseCountry:      al.ReleaseCountry,
		Bootleg:             al.Bootleg,
		Compilation:         al.Compilation,
		Live:                al.Live,
		GroupKey:            al.GroupKey,
		Version:             al.Version,
		PrimaryVersion:      al.PrimaryVersion,
	}
	for _, aa := range al.Artists {
		pb.Artists = append(pb.Artists, &libraryv1.AlbumArtist{
			ArtistId:     aa.ArtistID.String(),
			CreditedName: aa.CreditedName,
		})
	}
	return pb
}

func trackProto(t library.Track) *libraryv1.Track {
	pb := &libraryv1.Track{
		Id:                  t.ID.String(),
		AlbumId:             t.AlbumID.String(),
		Title:               t.Title,
		DiscNumber:          int32(t.DiscNumber),
		DiscSubtitle:        t.DiscSubtitle,
		TrackNumber:         int32(t.TrackNumber),
		ReleaseDate:         t.ReleaseDate,
		OriginalReleaseDate: t.OriginalReleaseDate,
		Duration:            durationpb.New(t.Duration),
		Audio: &libraryv1.AudioProperties{
			Codec:      t.Audio.Codec,
			SampleRate: int64(t.Audio.SampleRate),
			BitDepth:   int64(t.Audio.BitDepth),
			Bitrate:    int64(t.Audio.Bitrate),
			Channels:   int64(t.Audio.Channels),
		},
	}
	for _, c := range t.Credits {
		pb.Credits = append(pb.Credits, &libraryv1.TrackCredit{
			ArtistId:     c.ArtistID.String(),
			CreditedName: c.CreditedName,
			Role:         c.Role,
		})
	}
	return pb
}
