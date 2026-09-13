package gateway

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	groupsv1 "github.com/agynio/gateway/gen/agynio/api/groups/v1"
	"github.com/agynio/gateway/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeGroupsClient struct {
	groupsv1.GroupsServiceClient
	createReq       *groupsv1.CreateGroupRequest
	createMetadata  metadata.MD
	createErr       error
	getReq          *groupsv1.GetGroupRequest
	listReq         *groupsv1.ListGroupsRequest
	updateReq       *groupsv1.UpdateGroupRequest
	deleteReq       *groupsv1.DeleteGroupRequest
	addReq          *groupsv1.AddMemberRequest
	removeReq       *groupsv1.RemoveMemberRequest
	membersReq      *groupsv1.ListMembersRequest
	memberGroupsReq *groupsv1.ListMemberGroupsRequest
}

func (f *fakeGroupsClient) DeleteOrganizationResources(ctx context.Context, req *groupsv1.DeleteOrganizationResourcesRequest, opts ...grpc.CallOption) (*groupsv1.DeleteOrganizationResourcesResponse, error) {
	return &groupsv1.DeleteOrganizationResourcesResponse{}, nil
}

func (f *fakeGroupsClient) CreateGroup(ctx context.Context, req *groupsv1.CreateGroupRequest, _ ...grpc.CallOption) (*groupsv1.CreateGroupResponse, error) {
	f.createReq = req
	f.createMetadata, _ = metadata.FromOutgoingContext(ctx)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &groupsv1.CreateGroupResponse{}, nil
}
func (f *fakeGroupsClient) GetGroup(_ context.Context, req *groupsv1.GetGroupRequest, _ ...grpc.CallOption) (*groupsv1.GetGroupResponse, error) {
	f.getReq = req
	return &groupsv1.GetGroupResponse{}, nil
}
func (f *fakeGroupsClient) ListGroups(_ context.Context, req *groupsv1.ListGroupsRequest, _ ...grpc.CallOption) (*groupsv1.ListGroupsResponse, error) {
	f.listReq = req
	return &groupsv1.ListGroupsResponse{}, nil
}
func (f *fakeGroupsClient) UpdateGroup(_ context.Context, req *groupsv1.UpdateGroupRequest, _ ...grpc.CallOption) (*groupsv1.UpdateGroupResponse, error) {
	f.updateReq = req
	return &groupsv1.UpdateGroupResponse{}, nil
}
func (f *fakeGroupsClient) DeleteGroup(_ context.Context, req *groupsv1.DeleteGroupRequest, _ ...grpc.CallOption) (*groupsv1.DeleteGroupResponse, error) {
	f.deleteReq = req
	return &groupsv1.DeleteGroupResponse{}, nil
}
func (f *fakeGroupsClient) AddMember(_ context.Context, req *groupsv1.AddMemberRequest, _ ...grpc.CallOption) (*groupsv1.AddMemberResponse, error) {
	f.addReq = req
	return &groupsv1.AddMemberResponse{}, nil
}
func (f *fakeGroupsClient) RemoveMember(_ context.Context, req *groupsv1.RemoveMemberRequest, _ ...grpc.CallOption) (*groupsv1.RemoveMemberResponse, error) {
	f.removeReq = req
	return &groupsv1.RemoveMemberResponse{}, nil
}
func (f *fakeGroupsClient) ListMembers(_ context.Context, req *groupsv1.ListMembersRequest, _ ...grpc.CallOption) (*groupsv1.ListMembersResponse, error) {
	f.membersReq = req
	return &groupsv1.ListMembersResponse{}, nil
}
func (f *fakeGroupsClient) ListMemberGroups(_ context.Context, req *groupsv1.ListMemberGroupsRequest, _ ...grpc.CallOption) (*groupsv1.ListMemberGroupsResponse, error) {
	f.memberGroupsReq = req
	return &groupsv1.ListMemberGroupsResponse{}, nil
}
func (f *fakeGroupsClient) ListMemberGroupsBatch(context.Context, *groupsv1.ListMemberGroupsBatchRequest, ...grpc.CallOption) (*groupsv1.ListMemberGroupsBatchResponse, error) {
	return &groupsv1.ListMemberGroupsBatchResponse{}, nil
}

func TestGroupsGatewayForwardsAndPropagatesMetadata(t *testing.T) {
	client := &fakeGroupsClient{}
	gateway := NewGroupsGateway(client)
	resolved := identity.ResolvedIdentity{IdentityID: "user-1", IdentityType: identity.IdentityTypeUser}
	ctx := identity.WithIdentity(context.Background(), resolved)

	if _, err := gateway.CreateGroup(ctx, connect.NewRequest(&groupsv1.CreateGroupRequest{OrganizationId: "org-1", Name: "admins"})); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if client.createReq.GetName() != "admins" {
		t.Fatalf("expected create request to be forwarded")
	}
	assertMetadataValue(t, client.createMetadata, identity.MetadataKeyIdentityID, resolved.IdentityID)

	_, _ = gateway.GetGroup(context.Background(), connect.NewRequest(&groupsv1.GetGroupRequest{Id: "group-1"}))
	_, _ = gateway.ListGroups(context.Background(), connect.NewRequest(&groupsv1.ListGroupsRequest{OrganizationId: "org-1"}))
	_, _ = gateway.UpdateGroup(context.Background(), connect.NewRequest(&groupsv1.UpdateGroupRequest{Id: "group-2"}))
	_, _ = gateway.DeleteGroup(context.Background(), connect.NewRequest(&groupsv1.DeleteGroupRequest{Id: "group-3"}))
	_, _ = gateway.AddMember(context.Background(), connect.NewRequest(&groupsv1.AddMemberRequest{GroupId: "group-4", MemberId: "user-2"}))
	_, _ = gateway.RemoveMember(context.Background(), connect.NewRequest(&groupsv1.RemoveMemberRequest{GroupId: "group-5", MemberId: "user-3"}))
	_, _ = gateway.ListMembers(context.Background(), connect.NewRequest(&groupsv1.ListMembersRequest{GroupId: "group-6"}))
	_, _ = gateway.ListMemberGroups(context.Background(), connect.NewRequest(&groupsv1.ListMemberGroupsRequest{OrganizationId: "org-1", MemberId: "user-4"}))

	if client.getReq.GetId() != "group-1" || client.listReq.GetOrganizationId() != "org-1" || client.updateReq.GetId() != "group-2" || client.deleteReq.GetId() != "group-3" || client.addReq.GetGroupId() != "group-4" || client.removeReq.GetGroupId() != "group-5" || client.membersReq.GetGroupId() != "group-6" || client.memberGroupsReq.GetMemberId() != "user-4" {
		t.Fatalf("expected public group methods to be forwarded")
	}
}

func TestGroupsGatewayMapsErrors(t *testing.T) {
	client := &fakeGroupsClient{createErr: status.Error(codes.PermissionDenied, "nope")}
	_, err := NewGroupsGateway(client).CreateGroup(context.Background(), connect.NewRequest(&groupsv1.CreateGroupRequest{}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}
