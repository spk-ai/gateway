package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAgentsGRPCTarget         = "agents:50051"
	defaultAppsGRPCTarget           = "apps:50051"
	defaultThreadsGRPCTarget        = "threads:50051"
	defaultChatGRPCTarget           = "chat:50051"
	defaultNotificationsGRPCTarget  = "notifications:50051"
	defaultFilesGRPCTarget          = "files:50051"
	defaultAgentStateGRPCTarget     = "agent-state:50051"
	defaultTokenCountingGRPCTarget  = "token-counting:50051"
	defaultMeteringGRPCTarget       = "metering:50051"
	defaultLLMGRPCTarget            = "llm:50051"
	defaultSecretsGRPCTarget        = "secrets:50051"
	defaultTracingGRPCTarget        = "tracing:50051"
	defaultImagesGRPCTarget         = "images:50051"
	defaultZitiManagementGRPCTarget = "ziti-management:50051"
	defaultZitiLeaseRenewalInterval = 2 * time.Minute
	defaultZitiEnrollmentTimeout    = 2 * time.Minute
	defaultZitiBindTimeout          = 90 * time.Second
	defaultZitiIdentityLeaseTTL     = 5 * time.Minute
	defaultUsersGRPCTarget          = "users:50051"
	defaultOIDCProfileSource        = "userinfo"
	defaultOrganizationsGRPCTarget  = "organizations:50051"
	defaultRunnersGRPCTarget        = "runners:50051"
	defaultTerminalProxyGRPCTarget  = "terminal-proxy:50051"
	defaultExposeGRPCTarget         = "expose:50051"
	defaultEgressRulesGRPCTarget    = "egress-rules:50051"
	defaultGroupsGRPCTarget         = "groups:50051"
	defaultNetworksGRPCTarget       = "networks:50051"
)

// Config holds the runtime configuration for communicating with upstream services.
type Config struct {
	AgentsGRPCTarget         string
	AppsGRPCTarget           string
	ThreadsGRPCTarget        string
	ChatGRPCTarget           string
	NotificationsGRPCTarget  string
	FilesGRPCTarget          string
	AgentStateGRPCTarget     string
	TokenCountingGRPCTarget  string
	MeteringGRPCTarget       string
	LLMGRPCTarget            string
	SecretsGRPCTarget        string
	TracingGRPCTarget        string
	ImagesGRPCTarget         string
	ZitiEnabled              bool
	ZitiLeaseRenewalInterval time.Duration
	ZitiEnrollmentTimeout    time.Duration
	ZitiBindTimeout          time.Duration
	// ZitiIdentityLeaseTTL must equal ziti-management's SERVICE_IDENTITY_LEASE_TTL
	// (default 5m); it is only used to validate the lease budget.
	ZitiIdentityLeaseTTL     time.Duration
	ZitiManagementGRPCTarget string
	OIDCIssuerURL            string
	OIDCClientID             string
	OIDCProfileSource        string
	OIDCClaimName            string
	OIDCClaimEmail           string
	OIDCClaimPicture         string
	OIDCClaimPreferredUser   string
	ClusterAdminToken        string
	ClusterAdminIdentityID   string
	UsersGRPCTarget          string
	OrganizationsGRPCTarget  string
	RunnersGRPCTarget        string
	TerminalProxyGRPCTarget  string
	ExposeGRPCTarget         string
	EgressRulesGRPCTarget    string
	GroupsGRPCTarget         string
	NetworksGRPCTarget       string
}

// LoadConfigFromEnv constructs a Config instance from environment variables.
func LoadConfigFromEnv() (*Config, error) {
	zitiEnabled, err := envBool("ZITI_ENABLED")
	if err != nil {
		return nil, err
	}

	zitiLeaseRenewalInterval, err := envDuration("ZITI_LEASE_RENEWAL_INTERVAL", defaultZitiLeaseRenewalInterval)
	if err != nil {
		return nil, err
	}
	if zitiLeaseRenewalInterval <= 0 {
		return nil, fmt.Errorf("ZITI_LEASE_RENEWAL_INTERVAL must be positive")
	}

	zitiEnrollmentTimeout, err := envDuration("ZITI_ENROLLMENT_TIMEOUT", defaultZitiEnrollmentTimeout)
	if err != nil {
		return nil, err
	}
	if zitiEnrollmentTimeout <= 0 {
		return nil, fmt.Errorf("ZITI_ENROLLMENT_TIMEOUT must be positive")
	}

	zitiBindTimeout, err := envDuration("ZITI_BIND_TIMEOUT", defaultZitiBindTimeout)
	if err != nil {
		return nil, err
	}
	if zitiBindTimeout <= 0 {
		return nil, fmt.Errorf("ZITI_BIND_TIMEOUT must be positive")
	}

	zitiIdentityLeaseTTL, err := envDuration("ZITI_SERVICE_IDENTITY_LEASE_TTL", defaultZitiIdentityLeaseTTL)
	if err != nil {
		return nil, err
	}
	if zitiIdentityLeaseTTL <= 0 {
		return nil, fmt.Errorf("ZITI_SERVICE_IDENTITY_LEASE_TTL must be positive")
	}
	if zitiEnabled {
		if err := validateZitiTimeouts(zitiEnrollmentTimeout, zitiBindTimeout, zitiLeaseRenewalInterval, zitiIdentityLeaseTTL); err != nil {
			return nil, err
		}
	}

	// Where provisioning-time profile claims come from. Defaults to the UserInfo
	// endpoint; "token" reads them from the access token, which is required for
	// IdPs that issue audience-restricted tokens (UserInfo rejects any token
	// carrying an `aud` claim).
	oidcProfileSource := strings.TrimSpace(os.Getenv("OIDC_PROFILE_SOURCE"))
	if oidcProfileSource == "" {
		oidcProfileSource = defaultOIDCProfileSource
	}
	if oidcProfileSource != "userinfo" && oidcProfileSource != "token" {
		return nil, fmt.Errorf("OIDC_PROFILE_SOURCE must be either \"userinfo\" or \"token\"")
	}

	clusterAdminToken := strings.TrimSpace(os.Getenv("CLUSTER_ADMIN_TOKEN"))
	clusterAdminIdentityID := strings.TrimSpace(os.Getenv("CLUSTER_ADMIN_IDENTITY_ID"))
	if (clusterAdminToken == "") != (clusterAdminIdentityID == "") {
		return nil, fmt.Errorf("CLUSTER_ADMIN_TOKEN and CLUSTER_ADMIN_IDENTITY_ID must both be set or both be empty")
	}

	return &Config{
		AgentsGRPCTarget:         envOrDefault("AGENTS_GRPC_TARGET", defaultAgentsGRPCTarget),
		AppsGRPCTarget:           envOrDefault("APPS_GRPC_TARGET", defaultAppsGRPCTarget),
		ThreadsGRPCTarget:        envOrDefault("THREADS_GRPC_TARGET", defaultThreadsGRPCTarget),
		ChatGRPCTarget:           envOrDefault("CHAT_GRPC_TARGET", defaultChatGRPCTarget),
		NotificationsGRPCTarget:  envOrDefault("NOTIFICATIONS_GRPC_TARGET", defaultNotificationsGRPCTarget),
		FilesGRPCTarget:          envOrDefault("FILES_GRPC_TARGET", defaultFilesGRPCTarget),
		AgentStateGRPCTarget:     envOrDefault("AGENT_STATE_GRPC_TARGET", defaultAgentStateGRPCTarget),
		TokenCountingGRPCTarget:  envOrDefault("TOKEN_COUNTING_GRPC_TARGET", defaultTokenCountingGRPCTarget),
		MeteringGRPCTarget:       envOrDefault("METERING_GRPC_TARGET", defaultMeteringGRPCTarget),
		LLMGRPCTarget:            envOrDefault("LLM_GRPC_TARGET", defaultLLMGRPCTarget),
		SecretsGRPCTarget:        envOrDefault("SECRETS_GRPC_TARGET", defaultSecretsGRPCTarget),
		ImagesGRPCTarget:         envOrDefault("IMAGES_GRPC_TARGET", defaultImagesGRPCTarget),
		TracingGRPCTarget:        envOrDefault("TRACING_GRPC_TARGET", defaultTracingGRPCTarget),
		ZitiEnabled:              zitiEnabled,
		ZitiLeaseRenewalInterval: zitiLeaseRenewalInterval,
		ZitiEnrollmentTimeout:    zitiEnrollmentTimeout,
		ZitiBindTimeout:          zitiBindTimeout,
		ZitiIdentityLeaseTTL:     zitiIdentityLeaseTTL,
		ZitiManagementGRPCTarget: envOrDefault("ZITI_MANAGEMENT_GRPC_TARGET", defaultZitiManagementGRPCTarget),
		OIDCIssuerURL:            strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")),
		OIDCClientID:             strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID")),
		OIDCProfileSource:        oidcProfileSource,
		OIDCClaimName:            strings.TrimSpace(os.Getenv("OIDC_CLAIM_NAME")),
		OIDCClaimEmail:           strings.TrimSpace(os.Getenv("OIDC_CLAIM_EMAIL")),
		OIDCClaimPicture:         strings.TrimSpace(os.Getenv("OIDC_CLAIM_PICTURE")),
		OIDCClaimPreferredUser:   strings.TrimSpace(os.Getenv("OIDC_CLAIM_PREFERRED_USERNAME")),
		ClusterAdminToken:        clusterAdminToken,
		ClusterAdminIdentityID:   clusterAdminIdentityID,
		UsersGRPCTarget:          envOrDefault("USERS_GRPC_TARGET", defaultUsersGRPCTarget),
		OrganizationsGRPCTarget:  envOrDefault("ORGANIZATIONS_GRPC_TARGET", defaultOrganizationsGRPCTarget),
		RunnersGRPCTarget:        envOrDefault("RUNNERS_GRPC_TARGET", defaultRunnersGRPCTarget),
		TerminalProxyGRPCTarget:  envOrDefault("TERMINAL_PROXY_GRPC_TARGET", defaultTerminalProxyGRPCTarget),
		ExposeGRPCTarget:         envOrDefault("EXPOSE_GRPC_TARGET", defaultExposeGRPCTarget),
		EgressRulesGRPCTarget:    envOrDefault("EGRESS_RULES_GRPC_TARGET", defaultEgressRulesGRPCTarget),
		GroupsGRPCTarget:         envOrDefault("GROUPS_GRPC_TARGET", defaultGroupsGRPCTarget),
		NetworksGRPCTarget:       envOrDefault("NETWORKS_GRPC_TARGET", defaultNetworksGRPCTarget),
	}, nil
}

// zitiLeaseSafetyMargin covers the extension calls themselves (the manager
// bounds each attempt), RPC latency and clock skew between the gateway and
// ziti-management.
const zitiLeaseSafetyMargin = 30 * time.Second

// validateZitiTimeouts relates the Ziti timeouts to each other and to
// ziti-management's lease. It applies only when Ziti is enabled; the
// individual values are validated regardless.
//
// One enrollment attempt must fit a whole bind. The lease budget keeps a newly
// issued service identity leased until its first extension: ziti-management
// starts the lease when it issues the identity; the gateway extends it once a
// terminator is established, at most ZITI_BIND_TIMEOUT later, and if that
// extension fails transiently the next attempt is up to
// ZITI_LEASE_RENEWAL_INTERVAL after it.
func validateZitiTimeouts(enrollmentTimeout, bindTimeout, renewalInterval, leaseTTL time.Duration) error {
	if enrollmentTimeout < bindTimeout {
		return fmt.Errorf("ZITI_ENROLLMENT_TIMEOUT (%s) must be at least ZITI_BIND_TIMEOUT (%s)", enrollmentTimeout, bindTimeout)
	}
	if bindTimeout+renewalInterval+zitiLeaseSafetyMargin >= leaseTTL {
		return fmt.Errorf(
			"ZITI_BIND_TIMEOUT (%s) + ZITI_LEASE_RENEWAL_INTERVAL (%s) + %s safety margin must be less than ZITI_SERVICE_IDENTITY_LEASE_TTL (%s)",
			bindTimeout, renewalInterval, zitiLeaseSafetyMargin, leaseTTL,
		)
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}

	return parsed, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", name, err)
	}

	return parsed, nil
}
