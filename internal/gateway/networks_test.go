package gateway

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	networksv1 "github.com/agynio/gateway/gen/agynio/api/networks/v1"
	"github.com/agynio/gateway/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeNetworksClient struct {
	networksv1.NetworksServiceClient
	createNetworkReq      *networksv1.CreateNetworkRequest
	createNetworkMetadata metadata.MD
	createNetworkErr      error
	getNetworkReq         *networksv1.GetNetworkRequest
	listNetworksReq       *networksv1.ListNetworksRequest
	updateNetworkReq      *networksv1.UpdateNetworkRequest
	deleteNetworkReq      *networksv1.DeleteNetworkRequest
	createTunnelReq       *networksv1.CreateTunnelCredentialRequest
	getTunnelReq          *networksv1.GetTunnelCredentialRequest
	listTunnelsReq        *networksv1.ListTunnelCredentialsRequest
	deleteTunnelReq       *networksv1.DeleteTunnelCredentialRequest
	createResourceReq     *networksv1.CreatePrivateResourceRequest
	getResourceReq        *networksv1.GetPrivateResourceRequest
	listResourcesReq      *networksv1.ListPrivateResourcesRequest
	updateResourceReq     *networksv1.UpdatePrivateResourceRequest
	deleteResourceReq     *networksv1.DeletePrivateResourceRequest
	createAccessReq       *networksv1.CreatePrivateResourceAccessRequest
	deleteAccessReq       *networksv1.DeletePrivateResourceAccessRequest
	listAccessReq         *networksv1.ListPrivateResourceAccessRequest
}

func (f *fakeNetworksClient) CreateNetwork(ctx context.Context, req *networksv1.CreateNetworkRequest, _ ...grpc.CallOption) (*networksv1.CreateNetworkResponse, error) {
	f.createNetworkReq = req
	f.createNetworkMetadata, _ = metadata.FromOutgoingContext(ctx)
	if f.createNetworkErr != nil {
		return nil, f.createNetworkErr
	}
	return &networksv1.CreateNetworkResponse{}, nil
}
func (f *fakeNetworksClient) GetNetwork(_ context.Context, req *networksv1.GetNetworkRequest, _ ...grpc.CallOption) (*networksv1.GetNetworkResponse, error) {
	f.getNetworkReq = req
	return &networksv1.GetNetworkResponse{}, nil
}
func (f *fakeNetworksClient) ListNetworks(_ context.Context, req *networksv1.ListNetworksRequest, _ ...grpc.CallOption) (*networksv1.ListNetworksResponse, error) {
	f.listNetworksReq = req
	return &networksv1.ListNetworksResponse{}, nil
}
func (f *fakeNetworksClient) UpdateNetwork(_ context.Context, req *networksv1.UpdateNetworkRequest, _ ...grpc.CallOption) (*networksv1.UpdateNetworkResponse, error) {
	f.updateNetworkReq = req
	return &networksv1.UpdateNetworkResponse{}, nil
}
func (f *fakeNetworksClient) DeleteNetwork(_ context.Context, req *networksv1.DeleteNetworkRequest, _ ...grpc.CallOption) (*networksv1.DeleteNetworkResponse, error) {
	f.deleteNetworkReq = req
	return &networksv1.DeleteNetworkResponse{}, nil
}
func (f *fakeNetworksClient) CreateTunnelCredential(_ context.Context, req *networksv1.CreateTunnelCredentialRequest, _ ...grpc.CallOption) (*networksv1.CreateTunnelCredentialResponse, error) {
	f.createTunnelReq = req
	return &networksv1.CreateTunnelCredentialResponse{}, nil
}
func (f *fakeNetworksClient) GetTunnelCredential(_ context.Context, req *networksv1.GetTunnelCredentialRequest, _ ...grpc.CallOption) (*networksv1.GetTunnelCredentialResponse, error) {
	f.getTunnelReq = req
	return &networksv1.GetTunnelCredentialResponse{}, nil
}
func (f *fakeNetworksClient) ListTunnelCredentials(_ context.Context, req *networksv1.ListTunnelCredentialsRequest, _ ...grpc.CallOption) (*networksv1.ListTunnelCredentialsResponse, error) {
	f.listTunnelsReq = req
	return &networksv1.ListTunnelCredentialsResponse{}, nil
}
func (f *fakeNetworksClient) DeleteTunnelCredential(_ context.Context, req *networksv1.DeleteTunnelCredentialRequest, _ ...grpc.CallOption) (*networksv1.DeleteTunnelCredentialResponse, error) {
	f.deleteTunnelReq = req
	return &networksv1.DeleteTunnelCredentialResponse{}, nil
}
func (f *fakeNetworksClient) CreatePrivateResource(_ context.Context, req *networksv1.CreatePrivateResourceRequest, _ ...grpc.CallOption) (*networksv1.CreatePrivateResourceResponse, error) {
	f.createResourceReq = req
	return &networksv1.CreatePrivateResourceResponse{}, nil
}
func (f *fakeNetworksClient) GetPrivateResource(_ context.Context, req *networksv1.GetPrivateResourceRequest, _ ...grpc.CallOption) (*networksv1.GetPrivateResourceResponse, error) {
	f.getResourceReq = req
	return &networksv1.GetPrivateResourceResponse{}, nil
}
func (f *fakeNetworksClient) ListPrivateResources(_ context.Context, req *networksv1.ListPrivateResourcesRequest, _ ...grpc.CallOption) (*networksv1.ListPrivateResourcesResponse, error) {
	f.listResourcesReq = req
	return &networksv1.ListPrivateResourcesResponse{}, nil
}
func (f *fakeNetworksClient) UpdatePrivateResource(_ context.Context, req *networksv1.UpdatePrivateResourceRequest, _ ...grpc.CallOption) (*networksv1.UpdatePrivateResourceResponse, error) {
	f.updateResourceReq = req
	return &networksv1.UpdatePrivateResourceResponse{}, nil
}
func (f *fakeNetworksClient) DeletePrivateResource(_ context.Context, req *networksv1.DeletePrivateResourceRequest, _ ...grpc.CallOption) (*networksv1.DeletePrivateResourceResponse, error) {
	f.deleteResourceReq = req
	return &networksv1.DeletePrivateResourceResponse{}, nil
}
func (f *fakeNetworksClient) CreatePrivateResourceAccess(_ context.Context, req *networksv1.CreatePrivateResourceAccessRequest, _ ...grpc.CallOption) (*networksv1.CreatePrivateResourceAccessResponse, error) {
	f.createAccessReq = req
	return &networksv1.CreatePrivateResourceAccessResponse{}, nil
}
func (f *fakeNetworksClient) DeletePrivateResourceAccess(_ context.Context, req *networksv1.DeletePrivateResourceAccessRequest, _ ...grpc.CallOption) (*networksv1.DeletePrivateResourceAccessResponse, error) {
	f.deleteAccessReq = req
	return &networksv1.DeletePrivateResourceAccessResponse{}, nil
}
func (f *fakeNetworksClient) ListPrivateResourceAccess(_ context.Context, req *networksv1.ListPrivateResourceAccessRequest, _ ...grpc.CallOption) (*networksv1.ListPrivateResourceAccessResponse, error) {
	f.listAccessReq = req
	return &networksv1.ListPrivateResourceAccessResponse{}, nil
}

func TestNetworksGatewayForwardsAndPropagatesMetadata(t *testing.T) {
	client := &fakeNetworksClient{}
	gateway := NewNetworksGateway(client)
	resolved := identity.ResolvedIdentity{IdentityID: "user-1", IdentityType: identity.IdentityTypeUser}
	ctx := identity.WithIdentity(context.Background(), resolved)

	if _, err := gateway.CreateNetwork(ctx, connect.NewRequest(&networksv1.CreateNetworkRequest{OrganizationId: "org-1", Name: "corp"})); err != nil {
		t.Fatalf("create network: %v", err)
	}
	if client.createNetworkReq.GetName() != "corp" {
		t.Fatalf("expected create request forwarded")
	}
	assertMetadataValue(t, client.createNetworkMetadata, identity.MetadataKeyIdentityID, resolved.IdentityID)

	_, _ = gateway.GetNetwork(context.Background(), connect.NewRequest(&networksv1.GetNetworkRequest{Id: "network-1"}))
	_, _ = gateway.ListNetworks(context.Background(), connect.NewRequest(&networksv1.ListNetworksRequest{OrganizationId: "org-1"}))
	_, _ = gateway.UpdateNetwork(context.Background(), connect.NewRequest(&networksv1.UpdateNetworkRequest{Id: "network-2"}))
	_, _ = gateway.DeleteNetwork(context.Background(), connect.NewRequest(&networksv1.DeleteNetworkRequest{Id: "network-3"}))
	_, _ = gateway.CreateTunnelCredential(context.Background(), connect.NewRequest(&networksv1.CreateTunnelCredentialRequest{NetworkId: "network-4"}))
	_, _ = gateway.GetTunnelCredential(context.Background(), connect.NewRequest(&networksv1.GetTunnelCredentialRequest{Id: "tunnel-1"}))
	_, _ = gateway.ListTunnelCredentials(context.Background(), connect.NewRequest(&networksv1.ListTunnelCredentialsRequest{NetworkId: "network-5"}))
	_, _ = gateway.DeleteTunnelCredential(context.Background(), connect.NewRequest(&networksv1.DeleteTunnelCredentialRequest{Id: "tunnel-2"}))
	_, _ = gateway.CreatePrivateResource(context.Background(), connect.NewRequest(&networksv1.CreatePrivateResourceRequest{NetworkId: "network-6", Name: "db"}))
	_, _ = gateway.GetPrivateResource(context.Background(), connect.NewRequest(&networksv1.GetPrivateResourceRequest{Id: "resource-1"}))
	_, _ = gateway.ListPrivateResources(context.Background(), connect.NewRequest(&networksv1.ListPrivateResourcesRequest{NetworkId: stringPtr("network-7")}))
	_, _ = gateway.UpdatePrivateResource(context.Background(), connect.NewRequest(&networksv1.UpdatePrivateResourceRequest{Id: "resource-2"}))
	_, _ = gateway.DeletePrivateResource(context.Background(), connect.NewRequest(&networksv1.DeletePrivateResourceRequest{Id: "resource-3"}))
	_, _ = gateway.CreatePrivateResourceAccess(context.Background(), connect.NewRequest(&networksv1.CreatePrivateResourceAccessRequest{PrivateResourceId: "resource-4", PrincipalId: "user-2"}))
	_, _ = gateway.DeletePrivateResourceAccess(context.Background(), connect.NewRequest(&networksv1.DeletePrivateResourceAccessRequest{Id: "access-1"}))
	_, _ = gateway.ListPrivateResourceAccess(context.Background(), connect.NewRequest(&networksv1.ListPrivateResourceAccessRequest{PrivateResourceId: stringPtr("resource-5")}))

	if client.getNetworkReq.GetId() != "network-1" || client.listNetworksReq.GetOrganizationId() != "org-1" || client.updateNetworkReq.GetId() != "network-2" || client.deleteNetworkReq.GetId() != "network-3" || client.createTunnelReq.GetNetworkId() != "network-4" || client.getTunnelReq.GetId() != "tunnel-1" || client.listTunnelsReq.GetNetworkId() != "network-5" || client.deleteTunnelReq.GetId() != "tunnel-2" || client.createResourceReq.GetNetworkId() != "network-6" || client.getResourceReq.GetId() != "resource-1" || client.listResourcesReq.GetNetworkId() != "network-7" || client.updateResourceReq.GetId() != "resource-2" || client.deleteResourceReq.GetId() != "resource-3" || client.createAccessReq.GetPrivateResourceId() != "resource-4" || client.deleteAccessReq.GetId() != "access-1" || client.listAccessReq.GetPrivateResourceId() != "resource-5" {
		t.Fatalf("expected public network methods to be forwarded")
	}
}

func TestNetworksGatewayMapsErrors(t *testing.T) {
	client := &fakeNetworksClient{createNetworkErr: status.Error(codes.Unavailable, "down")}
	_, err := NewNetworksGateway(client).CreateNetwork(context.Background(), connect.NewRequest(&networksv1.CreateNetworkRequest{}))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func stringPtr(value string) *string { return &value }
