package rpc

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"

	managementv1 "github.com/zachorosz/byom/proto/management/v1"
	"github.com/zachorosz/byom/proto/management/v1/managementv1connect"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

// fakeScanner is a Scanner that returns canned results and
// records the arguments of the last call.
type fakeScanner struct {
	scan  scan.Scan
	scans []scan.Scan
	next  string
	err   error

	gotLocationID uuid.UUID
	gotScanID     uuid.UUID
	gotState      scan.State
	gotToken      string
	gotLimit      int
}

func (f *fakeScanner) Start(_ context.Context, locationID uuid.UUID) (scan.Scan, error) {
	f.gotLocationID = locationID
	return f.scan, f.err
}

func (f *fakeScanner) Cancel(_ context.Context, scanID uuid.UUID) error {
	f.gotScanID = scanID
	return f.err
}

func (f *fakeScanner) Scan(_ context.Context, id uuid.UUID) (scan.Scan, error) {
	f.gotScanID = id
	return f.scan, f.err
}

func (f *fakeScanner) Scans(_ context.Context, locationID uuid.UUID, state scan.State, token string, limit int) ([]scan.Scan, string, error) {
	f.gotLocationID, f.gotState, f.gotToken, f.gotLimit = locationID, state, token, limit
	return f.scans, f.next, f.err
}

// fakeLocations is a LocationStore that returns canned results and
// records the arguments of the last call.
type fakeLocations struct {
	loc       storage.Location
	locations []storage.Location
	next      string
	err       error

	gotLoc   storage.Location
	gotID    uuid.UUID
	gotToken string
	gotLimit int
}

func (f *fakeLocations) Insert(_ context.Context, loc storage.Location) error {
	f.gotLoc = loc
	return f.err
}

func (f *fakeLocations) Location(_ context.Context, id uuid.UUID) (storage.Location, error) {
	f.gotID = id
	return f.loc, f.err
}

func (f *fakeLocations) Locations(_ context.Context, token string, limit int) ([]storage.Location, string, error) {
	f.gotToken, f.gotLimit = token, limit
	return f.locations, f.next, f.err
}

func (f *fakeLocations) Update(_ context.Context, loc storage.Location) error {
	f.gotLoc = loc
	return f.err
}

func (f *fakeLocations) Delete(_ context.Context, id uuid.UUID) error {
	f.gotID = id
	return f.err
}

func newTestManagementClient(t *testing.T, scans Scanner, locations LocationStore) managementv1connect.ManagementServiceClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(NewHandler(logger, NewLibraryServer(nil), NewManagementServer(scans, locations)))
	t.Cleanup(srv.Close)
	return managementv1connect.NewManagementServiceClient(srv.Client(), srv.URL)
}

func TestManagementServer_ScanLocation(t *testing.T) {
	locationID := uuid.Must(uuid.NewV7())
	started := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	sc := scan.Scan{
		ID:         uuid.Must(uuid.NewV7()),
		LocationID: locationID,
		State:      scan.StateRunning,
		StartTime:  started,
		Progress:   scan.Progress{DirsSeen: 7, FilesSeen: 42},
	}
	scanner := &fakeScanner{scan: sc}
	client := newTestManagementClient(t, scanner, &fakeLocations{})

	req := &managementv1.ScanLocationRequest{LocationId: locationID.String()}
	got, err := client.ScanLocation(context.Background(), req)
	if err != nil {
		t.Fatalf("ScanLocation() returned an unexpected error: %v", err)
	}
	if scanner.gotLocationID != locationID {
		t.Errorf("Start(%v), want Start(%v)", scanner.gotLocationID, locationID)
	}

	want := &managementv1.ScanLocationResponse{
		Scan: &managementv1.Scan{
			Id:         sc.ID.String(),
			LocationId: locationID.String(),
			State:      managementv1.ScanState_SCAN_STATE_RUNNING,
			StartTime:  timestamppb.New(started),
			Progress:   &managementv1.Scan_Progress{DirsSeen: 7, FilesSeen: 42},
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("ScanLocation() mismatch (-want +got):\n%s", diff)
	}
}

func TestManagementServer_ScanLocation_Errors(t *testing.T) {
	tests := []struct {
		name       string
		locationID string
		err        error
		want       connect.Code
	}{
		{
			name:       "missingLocationID",
			locationID: "",
			want:       connect.CodeInvalidArgument,
		},
		{
			name:       "malformedLocationID",
			locationID: "not-a-uuid",
			want:       connect.CodeInvalidArgument,
		},
		{
			name:       "unknownLocation",
			locationID: uuid.Must(uuid.NewV7()).String(),
			err:        storage.ErrNotExists,
			want:       connect.CodeNotFound,
		},
		{
			name:       "alreadyScanning",
			locationID: uuid.Must(uuid.NewV7()).String(),
			err:        scan.ErrScanRunning,
			want:       connect.CodeAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestManagementClient(t, &fakeScanner{err: tt.err}, &fakeLocations{})

			req := &managementv1.ScanLocationRequest{LocationId: tt.locationID}
			_, err := client.ScanLocation(context.Background(), req)
			if got := connect.CodeOf(err); got != tt.want {
				t.Errorf("ScanLocation() code = %v, want %v (err: %v)", got, tt.want, err)
			}
		})
	}
}

func TestManagementServer_CancelScan(t *testing.T) {
	scanID := uuid.Must(uuid.NewV7())
	scanner := &fakeScanner{}
	client := newTestManagementClient(t, scanner, &fakeLocations{})

	req := &managementv1.CancelScanRequest{Id: scanID.String()}
	if _, err := client.CancelScan(context.Background(), req); err != nil {
		t.Fatalf("CancelScan() returned an unexpected error: %v", err)
	}
	if scanner.gotScanID != scanID {
		t.Errorf("Cancel(%v), want Cancel(%v)", scanner.gotScanID, scanID)
	}
}

func TestManagementServer_CancelScan_AlreadyFinished(t *testing.T) {
	client := newTestManagementClient(t, &fakeScanner{err: scan.ErrNotRunning}, &fakeLocations{})

	req := &managementv1.CancelScanRequest{Id: uuid.Must(uuid.NewV7()).String()}
	_, err := client.CancelScan(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("CancelScan() code = %v, want %v (err: %v)", got, connect.CodeFailedPrecondition, err)
	}
}

func TestManagementServer_GetScan_NotFound(t *testing.T) {
	client := newTestManagementClient(t, &fakeScanner{err: storage.ErrNotExists}, &fakeLocations{})

	req := &managementv1.GetScanRequest{Id: uuid.Must(uuid.NewV7()).String()}
	_, err := client.GetScan(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Errorf("GetScan() code = %v, want %v (err: %v)", got, connect.CodeNotFound, err)
	}
}

func TestManagementServer_ListScans_PassesFilters(t *testing.T) {
	locationID := uuid.Must(uuid.NewV7())
	scanner := &fakeScanner{next: "next-token"}
	client := newTestManagementClient(t, scanner, &fakeLocations{})

	req := &managementv1.ListScansRequest{
		LocationId: locationID.String(),
		State:      managementv1.ScanState_SCAN_STATE_DONE,
		PageSize:   5,
		PageToken:  "page-token",
	}
	got, err := client.ListScans(context.Background(), req)
	if err != nil {
		t.Fatalf("ListScans() returned an unexpected error: %v", err)
	}
	if got.GetNextPageToken() != "next-token" {
		t.Errorf("ListScans() next page token = %q, want %q", got.GetNextPageToken(), "next-token")
	}
	if scanner.gotLocationID != locationID {
		t.Errorf("Scans() location filter = %v, want %v", scanner.gotLocationID, locationID)
	}
	if scanner.gotState != scan.StateDone {
		t.Errorf("Scans() state filter = %q, want %q", scanner.gotState, scan.StateDone)
	}
	if scanner.gotToken != "page-token" || scanner.gotLimit != 5 {
		t.Errorf("Scans(%q, %d), want Scans(%q, %d)",
			scanner.gotToken, scanner.gotLimit, "page-token", 5)
	}
}

func TestManagementServer_ListScans_NoFilters(t *testing.T) {
	scanner := &fakeScanner{}
	client := newTestManagementClient(t, scanner, &fakeLocations{})

	if _, err := client.ListScans(context.Background(), &managementv1.ListScansRequest{}); err != nil {
		t.Fatalf("ListScans() returned an unexpected error: %v", err)
	}
	if scanner.gotLocationID != uuid.Nil {
		t.Errorf("Scans() location filter = %v, want uuid.Nil", scanner.gotLocationID)
	}
	if scanner.gotState != "" {
		t.Errorf("Scans() state filter = %q, want empty", scanner.gotState)
	}
}

func TestManagementServer_CreateLocation_MintsID(t *testing.T) {
	locations := &fakeLocations{}
	client := newTestManagementClient(t, &fakeScanner{}, locations)

	req := &managementv1.CreateLocationRequest{
		Location: &managementv1.Location{Path: "file:///music"},
	}
	got, err := client.CreateLocation(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateLocation() returned an unexpected error: %v", err)
	}
	if locations.gotLoc.ID == uuid.Nil {
		t.Error("Insert() location ID = uuid.Nil, want a minted ID")
	}
	if got := locations.gotLoc.Available; !got {
		t.Errorf("Insert() location Available = %v, want %v", got, true)
	}

	want := &managementv1.CreateLocationResponse{
		Location: &managementv1.Location{
			Id:   locations.gotLoc.ID.String(),
			Path: "file:///music",
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("CreateLocation() mismatch (-want +got):\n%s", diff)
	}
}

func TestManagementServer_CreateLocation_KeepsSuppliedID(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	locations := &fakeLocations{}
	client := newTestManagementClient(t, &fakeScanner{}, locations)

	req := &managementv1.CreateLocationRequest{
		Location: &managementv1.Location{Id: id.String(), Path: "/music"},
	}
	if _, err := client.CreateLocation(context.Background(), req); err != nil {
		t.Fatalf("CreateLocation() returned an unexpected error: %v", err)
	}
	if got := locations.gotLoc.ID; got != id {
		t.Errorf("Insert() location ID = %v, want %v", got, id)
	}
}

func TestManagementServer_CreateLocation_Errors(t *testing.T) {
	tests := []struct {
		name     string
		location *managementv1.Location
		err      error
		want     connect.Code
	}{
		{
			name: "missingLocation",
			want: connect.CodeInvalidArgument,
		},
		{
			name:     "missingPath",
			location: &managementv1.Location{},
			want:     connect.CodeInvalidArgument,
		},
		{
			name:     "malformedID",
			location: &managementv1.Location{Id: "not-a-uuid", Path: "/music"},
			want:     connect.CodeInvalidArgument,
		},
		{
			name:     "unsupportedScheme",
			location: &managementv1.Location{Path: "http://example.com/music"},
			want:     connect.CodeInvalidArgument,
		},
		{
			name:     "remoteHost",
			location: &managementv1.Location{Path: "file://nas/music"},
			want:     connect.CodeInvalidArgument,
		},
		{
			name:     "duplicatePath",
			location: &managementv1.Location{Path: "/music"},
			err:      storage.ErrExists,
			want:     connect.CodeAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestManagementClient(t, &fakeScanner{}, &fakeLocations{err: tt.err})

			req := &managementv1.CreateLocationRequest{Location: tt.location}
			_, err := client.CreateLocation(context.Background(), req)
			if got := connect.CodeOf(err); got != tt.want {
				t.Errorf("CreateLocation() code = %v, want %v (err: %v)", got, tt.want, err)
			}
		})
	}
}

func TestManagementServer_GetLocation(t *testing.T) {
	loc := storage.Location{ID: uuid.Must(uuid.NewV7()), URI: "file:///music", Available: true}
	locations := &fakeLocations{loc: loc}
	client := newTestManagementClient(t, &fakeScanner{}, locations)

	req := &managementv1.GetLocationRequest{Id: loc.ID.String()}
	got, err := client.GetLocation(context.Background(), req)
	if err != nil {
		t.Fatalf("GetLocation() returned an unexpected error: %v", err)
	}
	if locations.gotID != loc.ID {
		t.Errorf("Location(%v), want Location(%v)", locations.gotID, loc.ID)
	}

	want := &managementv1.GetLocationResponse{
		Location: &managementv1.Location{Id: loc.ID.String(), Path: "file:///music"},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("GetLocation() mismatch (-want +got):\n%s", diff)
	}
}

func TestManagementServer_GetLocation_NotFound(t *testing.T) {
	client := newTestManagementClient(t, &fakeScanner{}, &fakeLocations{err: storage.ErrNotExists})

	req := &managementv1.GetLocationRequest{Id: uuid.Must(uuid.NewV7()).String()}
	_, err := client.GetLocation(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Errorf("GetLocation() code = %v, want %v (err: %v)", got, connect.CodeNotFound, err)
	}
}

func TestManagementServer_ListLocations(t *testing.T) {
	first := storage.Location{ID: uuid.Must(uuid.NewV7()), URI: "/music/a"}
	second := storage.Location{ID: uuid.Must(uuid.NewV7()), URI: "/music/b"}
	locations := &fakeLocations{
		locations: []storage.Location{first, second},
		next:      "next-token",
	}
	client := newTestManagementClient(t, &fakeScanner{}, locations)

	req := &managementv1.ListLocationsRequest{PageSize: 3, PageToken: "page-token"}
	got, err := client.ListLocations(context.Background(), req)
	if err != nil {
		t.Fatalf("ListLocations() returned an unexpected error: %v", err)
	}
	if locations.gotToken != "page-token" || locations.gotLimit != 3 {
		t.Errorf("Locations(%q, %d), want Locations(%q, %d)",
			locations.gotToken, locations.gotLimit, "page-token", 3)
	}

	want := &managementv1.ListLocationsResponse{
		Items: []*managementv1.Location{
			{Id: first.ID.String(), Path: "/music/a"},
			{Id: second.ID.String(), Path: "/music/b"},
		},
		NextPageToken: "next-token",
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("ListLocations() mismatch (-want +got):\n%s", diff)
	}
}

func TestManagementServer_UpdateLocation(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	locations := &fakeLocations{}
	client := newTestManagementClient(t, &fakeScanner{}, locations)

	req := &managementv1.UpdateLocationRequest{
		Location: &managementv1.Location{Id: id.String(), Path: "file:///moved"},
	}
	got, err := client.UpdateLocation(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdateLocation() returned an unexpected error: %v", err)
	}
	want := storage.Location{ID: id, URI: "file:///moved"}
	if diff := cmp.Diff(want, locations.gotLoc); diff != "" {
		t.Errorf("Update() location mismatch (-want +got):\n%s", diff)
	}

	wantRes := &managementv1.UpdateLocationResponse{
		Location: &managementv1.Location{Id: id.String(), Path: "file:///moved"},
	}
	if diff := cmp.Diff(wantRes, got, protocmp.Transform()); diff != "" {
		t.Errorf("UpdateLocation() mismatch (-want +got):\n%s", diff)
	}
}

func TestManagementServer_UpdateLocation_RefusesWhileScanning(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	client := newTestManagementClient(t, &fakeScanner{}, &fakeLocations{err: scan.ErrScanRunning})

	req := &managementv1.UpdateLocationRequest{
		Location: &managementv1.Location{Id: id.String(), Path: "file:///moved"},
	}
	_, err := client.UpdateLocation(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("UpdateLocation() code = %v, want %v (err: %v)", got, connect.CodeFailedPrecondition, err)
	}
}

func TestManagementServer_UpdateLocation_Errors(t *testing.T) {
	tests := []struct {
		name     string
		location *managementv1.Location
		err      error
		want     connect.Code
	}{
		{
			name:     "missingID",
			location: &managementv1.Location{Path: "/music"},
			want:     connect.CodeInvalidArgument,
		},
		{
			name:     "unknownLocation",
			location: &managementv1.Location{Id: uuid.Must(uuid.NewV7()).String(), Path: "/music"},
			err:      storage.ErrNotExists,
			want:     connect.CodeNotFound,
		},
		{
			name:     "duplicatePath",
			location: &managementv1.Location{Id: uuid.Must(uuid.NewV7()).String(), Path: "/music"},
			err:      storage.ErrExists,
			want:     connect.CodeAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestManagementClient(t, &fakeScanner{}, &fakeLocations{err: tt.err})

			req := &managementv1.UpdateLocationRequest{Location: tt.location}
			_, err := client.UpdateLocation(context.Background(), req)
			if got := connect.CodeOf(err); got != tt.want {
				t.Errorf("UpdateLocation() code = %v, want %v (err: %v)", got, tt.want, err)
			}
		})
	}
}

func TestManagementServer_DeleteLocation(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	locations := &fakeLocations{}
	client := newTestManagementClient(t, &fakeScanner{}, locations)

	req := &managementv1.DeleteLocationRequest{Id: id.String()}
	if _, err := client.DeleteLocation(context.Background(), req); err != nil {
		t.Fatalf("DeleteLocation() returned an unexpected error: %v", err)
	}
	if locations.gotID != id {
		t.Errorf("Delete(%v), want Delete(%v)", locations.gotID, id)
	}
}

func TestManagementServer_DeleteLocation_RefusesWhileScanning(t *testing.T) {
	client := newTestManagementClient(t, &fakeScanner{}, &fakeLocations{err: scan.ErrScanRunning})

	req := &managementv1.DeleteLocationRequest{Id: uuid.Must(uuid.NewV7()).String()}
	_, err := client.DeleteLocation(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("DeleteLocation() code = %v, want %v (err: %v)", got, connect.CodeFailedPrecondition, err)
	}
}

func TestManagementServer_DeleteLocation_NotFound(t *testing.T) {
	client := newTestManagementClient(t, &fakeScanner{}, &fakeLocations{err: storage.ErrNotExists})

	req := &managementv1.DeleteLocationRequest{Id: uuid.Must(uuid.NewV7()).String()}
	_, err := client.DeleteLocation(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Errorf("DeleteLocation() code = %v, want %v (err: %v)", got, connect.CodeNotFound, err)
	}
}

func TestScanStateRoundTrip(t *testing.T) {
	states := []scan.State{
		scan.StateRunning, scan.StateCancelling,
		scan.StateDone, scan.StateFailed, scan.StateAborted,
	}
	for _, want := range states {
		if got := scanState(scanStates[want]); got != want {
			t.Errorf("scanState(%v) = %q, want %q", scanStates[want], got, want)
		}
	}
	if got := scanState(managementv1.ScanState_SCAN_STATE_UNSPECIFIED); got != "" {
		t.Errorf("scanState(UNSPECIFIED) = %q, want empty", got)
	}
}
