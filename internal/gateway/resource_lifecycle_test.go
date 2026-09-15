// SPDX-License-Identifier: AGPL-3.0-only
package gateway

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
	"time"

	runnerv1 "github.com/agynio/gateway/gen/agynio/api/runner/v1"
	runnersv1 "github.com/agynio/gateway/gen/agynio/api/runners/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The backend supplies synthetic persisted records. These tests exercise the
// real gRPC/Connect JSON boundary, not registry validation or native cleanup.
func resourceGatewayRecords(owner runnersv1.RuntimeOwnerKind) (*runnersv1.Workload, *runnersv1.Volume) {
	w := &runnersv1.Workload{Meta: &runnersv1.EntityMeta{Id: uuid.NewString()}, OwnerKind: owner, OwnerId: uuid.NewString(),
		RunnerId: uuid.NewString(), ThreadId: uuid.NewString(), OrganizationId: uuid.NewString(),
		Status: runnersv1.WorkloadStatus_WORKLOAD_STATUS_STARTING}
	labels := map[string]string{"app.kubernetes.io/managed-by": "k8s-runner", "agyn.dev/managed-by": "agents-orchestrator", "managed-by": "agents-orchestrator"}
	if owner == runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE {
		w.AgentId = uuid.NewString()
		w.AgentInstanceId, w.AgentClassId = &w.OwnerId, &w.AgentId
		labels["agent-instance-id"], labels["agent-id"] = w.OwnerId, w.AgentId
	} else {
		labels["sandbox-id"], labels["sandbox-owner-id"] = w.OwnerId, uuid.NewString()
	}
	backend := "kubernetes-namespace/v1/workloads/" + uuid.NewString()
	anchor := func(kind runnerv1.ResourceAnchorKind, id string) *runnerv1.ResourceAnchor {
		result := &runnerv1.ResourceAnchor{Kind: kind, ResourceId: id, InstanceUid: uuid.NewString(), BackendId: backend, IdentityLabels: maps.Clone(labels)}
		if kind == runnerv1.ResourceAnchorKind_RESOURCE_ANCHOR_KIND_VOLUME {
			result.IdentityLabels["volume_key"] = id
		} else if owner == runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE {
			result.IdentityLabels["thread-id"] = w.ThreadId
		}
		return result
	}
	workspace, state := anchor(runnerv1.ResourceAnchorKind_RESOURCE_ANCHOR_KIND_VOLUME, uuid.NewString()), anchor(runnerv1.ResourceAnchorKind_RESOURCE_ANCHOR_KIND_VOLUME, uuid.NewString())
	w.Preparation = &runnersv1.PreparedWorkloadLifecycle{Phase: runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_RESERVED,
		Revision: 9007199254740993, BackendId: backend, VolumeIds: []string{workspace.ResourceId, state.ResourceId},
		Resources: &runnersv1.WorkloadResourceAnchors{Revision: 9007199254740994, Workload: anchor(runnerv1.ResourceAnchorKind_RESOURCE_ANCHOR_KIND_WORKLOAD, w.Meta.Id),
			Volumes: []*runnerv1.ResourceAnchor{workspace, state}}}
	v := &runnersv1.Volume{Meta: &runnersv1.EntityMeta{Id: workspace.ResourceId}, OwnerKind: owner, OwnerId: w.OwnerId,
		RunnerId: w.RunnerId, ThreadId: w.ThreadId, AgentId: w.AgentId, OrganizationId: w.OrganizationId,
		AgentInstanceId: w.AgentInstanceId, AgentClassId: w.AgentClassId, SizeGb: "1", Status: runnersv1.VolumeStatus_VOLUME_STATUS_PROVISIONING,
		CheckedLifecycle: true, LifecycleRevision: 9007199254740995, ResourceAnchor: workspace,
		AnchorReservation: &runnersv1.VolumeAnchorReservation{WorkloadId: w.Meta.Id, PreparationRevision: 1, ResourceRevision: 1}}
	return w, v
}

func resourceGatewayBinding(anchor *runnerv1.ResourceAnchor) *runnerv1.VolumeListItem {
	return &runnerv1.VolumeListItem{InstanceId: "workspace-" + anchor.ResourceId, InstanceUid: uuid.NewString(), VolumeKey: anchor.ResourceId,
		BackendId: anchor.BackendId, Anchor: proto.Clone(anchor).(*runnerv1.ResourceAnchor), IdentityLabels: maps.Clone(anchor.IdentityLabels)}
}

func TestAnchoredWorkloadSurvivesGRPCToGatewayJSON(t *testing.T) {
	for _, owner := range []runnersv1.RuntimeOwnerKind{runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE, runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_SANDBOX} {
		t.Run(owner.String(), func(t *testing.T) {
			for _, phase := range []string{"reserved", "bound", "revoked", "confirmed", "zero-volume-confirmed"} {
				t.Run(phase, func(t *testing.T) {
					w, _ := resourceGatewayRecords(owner)
					p := w.Preparation
					if phase == "bound" {
						p.Phase = runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_BOUND
						p.Binding = &runnerv1.WorkloadBinding{WorkloadId: w.Meta.Id, InstanceUid: uuid.NewString(), BackendId: p.BackendId, Anchor: p.Resources.Workload}
						for _, a := range p.Resources.Volumes {
							p.Binding.Volumes = append(p.Binding.Volumes, resourceGatewayBinding(a))
						}
						w.InstanceId = &w.Meta.Id
					} else if phase != "reserved" {
						p.Phase = runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVING
						w.Status = runnersv1.WorkloadStatus_WORKLOAD_STATUS_STOPPING
						if phase == "zero-volume-confirmed" {
							p.VolumeIds, p.Resources.Volumes = nil, nil
						}
						proof := &runnerv1.PreparationRevocation{WorkloadAnchor: p.Resources.Workload, VolumeAnchors: p.Resources.Volumes, InstanceUid: uuid.NewString()}
						p.Resources.PreparationRevocation = proof
						if phase != "revoked" {
							p.Phase, w.Status = runnersv1.PreparedWorkloadPhase_PREPARED_WORKLOAD_PHASE_REMOVED, runnersv1.WorkloadStatus_WORKLOAD_STATUS_STOPPED
							w.RemovedAt = timestamppb.New(time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC))
							w.RemovalConfirmedAt = timestamppb.New(w.RemovedAt.AsTime().Add(time.Second))
							observation := &runnerv1.ObservePreparationRevocationResponse{Revocation: proto.Clone(proof).(*runnerv1.PreparationRevocation),
								State: runnerv1.RevokedPreparationState_REVOKED_PREPARATION_STATE_POD_ABSENT}
							if len(p.Resources.Volumes) != 0 {
								observation.Volumes = []*runnerv1.VolumeListItem{resourceGatewayBinding(p.Resources.Volumes[0])}
								observation.AbsentVolumeIds = []string{p.Resources.Volumes[1].ResourceId}
							}
							p.Resources.RevocationObservation = observation
						}
					}
					requests := []struct {
						method            string
						request, response proto.Message
					}{
						{"GetWorkload", &runnersv1.GetWorkloadRequest{Id: w.Meta.Id}, &runnersv1.GetWorkloadResponse{Workload: w}},
						{"ListWorkloads", &runnersv1.ListWorkloadsRequest{OrganizationId: &w.OrganizationId, PageSize: 2, PageToken: "page-two"}, &runnersv1.ListWorkloadsResponse{Workloads: []*runnersv1.Workload{w}, NextPageToken: "page-three"}},
						{"ListWorkloadsByThread", &runnersv1.ListWorkloadsByThreadRequest{ThreadId: w.ThreadId, PageSize: 2, PageToken: "page-two"}, &runnersv1.ListWorkloadsByThreadResponse{Workloads: []*runnersv1.Workload{w}, NextPageToken: "page-three"}},
					}
					if owner == runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE {
						requests = append(requests, struct {
							method            string
							request, response proto.Message
						}{"ListWorkloadsByAgentInstance",
							&runnersv1.ListWorkloadsByAgentInstanceRequest{AgentInstanceId: w.OwnerId, PageSize: 2, PageToken: "page-two", Statuses: []runnersv1.WorkloadStatus{w.Status}},
							&runnersv1.ListWorkloadsByAgentInstanceResponse{Workloads: []*runnersv1.Workload{w}, NextPageToken: "page-three"}})
					}
					for _, request := range requests {
						t.Run(request.method, func(t *testing.T) {
							body := assertPreparedGatewayJSON(t, w, request.method, request.request, request.response)
							fields := resourceGatewayFields(t, body, "workload", "workloads")
							var preparation map[string]json.RawMessage
							if err := json.Unmarshal(fields["preparation"], &preparation); err != nil {
								t.Fatal(err)
							}
							var resources map[string]json.RawMessage
							if err := json.Unmarshal(preparation["resources"], &resources); err != nil {
								t.Fatal(err)
							}
							if string(resources["revision"]) != `"9007199254740994"` {
								t.Fatal("resource revision lost JSON precision")
							}
							for field, want := range map[string]bool{"preparationRevocation": p.Resources.PreparationRevocation != nil, "revocationObservation": p.Resources.RevocationObservation != nil} {
								if _, present := resources[field]; present != want {
									t.Fatalf("%s evidence presence changed", field)
								}
							}
						})
					}
				})
			}
		})
	}
}

type resourceVolumesServer struct {
	runnersv1.UnimplementedRunnersServiceServer
	volume *runnersv1.Volume
}

func (s *resourceVolumesServer) GetVolume(context.Context, *runnersv1.GetVolumeRequest) (*runnersv1.GetVolumeResponse, error) {
	return &runnersv1.GetVolumeResponse{Volume: s.volume}, nil
}
func (s *resourceVolumesServer) ListVolumes(context.Context, *runnersv1.ListVolumesRequest) (*runnersv1.ListVolumesResponse, error) {
	return &runnersv1.ListVolumesResponse{Volumes: []*runnersv1.Volume{s.volume}, NextPageToken: "page-three"}, nil
}
func (s *resourceVolumesServer) ListVolumesByThread(context.Context, *runnersv1.ListVolumesByThreadRequest) (*runnersv1.ListVolumesByThreadResponse, error) {
	return &runnersv1.ListVolumesByThreadResponse{Volumes: []*runnersv1.Volume{s.volume}, NextPageToken: "page-three"}, nil
}
func (s *resourceVolumesServer) ListVolumesByAgentInstance(context.Context, *runnersv1.ListVolumesByAgentInstanceRequest) (*runnersv1.ListVolumesByAgentInstanceResponse, error) {
	return &runnersv1.ListVolumesByAgentInstanceResponse{Volumes: []*runnersv1.Volume{s.volume}, NextPageToken: "page-three"}, nil
}

func TestAnchoredVolumeSurvivesGRPCToGatewayJSON(t *testing.T) {
	for _, owner := range []runnersv1.RuntimeOwnerKind{runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE, runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_SANDBOX} {
		t.Run(owner.String(), func(t *testing.T) {
			for _, phase := range []string{"unbound", "bound", "removing", "removed"} {
				t.Run(phase, func(t *testing.T) {
					_, v := resourceGatewayRecords(owner)
					if phase != "unbound" {
						v.BoundInstance = resourceGatewayBinding(v.ResourceAnchor)
						v.InstanceId = &v.BoundInstance.InstanceId
						v.Status = runnersv1.VolumeStatus_VOLUME_STATUS_ACTIVE
					}
					if phase == "removing" || phase == "removed" {
						v.Status = runnersv1.VolumeStatus_VOLUME_STATUS_DEPROVISIONING
						v.RemovalIntent = &runnersv1.VolumeRemovalIntent{Id: uuid.NewString(), Expected: v.BoundInstance, Anchored: true,
							RequestedAt: timestamppb.New(time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC))}
						if phase == "removed" {
							v.Status = runnersv1.VolumeStatus_VOLUME_STATUS_DELETED
							v.RemovedAt = timestamppb.New(v.RemovalIntent.RequestedAt.AsTime().Add(time.Second))
							v.RemovalIntent.ConfirmedAt = v.RemovedAt
							v.AnchoredRemovalObservation = &runnerv1.RemoveVolumeAnchoredResponse{State: runnerv1.VolumeRemovalState_VOLUME_REMOVAL_STATE_ABSENT, BackendId: v.ResourceAnchor.BackendId, Anchor: v.ResourceAnchor}
						}
					}
					requests := []struct {
						method            string
						request, response proto.Message
					}{
						{"GetVolume", &runnersv1.GetVolumeRequest{Id: v.Meta.Id}, &runnersv1.GetVolumeResponse{Volume: v}},
						{"ListVolumes", &runnersv1.ListVolumesRequest{OrganizationId: &v.OrganizationId, PageSize: 2, PageToken: "page-two", Filter: &runnersv1.ListVolumesFilter{OwnerIdIn: []string{v.OwnerId}, OwnerKindIn: []runnersv1.RuntimeOwnerKind{owner}, StatusIn: []runnersv1.VolumeStatus{v.Status}}}, &runnersv1.ListVolumesResponse{Volumes: []*runnersv1.Volume{v}, NextPageToken: "page-three"}},
						{"ListVolumesByThread", &runnersv1.ListVolumesByThreadRequest{ThreadId: v.ThreadId, PageSize: 2, PageToken: "page-two"}, &runnersv1.ListVolumesByThreadResponse{Volumes: []*runnersv1.Volume{v}, NextPageToken: "page-three"}},
					}
					if owner == runnersv1.RuntimeOwnerKind_RUNTIME_OWNER_KIND_AGENT_INSTANCE {
						requests = append(requests, struct {
							method            string
							request, response proto.Message
						}{"ListVolumesByAgentInstance",
							&runnersv1.ListVolumesByAgentInstanceRequest{AgentInstanceId: v.OwnerId, PageSize: 2, PageToken: "page-two"},
							&runnersv1.ListVolumesByAgentInstanceResponse{Volumes: []*runnersv1.Volume{v}, NextPageToken: "page-three"}})
					}
					for _, request := range requests {
						t.Run(request.method, func(t *testing.T) {
							body := assertRunnersGatewayJSON(t, &resourceVolumesServer{volume: v}, request.method, request.request, request.response)
							fields := resourceGatewayFields(t, body, "volume", "volumes")
							if string(fields["lifecycleRevision"]) != `"9007199254740995"` {
								t.Fatal("volume revision lost JSON precision")
							}
							for field, want := range map[string]bool{"boundInstance": v.BoundInstance != nil, "removalIntent": v.RemovalIntent != nil, "anchoredRemovalObservation": v.AnchoredRemovalObservation != nil} {
								if _, present := fields[field]; present != want {
									t.Fatalf("%s evidence presence changed", field)
								}
							}
						})
					}
				})
			}
		})
	}
}

func resourceGatewayFields(t *testing.T, body []byte, singular, plural string) map[string]json.RawMessage {
	t.Helper()
	var response map[string]json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	record := response[singular]
	if record == nil {
		var records []json.RawMessage
		if err := json.Unmarshal(response[plural], &records); err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("expected one %s", singular)
		}
		record = records[0]
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(record, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}
