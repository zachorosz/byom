package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	managementv1 "github.com/zachorosz/byom/proto/management/v1"
	"github.com/zachorosz/byom/proto/management/v1/managementv1connect"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

type Scanner interface {
	Start(ctx context.Context, locationID uuid.UUID, force bool) (scan.Scan, error)
	Cancel(ctx context.Context, scanID uuid.UUID) error
	Scan(ctx context.Context, id uuid.UUID) (scan.Scan, error)
	Scans(ctx context.Context, locationID uuid.UUID, state scan.State, token string, limit int) ([]scan.Scan, string, error)
}

type LocationStore interface {
	Insert(ctx context.Context, loc storage.Location) error
	Location(ctx context.Context, id uuid.UUID) (storage.Location, error)
	Locations(ctx context.Context, token string, limit int) ([]storage.Location, string, error)
	Update(ctx context.Context, loc storage.Location) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type ManagementServer struct {
	managementv1connect.UnimplementedManagementServiceHandler
	scans     Scanner
	locations LocationStore
}

func NewManagementServer(scans Scanner, locations LocationStore) *ManagementServer {
	return &ManagementServer{scans: scans, locations: locations}
}

func (s *ManagementServer) CreateLocation(ctx context.Context, req *managementv1.CreateLocationRequest) (*managementv1.CreateLocationResponse, error) {
	loc, err := locationFromProto(req.GetLocation())
	if err != nil {
		return nil, err
	}
	if loc.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, rpcError(err)
		}
		loc.ID = id
	}
	loc.Available = true

	if err := s.locations.Insert(ctx, loc); err != nil {
		return nil, rpcError(err)
	}
	return &managementv1.CreateLocationResponse{Location: locationProto(loc)}, nil
}

func (s *ManagementServer) ListLocations(ctx context.Context, req *managementv1.ListLocationsRequest) (*managementv1.ListLocationsResponse, error) {
	locations, next, err := s.locations.Locations(ctx, req.GetPageToken(), int(req.GetPageSize()))
	if err != nil {
		return nil, rpcError(err)
	}

	res := &managementv1.ListLocationsResponse{
		Items:         make([]*managementv1.Location, 0, len(locations)),
		NextPageToken: next,
	}
	for _, loc := range locations {
		res.Items = append(res.Items, locationProto(loc))
	}
	return res, nil
}

func (s *ManagementServer) GetLocation(ctx context.Context, req *managementv1.GetLocationRequest) (*managementv1.GetLocationResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	loc, err := s.locations.Location(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &managementv1.GetLocationResponse{Location: locationProto(loc)}, nil
}

func (s *ManagementServer) UpdateLocation(ctx context.Context, req *managementv1.UpdateLocationRequest) (*managementv1.UpdateLocationResponse, error) {
	loc, err := locationFromProto(req.GetLocation())
	if err != nil {
		return nil, err
	}
	if loc.ID == uuid.Nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("location.id: value is required"))
	}

	if err := s.locations.Update(ctx, loc); err != nil {
		if errors.Is(err, scan.ErrScanRunning) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, rpcError(err)
	}
	return &managementv1.UpdateLocationResponse{Location: locationProto(loc)}, nil
}

func (s *ManagementServer) DeleteLocation(ctx context.Context, req *managementv1.DeleteLocationRequest) (*managementv1.DeleteLocationResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	if err := s.locations.Delete(ctx, id); err != nil {
		if errors.Is(err, scan.ErrScanRunning) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, rpcError(err)
	}
	return &managementv1.DeleteLocationResponse{}, nil
}

func (s *ManagementServer) ScanLocation(ctx context.Context, req *managementv1.ScanLocationRequest) (*managementv1.ScanLocationResponse, error) {
	locationID, err := parseID("location_id", req.GetLocationId())
	if err != nil {
		return nil, err
	}

	sc, err := s.scans.Start(ctx, locationID, req.GetForce())
	if err != nil {
		return nil, rpcError(err)
	}
	return &managementv1.ScanLocationResponse{Scan: scanProto(sc)}, nil
}

func (s *ManagementServer) GetScan(ctx context.Context, req *managementv1.GetScanRequest) (*managementv1.GetScanResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	sc, err := s.scans.Scan(ctx, id)
	if err != nil {
		return nil, rpcError(err)
	}
	return &managementv1.GetScanResponse{Scan: scanProto(sc)}, nil
}

func (s *ManagementServer) ListScans(ctx context.Context, req *managementv1.ListScansRequest) (*managementv1.ListScansResponse, error) {
	locationID, err := parseOptionalID("location_id", req.GetLocationId())
	if err != nil {
		return nil, err
	}

	scans, next, err := s.scans.Scans(ctx, locationID, scanState(req.GetState()), req.GetPageToken(), int(req.GetPageSize()))
	if err != nil {
		return nil, rpcError(err)
	}

	res := &managementv1.ListScansResponse{
		Items:         make([]*managementv1.Scan, 0, len(scans)),
		NextPageToken: next,
	}
	for _, sc := range scans {
		res.Items = append(res.Items, scanProto(sc))
	}
	return res, nil
}

func (s *ManagementServer) CancelScan(ctx context.Context, req *managementv1.CancelScanRequest) (*managementv1.CancelScanResponse, error) {
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}

	if err := s.scans.Cancel(ctx, id); err != nil {
		return nil, rpcError(err)
	}
	return &managementv1.CancelScanResponse{}, nil
}
