// SPDX-License-Identifier: AGPL-3.0-only
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gatewayv1connect "github.com/agynio/gateway/gen/agynio/api/gateway/v1/gatewayv1connect"
	runnerv1 "github.com/agynio/gateway/gen/agynio/api/runner/v1"
	runnersv1 "github.com/agynio/gateway/gen/agynio/api/runners/v1"
	"github.com/agynio/gateway/internal/grpcclient"
	"github.com/agynio/gateway/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type preparedRunnersServer struct {
	runnersv1.UnimplementedRunnersServiceServer
	workload *runnersv1.Workload
}

func (s *preparedRunnersServer) GetWorkload(context.Context, *runnersv1.GetWorkloadRequest) (*runnersv1.GetWorkloadResponse, error) {
	return &runnersv1.GetWorkloadResponse{Workload: s.workload}, nil
}

func (s *preparedRunnersServer) ListWorkloads(context.Context, *runnersv1.ListWorkloadsRequest) (*runnersv1.ListWorkloadsResponse, error) {
	return &runnersv1.ListWorkloadsResponse{Workloads: []*runnersv1.Workload{s.workload}, NextPageToken: "page-three"}, nil
}

func (s *preparedRunnersServer) ListWorkloadsByAgentInstance(context.Context, *runnersv1.ListWorkloadsByAgentInstanceRequest) (*runnersv1.ListWorkloadsByAgentInstanceResponse, error) {
	return &runnersv1.ListWorkloadsByAgentInstanceResponse{Workloads: []*runnersv1.Workload{s.workload}, NextPageToken: "page-three"}, nil
}

func (s *preparedRunnersServer) ListWorkloadsByThread(context.Context, *runnersv1.ListWorkloadsByThreadRequest) (*runnersv1.ListWorkloadsByThreadResponse, error) {
	return &runnersv1.ListWorkloadsByThreadResponse{Workloads: []*runnersv1.Workload{s.workload}, NextPageToken: "page-three"}, nil
}

func TestPreparedWorkloadSurvivesGRPCToGatewayJSON(t *testing.T) {
	for _, owner := range []runnersv1.RuntimeOwnerKind{runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE, runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_SANDBOX} {
		t.Run(owner.String(), func(t *testing.T) {
			for _, phase := range []struct {
				name      string
				phase     runnersv1.PreparedWorkloadPhase
				bound     bool
				confirmed bool
			}{
				{"legacy", 0, false, false},
				{"reserved", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_RESERVED, false, false},
				{"preparing", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_PREPARING, false, false},
				{"bound", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_BOUND, true, false},
				{"activating", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_ACTIVATING, true, false},
				{"active", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_ACTIVE, true, false},
				{"removing-unknown", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVING, false, false},
				{"removing-bound", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVING, true, false},
				{"removed", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVED, true, true},
				{"unused-reservation-aborted", runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVED, false, true},
			} {
				t.Run(phase.name, func(t *testing.T) {
					ownerID, classID, orgID := "owner", "agent-class", "organization"
					workload := &runnersv1.Workload{Meta: &runnersv1.EntityMeta{Id: "workload"},
						RunnerId: "runner", ThreadId: "thread", OrganizationId: orgID, OwnerId: ownerID, OwnerKind: owner,
						Status: runnersv1.WorkloadStatus_WORKLOAD_STATUS_STARTING}
					if owner == runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE {
						workload.AgentInstanceId, workload.AgentClassId = &ownerID, &classID
					}
					if phase.phase != 0 {
						workload.Preparation = &runnersv1.PreparedWorkloadLifecycle{Phase: phase.phase, Revision: 9007199254740993,
							BackendId: "kubernetes-namespace/v1/workloads/namespace-uid", VolumeIds: []string{"registry-workspace", "registry-state"}}
					}
					if phase.bound {
						binding := &runnerv1.WorkloadBinding{WorkloadId: workload.Meta.Id, InstanceUid: "pod-uid", BackendId: workload.Preparation.BackendId}
						for _, key := range []string{"workspace", "state"} {
							binding.Volumes = append(binding.Volumes, &runnerv1.VolumeListItem{InstanceId: key + "-pvc", InstanceUid: key + "-pvc-uid",
								VolumeKey: key, BackendId: binding.BackendId, IdentityLabels: map[string]string{
									"agyn.io/owner-id": ownerID, "agyn.io/owner-kind": owner.String(), "agyn.io/thread-id": workload.ThreadId,
								}})
						}
						workload.Preparation.Binding = binding
					}
					if phase.phase == runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_ACTIVE {
						workload.Status = runnersv1.WorkloadStatus_WORKLOAD_STATUS_RUNNING
					}
					if phase.phase == 0 || phase.phase >= runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVING {
						workload.Status = runnersv1.WorkloadStatus_WORKLOAD_STATUS_FAILED
						workload.RemovedAt = timestamppb.New(time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
					}
					if phase.confirmed {
						workload.RemovalConfirmedAt = timestamppb.New(workload.RemovedAt.AsTime().Add(time.Minute))
						if phase.bound {
							workload.Preparation.RemovalObservation = &runnerv1.RemovePreparedWorkloadResponse{
								State:   runnerv1.PreparedWorkloadRemovalState_PREPARED_WORKLOAD_REMOVAL_STATE_ABSENT,
								Binding: proto.Clone(workload.Preparation.Binding).(*runnerv1.WorkloadBinding)}
						}
					}
					requests := []struct {
						method   string
						request  proto.Message
						response proto.Message
					}{
						{"GetWorkload", &runnersv1.GetWorkloadRequest{Id: workload.Meta.Id}, &runnersv1.GetWorkloadResponse{Workload: workload}},
						{"ListWorkloads", &runnersv1.ListWorkloadsRequest{OrganizationId: &orgID, PageSize: 2, PageToken: "page-two"},
							&runnersv1.ListWorkloadsResponse{Workloads: []*runnersv1.Workload{workload}, NextPageToken: "page-three"}},
						{"ListWorkloadsByThread", &runnersv1.ListWorkloadsByThreadRequest{ThreadId: workload.ThreadId, PageSize: 2, PageToken: "page-two"},
							&runnersv1.ListWorkloadsByThreadResponse{Workloads: []*runnersv1.Workload{workload}, NextPageToken: "page-three"}},
					}
					if owner == runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE {
						requests = append(requests, struct {
							method   string
							request  proto.Message
							response proto.Message
						}{"ListWorkloadsByAgentInstance", &runnersv1.ListWorkloadsByAgentInstanceRequest{AgentInstanceId: ownerID, PageSize: 2, PageToken: "page-two",
							Statuses: []runnersv1.WorkloadStatus{workload.Status}},
							&runnersv1.ListWorkloadsByAgentInstanceResponse{Workloads: []*runnersv1.Workload{workload}, NextPageToken: "page-three"}})
					}
					for _, request := range requests {
						t.Run(request.method, func(t *testing.T) {
							assertPreparedGatewayJSON(t, workload, request.method, request.request, request.response)
						})
					}
				})
			}
		})
	}
}

func assertRunnersGatewayJSON(t *testing.T, backend runnersv1.RunnersServiceServer, method string, request, expected proto.Message) []byte {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		calls.Add(1)
		md, _ := metadata.FromIncomingContext(ctx)
		if ids := md.Get(identity.MetadataKeyIdentityID); len(ids) != 1 || ids[0] != "test-owner" {
			return nil, status.Error(codes.Unauthenticated, "identity was not forwarded exactly once")
		}
		if info.FullMethod != "/agynio.api.runners.v1.RunnersService/"+method || !proto.Equal(req.(proto.Message), request) {
			return nil, status.Error(codes.InvalidArgument, "request identity, filter or pagination changed")
		}
		return handler(ctx, req)
	}))
	runnersv1.RegisterRunnersServiceServer(server, backend)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		listener.Close()
		if err := <-served; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	client, err := grpcclient.New(listener.Addr().String(), runnersv1.NewRunnersServiceClient)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	_, handler := gatewayv1connect.NewRunnersGatewayHandler(NewRunnersGateway(client.Service()))
	// Authentication is a resolved-identity fixture; the test proves wire forwarding.
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved := identity.ResolvedIdentity{IdentityID: "test-owner", IdentityType: identity.IdentityTypeUser}
		handler.ServeHTTP(w, r.WithContext(identity.WithIdentity(r.Context(), resolved)))
	}))
	t.Cleanup(httpServer.Close)
	input, err := protojson.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := httpServer.Client()
	httpClient.Timeout = 5 * time.Second
	response, err := httpClient.Post(httpServer.URL+"/agynio.api.gateway.v1.RunnersGateway/"+method, "application/json", strings.NewReader(string(input)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 65536))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("HTTP %d, downstream calls %d: %s", response.StatusCode, calls.Load(), body)
	}
	actual := expected.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal(body, actual); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(actual, expected) {
		t.Fatalf("resource lifecycle, ownership, binding or confirmation changed: %s", body)
	}
	return body
}

func assertPreparedGatewayJSON(t *testing.T, workload *runnersv1.Workload, method string, request, expected proto.Message) []byte {
	t.Helper()
	body := assertRunnersGatewayJSON(t, &preparedRunnersServer{workload: workload}, method, request, expected)
	var raw struct {
		Workload  json.RawMessage   `json:"workload"`
		Workloads []json.RawMessage `json:"workloads"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if method != "GetWorkload" {
		if len(raw.Workloads) != 1 {
			t.Fatalf("expected one workload: %s", body)
		}
		raw.Workload = raw.Workloads[0]
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw.Workload, &fields); err != nil {
		t.Fatal(err)
	}
	if workload.Preparation == nil {
		if _, present := fields["preparation"]; present {
			t.Fatal("legacy workload acquired prepared lifecycle evidence")
		}
		return body
	}
	var preparation map[string]json.RawMessage
	if err := json.Unmarshal(fields["preparation"], &preparation); err != nil {
		t.Fatal(err)
	}
	if string(preparation["revision"]) != `"9007199254740993"` {
		t.Fatal("revision must remain an exact decimal string for JSON clients")
	}
	for field, want := range map[string]bool{"binding": workload.Preparation.Binding != nil, "removalObservation": workload.Preparation.RemovalObservation != nil} {
		if _, present := preparation[field]; present != want {
			t.Fatalf("%s evidence presence changed", field)
		}
	}
	if _, present := fields["removalConfirmedAt"]; present != (workload.RemovalConfirmedAt != nil) {
		t.Fatal("billing end changed explicit removal-confirmation presence")
	}
	return body
}
