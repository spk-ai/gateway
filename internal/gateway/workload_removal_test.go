// SPDX-License-Identifier: AGPL-3.0-only
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gatewayv1connect "github.com/agynio/gateway/gen/agynio/api/gateway/v1/gatewayv1connect"
	runnersv1 "github.com/agynio/gateway/gen/agynio/api/runners/v1"
	"github.com/agynio/gateway/internal/grpcclient"
	"github.com/agynio/gateway/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type removalRunnersServer struct {
	runnersv1.UnimplementedRunnersServiceServer
	workloads []*runnersv1.Workload
}

func (s *removalRunnersServer) ListWorkloadsByAgentInstance(ctx context.Context, req *runnersv1.ListWorkloadsByAgentInstanceRequest) (*runnersv1.ListWorkloadsByAgentInstanceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if ids := md.Get(identity.MetadataKeyIdentityID); len(ids) != 1 || ids[0] != "test-owner" {
		return nil, status.Error(codes.Unauthenticated, "identity was not forwarded exactly once")
	}
	if req.GetAgentInstanceId() != "instance" || req.GetPageSize() != 2 || req.GetPageToken() != "page-two" {
		return nil, status.Error(codes.InvalidArgument, "instance or pagination was not forwarded")
	}
	return &runnersv1.ListWorkloadsByAgentInstanceResponse{Workloads: s.workloads, NextPageToken: "page-three"}, nil
}

func TestWorkloadRemovalConfirmationSurvivesGRPCToGatewayJSON(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		name := "billing-only"
		if confirmed {
			name = "confirmed-absence"
		}
		t.Run(name, func(t *testing.T) {
			instanceID := "instance"
			billingEnd := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
			confirmation := billingEnd.Add(time.Minute)
			backend := &removalRunnersServer{}
			for _, state := range []runnersv1.WorkloadStatus{runnersv1.WorkloadStatus_WORKLOAD_STATUS_FAILED, runnersv1.WorkloadStatus_WORKLOAD_STATUS_STOPPED} {
				workload := &runnersv1.Workload{Meta: &runnersv1.EntityMeta{Id: state.String()}, AgentInstanceId: &instanceID,
					Status: state, RemovedAt: timestamppb.New(billingEnd)}
				if confirmed {
					workload.RemovalConfirmedAt = timestamppb.New(confirmation)
				}
				backend.workloads = append(backend.workloads, workload)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
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
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resolved := identity.ResolvedIdentity{IdentityID: "test-owner", IdentityType: identity.IdentityTypeUser}
				handler.ServeHTTP(w, r.WithContext(identity.WithIdentity(r.Context(), resolved)))
			}))
			t.Cleanup(httpServer.Close)
			httpClient := httpServer.Client()
			httpClient.Timeout = 5 * time.Second
			response, err := httpClient.Post(httpServer.URL+"/agynio.api.gateway.v1.RunnersGateway/ListWorkloadsByAgentInstance", "application/json",
				strings.NewReader(`{"agentInstanceId":"instance","pageSize":2,"pageToken":"page-two"}`))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("Gateway returned HTTP %d", response.StatusCode)
			}
			var body struct {
				Workloads []struct {
					Meta struct {
						ID string `json:"id"`
					} `json:"meta"`
					Status             string  `json:"status"`
					AgentInstanceID    string  `json:"agentInstanceId"`
					RemovedAt          string  `json:"removedAt"`
					RemovalConfirmedAt *string `json:"removalConfirmedAt"`
				} `json:"workloads"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.NextPageToken != "page-three" || len(body.Workloads) != len(backend.workloads) {
				t.Fatal("workload list or pagination was lost")
			}
			for index, workload := range body.Workloads {
				if workload.Meta.ID != backend.workloads[index].GetMeta().GetId() || workload.AgentInstanceID != instanceID || workload.Status != backend.workloads[index].GetStatus().String() {
					t.Fatal("workload identity or status changed")
				}
				if workload.RemovedAt != billingEnd.Format(time.RFC3339) {
					t.Fatal("billing end changed")
				}
				if !confirmed && workload.RemovalConfirmedAt != nil {
					t.Fatal("billing end became a removal confirmation")
				}
				if confirmed && (workload.RemovalConfirmedAt == nil || *workload.RemovalConfirmedAt != confirmation.Format(time.RFC3339)) {
					t.Fatal("explicit removal confirmation was lost in the Gateway JSON path")
				}
			}
		})
	}
}
