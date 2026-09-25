package gateway

import (
	"context"
	"errors"
	"io"

	"connectrpc.com/connect"
	runnerv1 "github.com/agynio/gateway/gen/agynio/api/runner/v1"
	runnersv1 "github.com/agynio/gateway/gen/agynio/api/runners/v1"
	"google.golang.org/grpc"
)

// RunnersGateway forwards generated lifecycle messages without synthesizing or
// projecting evidence. Reads retain optional confirmation, bindings, anchors and
// separate revocation/retirement receipts; removed_at is billing only. Connect
// protobuf JSON preserves uint64 revisions as decimal strings even above 2^53.
// Matching generated types are required: unknown gRPC fields on an older build
// do not survive JSON serialization. Preserve filters/pagination and forward
// identity through downstreamContext once per call. Lifecycle mutation RPCs stay
// internal; read compatibility adds no public mutation or cleanup authority.
// @see api::proto/agynio/api/runners/v1/runners
// @see runners::internal/server/workloads
// @see internal/gateway/workload_removal_test.go
// @see internal/gateway/prepared_workload_test.go
// @see internal/gateway/resource_lifecycle_test.go
type RunnersGateway struct {
	runners runnersClient
}

type runnersClient interface {
	RegisterRunner(context.Context, *runnersv1.RegisterRunnerRequest, ...grpc.CallOption) (*runnersv1.RegisterRunnerResponse, error)
	EnrollRunner(context.Context, *runnersv1.EnrollRunnerRequest, ...grpc.CallOption) (*runnersv1.EnrollRunnerResponse, error)
	GetRunner(context.Context, *runnersv1.GetRunnerRequest, ...grpc.CallOption) (*runnersv1.GetRunnerResponse, error)
	ListRunners(context.Context, *runnersv1.ListRunnersRequest, ...grpc.CallOption) (*runnersv1.ListRunnersResponse, error)
	UpdateRunner(context.Context, *runnersv1.UpdateRunnerRequest, ...grpc.CallOption) (*runnersv1.UpdateRunnerResponse, error)
	DeleteRunner(context.Context, *runnersv1.DeleteRunnerRequest, ...grpc.CallOption) (*runnersv1.DeleteRunnerResponse, error)
	CreateFlavor(context.Context, *runnersv1.CreateFlavorRequest, ...grpc.CallOption) (*runnersv1.CreateFlavorResponse, error)
	GetFlavor(context.Context, *runnersv1.GetFlavorRequest, ...grpc.CallOption) (*runnersv1.GetFlavorResponse, error)
	UpdateFlavor(context.Context, *runnersv1.UpdateFlavorRequest, ...grpc.CallOption) (*runnersv1.UpdateFlavorResponse, error)
	DeleteFlavor(context.Context, *runnersv1.DeleteFlavorRequest, ...grpc.CallOption) (*runnersv1.DeleteFlavorResponse, error)
	ListFlavors(context.Context, *runnersv1.ListFlavorsRequest, ...grpc.CallOption) (*runnersv1.ListFlavorsResponse, error)
	ReportRunnerCatalog(context.Context, *runnersv1.ReportRunnerCatalogRequest, ...grpc.CallOption) (*runnersv1.ReportRunnerCatalogResponse, error)
	ReportWorkloadState(context.Context, *runnersv1.ReportWorkloadStateRequest, ...grpc.CallOption) (*runnersv1.ReportWorkloadStateResponse, error)
	ListStorageClasses(context.Context, *runnersv1.ListStorageClassesRequest, ...grpc.CallOption) (*runnersv1.ListStorageClassesResponse, error)
	GetVolume(context.Context, *runnersv1.GetVolumeRequest, ...grpc.CallOption) (*runnersv1.GetVolumeResponse, error)
	ListVolumes(context.Context, *runnersv1.ListVolumesRequest, ...grpc.CallOption) (*runnersv1.ListVolumesResponse, error)
	ListVolumesByThread(context.Context, *runnersv1.ListVolumesByThreadRequest, ...grpc.CallOption) (*runnersv1.ListVolumesByThreadResponse, error)
	ListVolumesByAgentInstance(context.Context, *runnersv1.ListVolumesByAgentInstanceRequest, ...grpc.CallOption) (*runnersv1.ListVolumesByAgentInstanceResponse, error)
	ListWorkloadsByThread(context.Context, *runnersv1.ListWorkloadsByThreadRequest, ...grpc.CallOption) (*runnersv1.ListWorkloadsByThreadResponse, error)
	ListWorkloadsByAgentInstance(context.Context, *runnersv1.ListWorkloadsByAgentInstanceRequest, ...grpc.CallOption) (*runnersv1.ListWorkloadsByAgentInstanceResponse, error)
	ListWorkloads(context.Context, *runnersv1.ListWorkloadsRequest, ...grpc.CallOption) (*runnersv1.ListWorkloadsResponse, error)
	GetWorkload(context.Context, *runnersv1.GetWorkloadRequest, ...grpc.CallOption) (*runnersv1.GetWorkloadResponse, error)
	TouchWorkload(context.Context, *runnersv1.TouchWorkloadRequest, ...grpc.CallOption) (*runnersv1.TouchWorkloadResponse, error)
	StreamWorkloadLogs(context.Context, *runnerv1.StreamWorkloadLogsRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[runnerv1.StreamWorkloadLogsResponse], error)
}

func NewRunnersGateway(runners runnersClient) *RunnersGateway {
	return &RunnersGateway{runners: runners}
}

func (g *RunnersGateway) RegisterRunner(ctx context.Context, req *connect.Request[runnersv1.RegisterRunnerRequest]) (*connect.Response[runnersv1.RegisterRunnerResponse], error) {
	resp, err := g.runners.RegisterRunner(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) EnrollRunner(ctx context.Context, req *connect.Request[runnersv1.EnrollRunnerRequest]) (*connect.Response[runnersv1.EnrollRunnerResponse], error) {
	resp, err := g.runners.EnrollRunner(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) GetRunner(ctx context.Context, req *connect.Request[runnersv1.GetRunnerRequest]) (*connect.Response[runnersv1.GetRunnerResponse], error) {
	resp, err := g.runners.GetRunner(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListRunners(ctx context.Context, req *connect.Request[runnersv1.ListRunnersRequest]) (*connect.Response[runnersv1.ListRunnersResponse], error) {
	resp, err := g.runners.ListRunners(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) UpdateRunner(ctx context.Context, req *connect.Request[runnersv1.UpdateRunnerRequest]) (*connect.Response[runnersv1.UpdateRunnerResponse], error) {
	resp, err := g.runners.UpdateRunner(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) DeleteRunner(ctx context.Context, req *connect.Request[runnersv1.DeleteRunnerRequest]) (*connect.Response[runnersv1.DeleteRunnerResponse], error) {
	resp, err := g.runners.DeleteRunner(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) CreateFlavor(ctx context.Context, req *connect.Request[runnersv1.CreateFlavorRequest]) (*connect.Response[runnersv1.CreateFlavorResponse], error) {
	resp, err := g.runners.CreateFlavor(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) GetFlavor(ctx context.Context, req *connect.Request[runnersv1.GetFlavorRequest]) (*connect.Response[runnersv1.GetFlavorResponse], error) {
	resp, err := g.runners.GetFlavor(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) UpdateFlavor(ctx context.Context, req *connect.Request[runnersv1.UpdateFlavorRequest]) (*connect.Response[runnersv1.UpdateFlavorResponse], error) {
	resp, err := g.runners.UpdateFlavor(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) DeleteFlavor(ctx context.Context, req *connect.Request[runnersv1.DeleteFlavorRequest]) (*connect.Response[runnersv1.DeleteFlavorResponse], error) {
	resp, err := g.runners.DeleteFlavor(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListFlavors(ctx context.Context, req *connect.Request[runnersv1.ListFlavorsRequest]) (*connect.Response[runnersv1.ListFlavorsResponse], error) {
	resp, err := g.runners.ListFlavors(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

// ReportRunnerCatalog carries the runner's service token in the request, as
// EnrollRunner does; the runners service authenticates it there.
func (g *RunnersGateway) ReportRunnerCatalog(ctx context.Context, req *connect.Request[runnersv1.ReportRunnerCatalogRequest]) (*connect.Response[runnersv1.ReportRunnerCatalogResponse], error) {
	resp, err := g.runners.ReportRunnerCatalog(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

// ReportWorkloadState carries the runner's service token in the request, as
// ReportRunnerCatalog does; the runners service authenticates it there.
func (g *RunnersGateway) ReportWorkloadState(ctx context.Context, req *connect.Request[runnersv1.ReportWorkloadStateRequest]) (*connect.Response[runnersv1.ReportWorkloadStateResponse], error) {
	resp, err := g.runners.ReportWorkloadState(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListStorageClasses(ctx context.Context, req *connect.Request[runnersv1.ListStorageClassesRequest]) (*connect.Response[runnersv1.ListStorageClassesResponse], error) {
	resp, err := g.runners.ListStorageClasses(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) GetVolume(ctx context.Context, req *connect.Request[runnersv1.GetVolumeRequest]) (*connect.Response[runnersv1.GetVolumeResponse], error) {
	resp, err := g.runners.GetVolume(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListVolumes(ctx context.Context, req *connect.Request[runnersv1.ListVolumesRequest]) (*connect.Response[runnersv1.ListVolumesResponse], error) {
	resp, err := g.runners.ListVolumes(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListVolumesByThread(ctx context.Context, req *connect.Request[runnersv1.ListVolumesByThreadRequest]) (*connect.Response[runnersv1.ListVolumesByThreadResponse], error) {
	resp, err := g.runners.ListVolumesByThread(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListVolumesByAgentInstance(ctx context.Context, req *connect.Request[runnersv1.ListVolumesByAgentInstanceRequest]) (*connect.Response[runnersv1.ListVolumesByAgentInstanceResponse], error) {
	resp, err := g.runners.ListVolumesByAgentInstance(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListWorkloadsByThread(ctx context.Context, req *connect.Request[runnersv1.ListWorkloadsByThreadRequest]) (*connect.Response[runnersv1.ListWorkloadsByThreadResponse], error) {
	resp, err := g.runners.ListWorkloadsByThread(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListWorkloadsByAgentInstance(ctx context.Context, req *connect.Request[runnersv1.ListWorkloadsByAgentInstanceRequest]) (*connect.Response[runnersv1.ListWorkloadsByAgentInstanceResponse], error) {
	resp, err := g.runners.ListWorkloadsByAgentInstance(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) ListWorkloads(ctx context.Context, req *connect.Request[runnersv1.ListWorkloadsRequest]) (*connect.Response[runnersv1.ListWorkloadsResponse], error) {
	resp, err := g.runners.ListWorkloads(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) GetWorkload(ctx context.Context, req *connect.Request[runnersv1.GetWorkloadRequest]) (*connect.Response[runnersv1.GetWorkloadResponse], error) {
	resp, err := g.runners.GetWorkload(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) TouchWorkload(ctx context.Context, req *connect.Request[runnersv1.TouchWorkloadRequest]) (*connect.Response[runnersv1.TouchWorkloadResponse], error) {
	resp, err := g.runners.TouchWorkload(downstreamContext(ctx), req.Msg)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (g *RunnersGateway) StreamWorkloadLogs(ctx context.Context, req *connect.Request[runnerv1.StreamWorkloadLogsRequest], stream *connect.ServerStream[runnerv1.StreamWorkloadLogsResponse]) error {
	grpcStream, err := g.runners.StreamWorkloadLogs(downstreamContext(ctx), req.Msg)
	if err != nil {
		return toConnectError(err)
	}

	for {
		msg, err := grpcStream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return toConnectError(err)
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
}
