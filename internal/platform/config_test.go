package platform

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("AGENTS_GRPC_TARGET", "agents:50052")
	t.Setenv("APPS_GRPC_TARGET", "apps:50062")
	t.Setenv("THREADS_GRPC_TARGET", "threads:50053")
	t.Setenv("CHAT_GRPC_TARGET", "chat:50054")
	t.Setenv("NOTIFICATIONS_GRPC_TARGET", "notifications:50055")
	t.Setenv("FILES_GRPC_TARGET", "files:50053")
	t.Setenv("AGENT_STATE_GRPC_TARGET", "agent-state:50056")
	t.Setenv("TOKEN_COUNTING_GRPC_TARGET", "token-counting:50057")
	t.Setenv("METERING_GRPC_TARGET", "metering:50064")
	t.Setenv("LLM_GRPC_TARGET", "llm:50058")
	t.Setenv("SECRETS_GRPC_TARGET", "secrets:50059")
	t.Setenv("USERS_GRPC_TARGET", "users:50060")
	t.Setenv("ORGANIZATIONS_GRPC_TARGET", "organizations:50062")
	t.Setenv("RUNNERS_GRPC_TARGET", "runners:50063")
	t.Setenv("TERMINAL_PROXY_GRPC_TARGET", "terminal-proxy:50070")
	t.Setenv("EXPOSE_GRPC_TARGET", "expose:50065")
	t.Setenv("EGRESS_RULES_GRPC_TARGET", "egress-rules:50066")
	t.Setenv("GROUPS_GRPC_TARGET", "groups:50067")
	t.Setenv("NETWORKS_GRPC_TARGET", "networks:50068")
	t.Setenv("ZITI_ENABLED", "true")
	t.Setenv("ZITI_LEASE_RENEWAL_INTERVAL", "3m")
	t.Setenv("ZITI_ENROLLMENT_TIMEOUT", "90s")
	t.Setenv("ZITI_BIND_TIMEOUT", "45s")
	t.Setenv("ZITI_SERVICE_IDENTITY_LEASE_TTL", "6m")
	t.Setenv("ZITI_MANAGEMENT_GRPC_TARGET", "ziti-management:50061")
	t.Setenv("OIDC_ISSUER_URL", "https://issuer.example.com")
	t.Setenv("OIDC_CLIENT_ID", "client-123")
	t.Setenv("CLUSTER_ADMIN_TOKEN", "cluster-token")
	t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "cluster-identity")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := cfg.AgentsGRPCTarget; got != "agents:50052" {
		t.Fatalf("unexpected agents grpc target: %s", got)
	}

	if got := cfg.AppsGRPCTarget; got != "apps:50062" {
		t.Fatalf("unexpected apps grpc target: %s", got)
	}

	if got := cfg.ThreadsGRPCTarget; got != "threads:50053" {
		t.Fatalf("unexpected threads grpc target: %s", got)
	}

	if got := cfg.ChatGRPCTarget; got != "chat:50054" {
		t.Fatalf("unexpected chat grpc target: %s", got)
	}

	if got := cfg.NotificationsGRPCTarget; got != "notifications:50055" {
		t.Fatalf("unexpected notifications grpc target: %s", got)
	}

	if got := cfg.FilesGRPCTarget; got != "files:50053" {
		t.Fatalf("unexpected files grpc target: %s", got)
	}

	if got := cfg.AgentStateGRPCTarget; got != "agent-state:50056" {
		t.Fatalf("unexpected agent state grpc target: %s", got)
	}

	if got := cfg.TokenCountingGRPCTarget; got != "token-counting:50057" {
		t.Fatalf("unexpected token counting grpc target: %s", got)
	}

	if got := cfg.MeteringGRPCTarget; got != "metering:50064" {
		t.Fatalf("unexpected metering grpc target: %s", got)
	}

	if got := cfg.LLMGRPCTarget; got != "llm:50058" {
		t.Fatalf("unexpected llm grpc target: %s", got)
	}

	if got := cfg.SecretsGRPCTarget; got != "secrets:50059" {
		t.Fatalf("unexpected secrets grpc target: %s", got)
	}

	if got := cfg.UsersGRPCTarget; got != "users:50060" {
		t.Fatalf("unexpected users grpc target: %s", got)
	}

	if got := cfg.OrganizationsGRPCTarget; got != "organizations:50062" {
		t.Fatalf("unexpected organizations grpc target: %s", got)
	}

	if got := cfg.RunnersGRPCTarget; got != "runners:50063" {
		t.Fatalf("unexpected runners grpc target: %s", got)
	}

	if got := cfg.TerminalProxyGRPCTarget; got != "terminal-proxy:50070" {
		t.Fatalf("unexpected terminal proxy grpc target: %s", got)
	}

	if got := cfg.ExposeGRPCTarget; got != "expose:50065" {
		t.Fatalf("unexpected expose grpc target: %s", got)
	}

	if got := cfg.EgressRulesGRPCTarget; got != "egress-rules:50066" {
		t.Fatalf("unexpected egress rules grpc target: %s", got)
	}

	if got := cfg.GroupsGRPCTarget; got != "groups:50067" {
		t.Fatalf("unexpected groups grpc target: %s", got)
	}

	if got := cfg.NetworksGRPCTarget; got != "networks:50068" {
		t.Fatalf("unexpected networks grpc target: %s", got)
	}

	if !cfg.ZitiEnabled {
		t.Fatalf("expected ziti to be enabled")
	}

	if got := cfg.ZitiLeaseRenewalInterval; got != 3*time.Minute {
		t.Fatalf("unexpected ziti lease renewal interval: %s", got)
	}

	if got := cfg.ZitiEnrollmentTimeout; got != 90*time.Second {
		t.Fatalf("unexpected ziti enrollment timeout: %s", got)
	}

	if got := cfg.ZitiBindTimeout; got != 45*time.Second {
		t.Fatalf("unexpected ziti bind timeout: %s", got)
	}

	if got := cfg.ZitiIdentityLeaseTTL; got != 6*time.Minute {
		t.Fatalf("unexpected ziti identity lease ttl: %s", got)
	}

	if got := cfg.ZitiManagementGRPCTarget; got != "ziti-management:50061" {
		t.Fatalf("unexpected ziti management grpc target: %s", got)
	}

	if got := cfg.OIDCIssuerURL; got != "https://issuer.example.com" {
		t.Fatalf("unexpected oidc issuer url: %s", got)
	}

	if got := cfg.OIDCClientID; got != "client-123" {
		t.Fatalf("unexpected oidc client id: %s", got)
	}

	if got := cfg.ClusterAdminToken; got != "cluster-token" {
		t.Fatalf("unexpected cluster admin token: %s", got)
	}
	if got := cfg.ClusterAdminIdentityID; got != "cluster-identity" {
		t.Fatalf("unexpected cluster admin identity id: %s", got)
	}
}

func TestLoadConfigFromEnvMissingAgentsGRPC(t *testing.T) {
	t.Setenv("AGENTS_GRPC_TARGET", "")
	t.Setenv("CLUSTER_ADMIN_TOKEN", "")
	t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AgentsGRPCTarget != defaultAgentsGRPCTarget {
		t.Fatalf("unexpected agents grpc target: %s", cfg.AgentsGRPCTarget)
	}
}

func TestLoadConfigFromEnvAllDefaults(t *testing.T) {
	t.Setenv("AGENTS_GRPC_TARGET", "")
	t.Setenv("APPS_GRPC_TARGET", "")
	t.Setenv("THREADS_GRPC_TARGET", "")
	t.Setenv("CHAT_GRPC_TARGET", "")
	t.Setenv("NOTIFICATIONS_GRPC_TARGET", "")
	t.Setenv("FILES_GRPC_TARGET", "")
	t.Setenv("AGENT_STATE_GRPC_TARGET", "")
	t.Setenv("TOKEN_COUNTING_GRPC_TARGET", "")
	t.Setenv("METERING_GRPC_TARGET", "")
	t.Setenv("LLM_GRPC_TARGET", "")
	t.Setenv("SECRETS_GRPC_TARGET", "")
	t.Setenv("USERS_GRPC_TARGET", "")
	t.Setenv("ORGANIZATIONS_GRPC_TARGET", "")
	t.Setenv("RUNNERS_GRPC_TARGET", "")
	t.Setenv("TERMINAL_PROXY_GRPC_TARGET", "")
	t.Setenv("EXPOSE_GRPC_TARGET", "")
	t.Setenv("EGRESS_RULES_GRPC_TARGET", "")
	t.Setenv("GROUPS_GRPC_TARGET", "")
	t.Setenv("NETWORKS_GRPC_TARGET", "")
	t.Setenv("ZITI_ENABLED", "")
	t.Setenv("ZITI_LEASE_RENEWAL_INTERVAL", "")
	t.Setenv("ZITI_ENROLLMENT_TIMEOUT", "")
	t.Setenv("ZITI_BIND_TIMEOUT", "")
	t.Setenv("ZITI_SERVICE_IDENTITY_LEASE_TTL", "")
	t.Setenv("ZITI_MANAGEMENT_GRPC_TARGET", "")
	t.Setenv("OIDC_ISSUER_URL", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("CLUSTER_ADMIN_TOKEN", "")
	t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AgentsGRPCTarget != defaultAgentsGRPCTarget {
		t.Fatalf("unexpected agents grpc target: %s", cfg.AgentsGRPCTarget)
	}
	if cfg.AppsGRPCTarget != defaultAppsGRPCTarget {
		t.Fatalf("unexpected apps grpc target: %s", cfg.AppsGRPCTarget)
	}
	if cfg.ThreadsGRPCTarget != defaultThreadsGRPCTarget {
		t.Fatalf("unexpected threads grpc target: %s", cfg.ThreadsGRPCTarget)
	}
	if cfg.ChatGRPCTarget != defaultChatGRPCTarget {
		t.Fatalf("unexpected chat grpc target: %s", cfg.ChatGRPCTarget)
	}
	if cfg.NotificationsGRPCTarget != defaultNotificationsGRPCTarget {
		t.Fatalf("unexpected notifications grpc target: %s", cfg.NotificationsGRPCTarget)
	}
	if cfg.FilesGRPCTarget != defaultFilesGRPCTarget {
		t.Fatalf("unexpected files grpc target: %s", cfg.FilesGRPCTarget)
	}
	if cfg.AgentStateGRPCTarget != defaultAgentStateGRPCTarget {
		t.Fatalf("unexpected agent state grpc target: %s", cfg.AgentStateGRPCTarget)
	}
	if cfg.TokenCountingGRPCTarget != defaultTokenCountingGRPCTarget {
		t.Fatalf("unexpected token counting grpc target: %s", cfg.TokenCountingGRPCTarget)
	}
	if cfg.MeteringGRPCTarget != defaultMeteringGRPCTarget {
		t.Fatalf("unexpected metering grpc target: %s", cfg.MeteringGRPCTarget)
	}
	if cfg.LLMGRPCTarget != defaultLLMGRPCTarget {
		t.Fatalf("unexpected llm grpc target: %s", cfg.LLMGRPCTarget)
	}
	if cfg.SecretsGRPCTarget != defaultSecretsGRPCTarget {
		t.Fatalf("unexpected secrets grpc target: %s", cfg.SecretsGRPCTarget)
	}
	if cfg.UsersGRPCTarget != defaultUsersGRPCTarget {
		t.Fatalf("unexpected users grpc target: %s", cfg.UsersGRPCTarget)
	}
	if cfg.OrganizationsGRPCTarget != defaultOrganizationsGRPCTarget {
		t.Fatalf("unexpected organizations grpc target: %s", cfg.OrganizationsGRPCTarget)
	}
	if cfg.RunnersGRPCTarget != defaultRunnersGRPCTarget {
		t.Fatalf("unexpected runners grpc target: %s", cfg.RunnersGRPCTarget)
	}
	if cfg.TerminalProxyGRPCTarget != defaultTerminalProxyGRPCTarget {
		t.Fatalf("unexpected terminal proxy grpc target: %s", cfg.TerminalProxyGRPCTarget)
	}
	if cfg.ExposeGRPCTarget != defaultExposeGRPCTarget {
		t.Fatalf("unexpected expose grpc target: %s", cfg.ExposeGRPCTarget)
	}
	if cfg.EgressRulesGRPCTarget != defaultEgressRulesGRPCTarget {
		t.Fatalf("unexpected egress rules grpc target: %s", cfg.EgressRulesGRPCTarget)
	}
	if cfg.GroupsGRPCTarget != defaultGroupsGRPCTarget {
		t.Fatalf("unexpected groups grpc target: %s", cfg.GroupsGRPCTarget)
	}
	if cfg.NetworksGRPCTarget != defaultNetworksGRPCTarget {
		t.Fatalf("unexpected networks grpc target: %s", cfg.NetworksGRPCTarget)
	}
	if cfg.ZitiEnabled {
		t.Fatalf("expected ziti to be disabled")
	}
	if cfg.ZitiLeaseRenewalInterval != defaultZitiLeaseRenewalInterval {
		t.Fatalf("unexpected ziti lease renewal interval: %s", cfg.ZitiLeaseRenewalInterval)
	}
	if cfg.ZitiEnrollmentTimeout != defaultZitiEnrollmentTimeout {
		t.Fatalf("unexpected ziti enrollment timeout: %s", cfg.ZitiEnrollmentTimeout)
	}
	if cfg.ZitiBindTimeout != 90*time.Second {
		t.Fatalf("unexpected ziti bind timeout: %s", cfg.ZitiBindTimeout)
	}
	if cfg.ZitiIdentityLeaseTTL != 5*time.Minute {
		t.Fatalf("unexpected ziti identity lease ttl: %s", cfg.ZitiIdentityLeaseTTL)
	}
	if cfg.ZitiManagementGRPCTarget != defaultZitiManagementGRPCTarget {
		t.Fatalf("unexpected ziti management grpc target: %s", cfg.ZitiManagementGRPCTarget)
	}
	if cfg.OIDCIssuerURL != "" {
		t.Fatalf("unexpected oidc issuer url: %s", cfg.OIDCIssuerURL)
	}
	if cfg.OIDCClientID != "" {
		t.Fatalf("unexpected oidc client id: %s", cfg.OIDCClientID)
	}
	if cfg.ClusterAdminToken != "" {
		t.Fatalf("unexpected cluster admin token: %s", cfg.ClusterAdminToken)
	}
	if cfg.ClusterAdminIdentityID != "" {
		t.Fatalf("unexpected cluster admin identity id: %s", cfg.ClusterAdminIdentityID)
	}
}

func TestLoadConfigFromEnvClusterAdminPairValidation(t *testing.T) {
	t.Run("both set", func(t *testing.T) {
		t.Setenv("CLUSTER_ADMIN_TOKEN", "cluster-token")
		t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "cluster-identity")

		cfg, err := LoadConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ClusterAdminToken != "cluster-token" {
			t.Fatalf("unexpected cluster admin token: %s", cfg.ClusterAdminToken)
		}
		if cfg.ClusterAdminIdentityID != "cluster-identity" {
			t.Fatalf("unexpected cluster admin identity id: %s", cfg.ClusterAdminIdentityID)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		t.Setenv("CLUSTER_ADMIN_TOKEN", "")
		t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

		cfg, err := LoadConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ClusterAdminToken != "" {
			t.Fatalf("unexpected cluster admin token: %s", cfg.ClusterAdminToken)
		}
		if cfg.ClusterAdminIdentityID != "" {
			t.Fatalf("unexpected cluster admin identity id: %s", cfg.ClusterAdminIdentityID)
		}
	})

	t.Run("missing identity", func(t *testing.T) {
		t.Setenv("CLUSTER_ADMIN_TOKEN", "cluster-token")
		t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

		_, err := LoadConfigFromEnv()
		if err == nil {
			t.Fatalf("expected error for missing cluster admin identity id")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("CLUSTER_ADMIN_TOKEN", "")
		t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "cluster-identity")

		_, err := LoadConfigFromEnv()
		if err == nil {
			t.Fatalf("expected error for missing cluster admin token")
		}
	})
}

func TestLoadConfigFromEnvInvalidZitiLeaseRenewalInterval(t *testing.T) {
	t.Setenv("ZITI_LEASE_RENEWAL_INTERVAL", "0s")
	t.Setenv("CLUSTER_ADMIN_TOKEN", "")
	t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid ziti lease renewal interval")
	}
}

func TestLoadConfigFromEnvInvalidZitiEnrollmentTimeout(t *testing.T) {
	t.Setenv("ZITI_ENROLLMENT_TIMEOUT", "0s")
	t.Setenv("CLUSTER_ADMIN_TOKEN", "")
	t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid ziti enrollment timeout")
	}
}

func TestLoadConfigFromEnvZitiBindTimeoutValidation(t *testing.T) {
	tests := []struct {
		name              string
		bindTimeout       string
		enrollmentTimeout string
		renewalInterval   string
		leaseTTL          string
		wantErr           bool
	}{
		{name: "defaults fit the lease", wantErr: false},
		{name: "largest bind timeout within the lease", bindTimeout: "149s", enrollmentTimeout: "5m", wantErr: false},
		{name: "bind timeout reaching the lease budget", bindTimeout: "150s", enrollmentTimeout: "5m", wantErr: true},
		{name: "bind timeout beyond the lease", bindTimeout: "3m", enrollmentTimeout: "5m", wantErr: true},
		{name: "renewal interval consuming the margin", renewalInterval: "3m", wantErr: true},
		{name: "lease shorter than ziti-management default", leaseTTL: "3m", wantErr: true},
		{name: "longer lease admits longer bind", bindTimeout: "3m", enrollmentTimeout: "5m", leaseTTL: "10m", wantErr: false},
		{name: "enrollment shorter than bind", bindTimeout: "60s", enrollmentTimeout: "59s", wantErr: true},
		{name: "zero bind timeout", bindTimeout: "0s", wantErr: true},
		{name: "negative bind timeout", bindTimeout: "-1s", wantErr: true},
		{name: "invalid bind timeout", bindTimeout: "soon", wantErr: true},
		{name: "zero lease", leaseTTL: "0s", wantErr: true},
		{name: "invalid lease", leaseTTL: "forever", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ZITI_ENABLED", "true")
			t.Setenv("ZITI_BIND_TIMEOUT", tt.bindTimeout)
			t.Setenv("ZITI_ENROLLMENT_TIMEOUT", tt.enrollmentTimeout)
			t.Setenv("ZITI_LEASE_RENEWAL_INTERVAL", tt.renewalInterval)
			t.Setenv("ZITI_SERVICE_IDENTITY_LEASE_TTL", tt.leaseTTL)
			t.Setenv("CLUSTER_ADMIN_TOKEN", "")
			t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

			_, err := LoadConfigFromEnv()
			if tt.wantErr && err == nil {
				t.Fatalf("expected configuration to be rejected")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadConfigFromEnvZitiDisabledSkipsTimeoutRelations(t *testing.T) {
	tests := []struct {
		name              string
		zitiEnabled       string
		bindTimeout       string
		enrollmentTimeout string
		renewalInterval   string
		leaseTTL          string
		wantErr           bool
	}{
		{name: "enrollment shorter than bind", bindTimeout: "60s", enrollmentTimeout: "59s"},
		{name: "bind beyond the lease", bindTimeout: "3m", enrollmentTimeout: "5m"},
		{name: "renewal interval consuming the margin", renewalInterval: "3m"},
		{name: "lease shorter than the budget", zitiEnabled: "false", leaseTTL: "3m"},
		{name: "zero bind timeout still rejected", bindTimeout: "0s", wantErr: true},
		{name: "invalid lease still rejected", zitiEnabled: "false", leaseTTL: "forever", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ZITI_ENABLED", tt.zitiEnabled)
			t.Setenv("ZITI_BIND_TIMEOUT", tt.bindTimeout)
			t.Setenv("ZITI_ENROLLMENT_TIMEOUT", tt.enrollmentTimeout)
			t.Setenv("ZITI_LEASE_RENEWAL_INTERVAL", tt.renewalInterval)
			t.Setenv("ZITI_SERVICE_IDENTITY_LEASE_TTL", tt.leaseTTL)
			t.Setenv("CLUSTER_ADMIN_TOKEN", "")
			t.Setenv("CLUSTER_ADMIN_IDENTITY_ID", "")

			cfg, err := LoadConfigFromEnv()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected configuration to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected disabled ziti to skip timeout relations: %v", err)
			}
			if cfg.ZitiEnabled {
				t.Fatalf("expected ziti to be disabled")
			}
		})
	}
}
