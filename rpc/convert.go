package rpc

import (
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zachorosz/byom/library"
	libraryv1 "github.com/zachorosz/byom/proto/library/v1"
	managementv1 "github.com/zachorosz/byom/proto/management/v1"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

var albumTypes = map[library.AlbumType]libraryv1.AlbumType{
	library.AlbumTypeUnknown: libraryv1.AlbumType_ALBUM_TYPE_UNSPECIFIED,
	library.AlbumMain:        libraryv1.AlbumType_ALBUM_TYPE_MAIN,
	library.AlbumSingle:      libraryv1.AlbumType_ALBUM_TYPE_SINGLE,
	library.AlbumEP:          libraryv1.AlbumType_ALBUM_TYPE_EP,
	library.AlbumOther:       libraryv1.AlbumType_ALBUM_TYPE_OTHER,
}

var albumTypesByProto = func() map[libraryv1.AlbumType]library.AlbumType {
	byProto := make(map[libraryv1.AlbumType]library.AlbumType, len(albumTypes))
	for domain, pb := range albumTypes {
		byProto[pb] = domain
	}
	return byProto
}()

var bootlegFilters = map[libraryv1.BootlegFilter]library.BootlegFilter{
	libraryv1.BootlegFilter_BOOTLEG_FILTER_UNSPECIFIED: library.BootlegsInclude,
	libraryv1.BootlegFilter_BOOTLEG_FILTER_EXCLUDE:     library.BootlegsExclude,
	libraryv1.BootlegFilter_BOOTLEG_FILTER_ONLY:        library.BootlegsOnly,
}

// albumTypeFilter converts requested album types into a sorted,
// deduplicated filter so that equivalent requests share a page token.
//
// It fails with InvalidArgument on an unrecognized type rather than
// dropping it, which would silently widen the listing.
func albumTypeFilter(field string, pbTypes []libraryv1.AlbumType) ([]library.AlbumType, error) {
	if len(pbTypes) == 0 {
		return nil, nil
	}
	types := make([]library.AlbumType, 0, len(pbTypes))
	for _, pb := range pbTypes {
		t, ok := albumTypesByProto[pb]
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("%s: unknown album type %d", field, pb))
		}
		types = append(types, t)
	}
	slices.Sort(types)
	return slices.Compact(types), nil
}

var albumOrders = map[libraryv1.AlbumOrder]library.AlbumOrder{
	libraryv1.AlbumOrder_ALBUM_ORDER_UNSPECIFIED:    library.AlbumOrderTitle,
	libraryv1.AlbumOrder_ALBUM_ORDER_TITLE:          library.AlbumOrderTitle,
	libraryv1.AlbumOrder_ALBUM_ORDER_ARTIST:         library.AlbumOrderArtist,
	libraryv1.AlbumOrder_ALBUM_ORDER_RELEASE_DATE:   library.AlbumOrderReleaseDate,
	libraryv1.AlbumOrder_ALBUM_ORDER_ORIGINAL_DATE:  library.AlbumOrderOriginalDate,
	libraryv1.AlbumOrder_ALBUM_ORDER_RECENTLY_ADDED: library.AlbumOrderRecentlyAdded,
}

// albumOrder converts a requested sort order, failing with
// InvalidArgument on an unrecognized value.
func albumOrder(field string, pb libraryv1.AlbumOrder) (library.AlbumOrder, error) {
	order, ok := albumOrders[pb]
	if !ok {
		return "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s: unknown album order %d", field, pb))
	}
	return order, nil
}

// bootlegFilter converts a requested bootleg filter, failing with
// InvalidArgument on an unrecognized value.
func bootlegFilter(field string, pb libraryv1.BootlegFilter) (library.BootlegFilter, error) {
	f, ok := bootlegFilters[pb]
	if !ok {
		return "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s: unknown bootleg filter %d", field, pb))
	}
	return f, nil
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
		CoverHash:           al.CoverHash,
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

var scanStates = map[scan.State]managementv1.ScanState{
	scan.StateRunning:    managementv1.ScanState_SCAN_STATE_RUNNING,
	scan.StateCancelling: managementv1.ScanState_SCAN_STATE_CANCELLING,
	scan.StateDone:       managementv1.ScanState_SCAN_STATE_DONE,
	scan.StateFailed:     managementv1.ScanState_SCAN_STATE_FAILED,
	scan.StateAborted:    managementv1.ScanState_SCAN_STATE_ABORTED,
}

// scanState maps a requested filter to a domain state, treating the
// unspecified enum value as no filter.
func scanState(pb managementv1.ScanState) scan.State {
	for state, v := range scanStates {
		if v == pb {
			return state
		}
	}
	return ""
}

func scanProto(sc scan.Scan) *managementv1.Scan {
	pb := &managementv1.Scan{
		Id:         sc.ID.String(),
		LocationId: sc.LocationID.String(),
		State:      scanStates[sc.State],
		StartTime:  timestamppb.New(sc.StartTime),
		Error:      sc.Error,
		Progress: &managementv1.Scan_Progress{
			DirsSeen:     sc.Progress.DirsSeen,
			DirsMissing:  sc.Progress.DirsMissing,
			FilesSeen:    sc.Progress.FilesSeen,
			FilesMissing: sc.Progress.FilesMissing,
		},
	}
	if !sc.FinishTime.IsZero() {
		pb.FinishTime = timestamppb.New(sc.FinishTime)
	}
	return pb
}

func locationProto(loc storage.Location) *managementv1.Location {
	return &managementv1.Location{
		Id:   loc.ID.String(),
		Path: loc.URI,
	}
}

// locationFromProto converts a request's location and validates its
// path. A zero ID stays uuid.Nil for the caller to fill or reject.
func locationFromProto(pb *managementv1.Location) (storage.Location, error) {
	loc := storage.Location{URI: pb.GetPath()}
	if id := pb.GetId(); id != "" {
		parsed, err := parseID("location.id", id)
		if err != nil {
			return storage.Location{}, err
		}
		loc.ID = parsed
	}
	// Root rejects unsupported schemes, remote hosts, and empty paths,
	// so a location that cannot be scanned never reaches the store.
	if _, err := loc.Root(); err != nil {
		return storage.Location{}, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("location.path: %w", err))
	}
	return loc, nil
}
