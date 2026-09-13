package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentsv1 "github.com/agynio/gateway/gen/agynio/api/agents/v1"
	appsv1 "github.com/agynio/gateway/gen/agynio/api/apps/v1"
	"github.com/agynio/gateway/internal/identity"
	"github.com/openziti/sdk-golang/ziti"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAppsClient struct {
	appsv1.AppsServiceClient
	getApp                func(ctx context.Context, req *appsv1.GetAppRequest, opts ...grpc.CallOption) (*appsv1.GetAppResponse, error)
	getInstallationBySlug func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error)
}

func (f *fakeAppsClient) CreateApp(ctx context.Context, req *appsv1.CreateAppRequest, opts ...grpc.CallOption) (*appsv1.CreateAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) UpdateApp(ctx context.Context, req *appsv1.UpdateAppRequest, opts ...grpc.CallOption) (*appsv1.UpdateAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetApp(ctx context.Context, req *appsv1.GetAppRequest, opts ...grpc.CallOption) (*appsv1.GetAppResponse, error) {
	if f.getApp != nil {
		return f.getApp(ctx, req, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetAppBySlug(ctx context.Context, req *appsv1.GetAppBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetAppBySlugResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) ListApps(ctx context.Context, req *appsv1.ListAppsRequest, opts ...grpc.CallOption) (*appsv1.ListAppsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) DeleteApp(ctx context.Context, req *appsv1.DeleteAppRequest, opts ...grpc.CallOption) (*appsv1.DeleteAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetAppProfile(ctx context.Context, req *appsv1.GetAppProfileRequest, opts ...grpc.CallOption) (*appsv1.GetAppProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) ValidateServiceToken(ctx context.Context, req *appsv1.ValidateServiceTokenRequest, opts ...grpc.CallOption) (*appsv1.ValidateServiceTokenResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) EnrollApp(ctx context.Context, req *appsv1.EnrollAppRequest, opts ...grpc.CallOption) (*appsv1.EnrollAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) InstallApp(ctx context.Context, req *appsv1.InstallAppRequest, opts ...grpc.CallOption) (*appsv1.InstallAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetInstallation(ctx context.Context, req *appsv1.GetInstallationRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetInstallationBySlug(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
	if f.getInstallationBySlug != nil {
		return f.getInstallationBySlug(ctx, req, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetInstallationByIdentityId(ctx context.Context, req *appsv1.GetInstallationByIdentityIdRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationByIdentityIdResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) ListInstallations(ctx context.Context, req *appsv1.ListInstallationsRequest, opts ...grpc.CallOption) (*appsv1.ListInstallationsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) UpdateInstallation(ctx context.Context, req *appsv1.UpdateInstallationRequest, opts ...grpc.CallOption) (*appsv1.UpdateInstallationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) UninstallApp(ctx context.Context, req *appsv1.UninstallAppRequest, opts ...grpc.CallOption) (*appsv1.UninstallAppResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) GetInstallationConfiguration(ctx context.Context, req *appsv1.GetInstallationConfigurationRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationConfigurationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) ReportInstallationStatus(ctx context.Context, req *appsv1.ReportInstallationStatusRequest, opts ...grpc.CallOption) (*appsv1.ReportInstallationStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) AppendInstallationAuditLogEntry(ctx context.Context, req *appsv1.AppendInstallationAuditLogEntryRequest, opts ...grpc.CallOption) (*appsv1.AppendInstallationAuditLogEntryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAppsClient) ReportConfigurationSchema(ctx context.Context, req *appsv1.ReportConfigurationSchemaRequest, opts ...grpc.CallOption) (*appsv1.ReportConfigurationSchemaResponse, error) {
	return &appsv1.ReportConfigurationSchemaResponse{}, nil
}

func (f *fakeAppsClient) DeleteOrganizationResources(ctx context.Context, req *appsv1.DeleteOrganizationResourcesRequest, opts ...grpc.CallOption) (*appsv1.DeleteOrganizationResourcesResponse, error) {
	return &appsv1.DeleteOrganizationResourcesResponse{}, nil
}

func (f *fakeAppsClient) ListInstallationAuditLogEntries(ctx context.Context, req *appsv1.ListInstallationAuditLogEntriesRequest, opts ...grpc.CallOption) (*appsv1.ListInstallationAuditLogEntriesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// The generated client is embedded rather than restated. This fake answers one
// RPC and used to spell out the other seventy as Unimplemented, so every RPC
// added to the service broke this file -- which is how main came to be red.
// Reaching an unstubbed RPC now panics on the nil embedded interface, which is
// what a test that got there by accident wants anyway.
type fakeAgentsClient struct {
	agentsv1.AgentsServiceClient
	resolveAgentIdentity func(ctx context.Context, req *agentsv1.ResolveAgentIdentityRequest, opts ...grpc.CallOption) (*agentsv1.ResolveAgentIdentityResponse, error)
	getSandbox           func(ctx context.Context, req *agentsv1.GetSandboxRequest, opts ...grpc.CallOption) (*agentsv1.GetSandboxResponse, error)
}

func (f *fakeAgentsClient) ResolveAgentIdentity(ctx context.Context, req *agentsv1.ResolveAgentIdentityRequest, opts ...grpc.CallOption) (*agentsv1.ResolveAgentIdentityResponse, error) {
	if f.resolveAgentIdentity != nil {
		return f.resolveAgentIdentity(ctx, req, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeAgentsClient) GetSandbox(ctx context.Context, req *agentsv1.GetSandboxRequest, opts ...grpc.CallOption) (*agentsv1.GetSandboxResponse, error) {
	if f.getSandbox != nil {
		return f.getSandbox(ctx, req, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func TestParseAppProxyPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		expSlug   string
		expMethod string
		wantErr   bool
	}{
		{name: "simple", path: "/apps/my-app/run", expSlug: "my-app", expMethod: "run"},
		{name: "nested", path: "/apps/my-app/run/task", expSlug: "my-app", expMethod: "run/task"},
		{name: "invalid prefix", path: "/app/my-app/run", wantErr: true},
		{name: "missing method", path: "/apps/my-app", wantErr: true},
		{name: "missing slug", path: "/apps//run", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, method, err := parseAppProxyPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if slug != tt.expSlug {
				t.Fatalf("expected slug %q, got %q", tt.expSlug, slug)
			}
			if method != tt.expMethod {
				t.Fatalf("expected method %q, got %q", tt.expMethod, method)
			}
		})
	}
}

func TestResolveInstallationCache(t *testing.T) {
	var installationCalls int
	var appCalls int
	appsClient := &fakeAppsClient{
		getInstallationBySlug: func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
			installationCalls++
			return &appsv1.GetInstallationBySlugResponse{Installation: &appsv1.Installation{
				Meta:  &appsv1.EntityMeta{Id: "inst-" + req.OrganizationId},
				AppId: "app-" + req.OrganizationId,
			}}, nil
		},
		getApp: func(ctx context.Context, req *appsv1.GetAppRequest, opts ...grpc.CallOption) (*appsv1.GetAppResponse, error) {
			appCalls++
			return &appsv1.GetAppResponse{App: &appsv1.App{Slug: "slug-" + req.Id}}, nil
		},
	}
	handler := &AppProxyHandler{
		apps:     appsClient,
		cache:    make(map[string]cachedInstallation),
		cacheTTL: time.Minute,
	}

	ctx := context.Background()
	resolved, err := handler.resolveInstallation(ctx, "org-1", "slug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.serviceName != "app-slug-app-org-1" {
		t.Fatalf("expected service name %q, got %q", "app-slug-app-org-1", resolved.serviceName)
	}
	if resolved.installationID != "inst-org-1" {
		t.Fatalf("expected installation id %q, got %q", "inst-org-1", resolved.installationID)
	}

	resolved, err = handler.resolveInstallation(ctx, "org-1", "slug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.serviceName != "app-slug-app-org-1" {
		t.Fatalf("expected service name %q, got %q", "app-slug-app-org-1", resolved.serviceName)
	}
	if installationCalls != 1 {
		t.Fatalf("expected installation lookup once, got %d", installationCalls)
	}
	if appCalls != 1 {
		t.Fatalf("expected app lookup once, got %d", appCalls)
	}

	t.Run("different organization", func(t *testing.T) {
		resolved, err = handler.resolveInstallation(ctx, "org-2", "slug")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.serviceName != "app-slug-app-org-2" {
			t.Fatalf("expected service name %q, got %q", "app-slug-app-org-2", resolved.serviceName)
		}
		if resolved.installationID != "inst-org-2" {
			t.Fatalf("expected installation id %q, got %q", "inst-org-2", resolved.installationID)
		}
		if installationCalls != 2 {
			t.Fatalf("expected installation lookup twice, got %d", installationCalls)
		}
		if appCalls != 2 {
			t.Fatalf("expected app lookup twice, got %d", appCalls)
		}
	})
}

func TestResolveInstallationErrors(t *testing.T) {
	t.Run("installation lookup error", func(t *testing.T) {
		handler := &AppProxyHandler{
			apps: &fakeAppsClient{
				getInstallationBySlug: func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
					return nil, status.Error(codes.NotFound, "missing")
				},
			},
			cache:    make(map[string]cachedInstallation),
			cacheTTL: time.Minute,
		}

		_, err := handler.resolveInstallation(context.Background(), "org", "slug")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing installation", func(t *testing.T) {
		handler := &AppProxyHandler{
			apps: &fakeAppsClient{
				getInstallationBySlug: func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
					return &appsv1.GetInstallationBySlugResponse{}, nil
				},
			},
			cache:    make(map[string]cachedInstallation),
			cacheTTL: time.Minute,
		}

		_, err := handler.resolveInstallation(context.Background(), "org", "slug")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("get app error", func(t *testing.T) {
		handler := &AppProxyHandler{
			apps: &fakeAppsClient{
				getInstallationBySlug: func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
					return &appsv1.GetInstallationBySlugResponse{Installation: &appsv1.Installation{
						Meta:  &appsv1.EntityMeta{Id: "inst"},
						AppId: "app",
					}}, nil
				},
				getApp: func(ctx context.Context, req *appsv1.GetAppRequest, opts ...grpc.CallOption) (*appsv1.GetAppResponse, error) {
					return nil, status.Error(codes.Internal, "boom")
				},
			},
			cache:    make(map[string]cachedInstallation),
			cacheTTL: time.Minute,
		}

		_, err := handler.resolveInstallation(context.Background(), "org", "slug")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing slug", func(t *testing.T) {
		handler := &AppProxyHandler{
			apps: &fakeAppsClient{
				getInstallationBySlug: func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
					return &appsv1.GetInstallationBySlugResponse{Installation: &appsv1.Installation{
						Meta:  &appsv1.EntityMeta{Id: "inst"},
						AppId: "app",
					}}, nil
				},
				getApp: func(ctx context.Context, req *appsv1.GetAppRequest, opts ...grpc.CallOption) (*appsv1.GetAppResponse, error) {
					return &appsv1.GetAppResponse{App: &appsv1.App{Slug: " "}}, nil
				},
			},
			cache:    make(map[string]cachedInstallation),
			cacheTTL: time.Minute,
		}

		_, err := handler.resolveInstallation(context.Background(), "org", "slug")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestAppProxyHandlerServeHTTP(t *testing.T) {
	var gotIdentityID string
	var gotIdentityType string
	var gotInstallationID string
	var gotPath string
	var gotOrganizationID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentityID = r.Header.Get(identity.MetadataKeyIdentityID)
		gotIdentityType = r.Header.Get(identity.MetadataKeyIdentityType)
		gotInstallationID = r.Header.Get(identity.MetadataKeyAppInstallationID)
		gotOrganizationID = r.Header.Get(identity.MetadataKeyOrganizationID)
		gotPath = r.URL.Path
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()

	serverAddr := server.Listener.Addr().String()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_ = network
			_ = addr
			return (&net.Dialer{}).DialContext(ctx, "tcp", serverAddr)
		},
	}
	agentsClient := &fakeAgentsClient{
		resolveAgentIdentity: func(ctx context.Context, req *agentsv1.ResolveAgentIdentityRequest, opts ...grpc.CallOption) (*agentsv1.ResolveAgentIdentityResponse, error) {
			return &agentsv1.ResolveAgentIdentityResponse{OrganizationId: "org-1"}, nil
		},
	}
	appsClient := &fakeAppsClient{
		getInstallationBySlug: func(ctx context.Context, req *appsv1.GetInstallationBySlugRequest, opts ...grpc.CallOption) (*appsv1.GetInstallationBySlugResponse, error) {
			return &appsv1.GetInstallationBySlugResponse{Installation: &appsv1.Installation{
				Meta:  &appsv1.EntityMeta{Id: "inst-1"},
				AppId: "app-1",
			}}, nil
		},
		getApp: func(ctx context.Context, req *appsv1.GetAppRequest, opts ...grpc.CallOption) (*appsv1.GetAppResponse, error) {
			return &appsv1.GetAppResponse{App: &appsv1.App{Slug: "app-slug"}}, nil
		},
	}
	handler := &AppProxyHandler{
		apps:     appsClient,
		agents:   agentsClient,
		client:   &http.Client{Transport: transport},
		cache:    make(map[string]cachedInstallation),
		cacheTTL: time.Minute,
	}

	resolved := identity.ResolvedIdentity{IdentityID: "identity-1", IdentityType: identity.IdentityTypeAgent}
	req := httptest.NewRequest(http.MethodGet, "/apps/app-slug/health", nil)
	req = req.WithContext(identity.WithIdentity(req.Context(), resolved))
	resp := httptest.NewRecorder()
	req.Header.Set(identity.MetadataKeyOrganizationID, "org-override")

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}
	if resp.Header().Get("X-Test") != "ok" {
		t.Fatalf("expected response headers to be forwarded")
	}
	if resp.Body.String() != "response" {
		t.Fatalf("expected response body to be forwarded")
	}
	if gotIdentityID != resolved.IdentityID {
		t.Fatalf("expected identity header %q, got %q", resolved.IdentityID, gotIdentityID)
	}
	if gotIdentityType != string(resolved.IdentityType) {
		t.Fatalf("expected identity type header %q, got %q", resolved.IdentityType, gotIdentityType)
	}
	if gotInstallationID != "inst-1" {
		t.Fatalf("expected installation header %q, got %q", "inst-1", gotInstallationID)
	}
	if gotOrganizationID != "" {
		t.Fatalf("expected organization header stripped, got %q", gotOrganizationID)
	}
	if gotPath != "/health" {
		t.Fatalf("expected proxied path %q, got %q", "/health", gotPath)
	}
}

func TestAppProxyHandlerServeHTTPMissingOrgID(t *testing.T) {
	appsClient := &fakeAppsClient{}
	handler := &AppProxyHandler{apps: appsClient}
	req := httptest.NewRequest(http.MethodGet, "/apps/app-slug/health", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, resp.Code)
	}
}

func TestAppProxyHandlerServeHTTPUserIdentityUnsupported(t *testing.T) {
	appsClient := &fakeAppsClient{}
	agentsClient := &fakeAgentsClient{}
	handler := &AppProxyHandler{apps: appsClient, agents: agentsClient}
	req := httptest.NewRequest(http.MethodGet, "/apps/app-slug/health", nil)
	req = req.WithContext(identity.WithIdentity(req.Context(), identity.ResolvedIdentity{IdentityID: "user-1", IdentityType: identity.IdentityTypeUser}))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected status %d, got %d", http.StatusPreconditionFailed, resp.Code)
	}
}

func TestAppProxyHandlerServeHTTPInvalidPath(t *testing.T) {
	handler := &AppProxyHandler{apps: &fakeAppsClient{}}
	req := httptest.NewRequest(http.MethodGet, "/apps/app-slug", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.Code)
	}
	if resp.Header().Get("Content-Type") != problemContentType {
		t.Fatalf("expected problem content type")
	}
}

// resolveOrganizationID never dials the overlay, but the constructor requires a
// provider; nil is what an unstarted manager reports anyway.
type nilZitiContextProvider struct{}

func (nilZitiContextProvider) ZitiContext() ziti.Context { return nil }

// A sandbox reaches an app the same way an agent does: the organization comes
// from its own record, not from anything it sends.
func TestResolveOrganizationIDFromSandboxRecord(t *testing.T) {
	var askedFor string
	agents := &fakeAgentsClient{
		getSandbox: func(_ context.Context, req *agentsv1.GetSandboxRequest, _ ...grpc.CallOption) (*agentsv1.GetSandboxResponse, error) {
			askedFor = req.GetId()
			return &agentsv1.GetSandboxResponse{
				Sandbox: &agentsv1.Sandbox{OrganizationId: "org-from-sandbox"},
			}, nil
		},
	}
	handler := NewAppProxyHandler(&fakeAppsClient{}, agents, nilZitiContextProvider{}, time.Minute)

	orgID, err := handler.resolveOrganizationID(context.Background(), identity.ResolvedIdentity{
		IdentityType: identity.IdentityTypeSandbox,
		IdentityID:   "sandbox-1",
	})
	if err != nil {
		t.Fatalf("resolve organization: %v", err)
	}
	if orgID != "org-from-sandbox" {
		t.Fatalf("expected the sandbox's organization, got %q", orgID)
	}
	if askedFor != "sandbox-1" {
		t.Fatalf("expected the caller's own sandbox to be read, got %q", askedFor)
	}
}

// A sandbox whose record carries no organization must not fall through to a
// blank one, which would resolve installations in the wrong place.
func TestResolveOrganizationIDRejectsSandboxWithoutOrganization(t *testing.T) {
	agents := &fakeAgentsClient{
		getSandbox: func(context.Context, *agentsv1.GetSandboxRequest, ...grpc.CallOption) (*agentsv1.GetSandboxResponse, error) {
			return &agentsv1.GetSandboxResponse{Sandbox: &agentsv1.Sandbox{}}, nil
		},
	}
	handler := NewAppProxyHandler(&fakeAppsClient{}, agents, nilZitiContextProvider{}, time.Minute)

	if _, err := handler.resolveOrganizationID(context.Background(), identity.ResolvedIdentity{
		IdentityType: identity.IdentityTypeSandbox,
		IdentityID:   "sandbox-1",
	}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}
