package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/rs/cors"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"

	agentstatev1 "github.com/agynio/gateway/gen/agynio/api/agent_state/v1"
	agentsv1 "github.com/agynio/gateway/gen/agynio/api/agents/v1"
	appsv1 "github.com/agynio/gateway/gen/agynio/api/apps/v1"
	chatv1 "github.com/agynio/gateway/gen/agynio/api/chat/v1"
	egressv1 "github.com/agynio/gateway/gen/agynio/api/egress/v1"
	exposev1 "github.com/agynio/gateway/gen/agynio/api/expose/v1"
	filesv1 "github.com/agynio/gateway/gen/agynio/api/files/v1"
	"github.com/agynio/gateway/gen/agynio/api/gateway/v1/gatewayv1connect"
	groupsv1 "github.com/agynio/gateway/gen/agynio/api/groups/v1"
	imagesv1 "github.com/agynio/gateway/gen/agynio/api/images/v1"
	llmv1 "github.com/agynio/gateway/gen/agynio/api/llm/v1"
	meteringv1 "github.com/agynio/gateway/gen/agynio/api/metering/v1"
	networksv1 "github.com/agynio/gateway/gen/agynio/api/networks/v1"
	notificationsv1 "github.com/agynio/gateway/gen/agynio/api/notifications/v1"
	organizationsv1 "github.com/agynio/gateway/gen/agynio/api/organizations/v1"
	runnersv1 "github.com/agynio/gateway/gen/agynio/api/runners/v1"
	secretsv1 "github.com/agynio/gateway/gen/agynio/api/secrets/v1"
	terminalproxyv1 "github.com/agynio/gateway/gen/agynio/api/terminal_proxy/v1"
	threadsv1 "github.com/agynio/gateway/gen/agynio/api/threads/v1"
	tokencountingv1 "github.com/agynio/gateway/gen/agynio/api/token_counting/v1"
	tracingv1 "github.com/agynio/gateway/gen/agynio/api/tracing/v1"
	usersv1 "github.com/agynio/gateway/gen/agynio/api/users/v1"
	zitimgmtv1 "github.com/agynio/gateway/gen/agynio/api/ziti_management/v1"
	"github.com/agynio/gateway/internal/apitokenresolver"
	"github.com/agynio/gateway/internal/clusteradminresolver"
	"github.com/agynio/gateway/internal/gateway"
	"github.com/agynio/gateway/internal/grpcclient"
	"github.com/agynio/gateway/internal/oidcauth"
	"github.com/agynio/gateway/internal/oidcresolver"
	"github.com/agynio/gateway/internal/platform"
	"github.com/agynio/gateway/internal/ziticonn"
	"github.com/agynio/gateway/internal/zitimanager"
	"github.com/agynio/gateway/internal/zitimgmtclient"
)

const (
	defaultAddr             = ":8080"
	defaultZitiServiceName  = "gateway"
	defaultAppProxyCacheTTL = 30 * time.Second
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := platform.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("failed to load platform configuration: %v", err)
	}

	var zitiMgmtClient *zitimgmtclient.Client
	if config.ZitiEnabled {
		zitiMgmtClient, err = zitimgmtclient.NewClient(config.ZitiManagementGRPCTarget)
		if err != nil {
			log.Fatalf("failed to create ziti management gRPC client: %v", err)
		}
		defer func() {
			if err := zitiMgmtClient.Close(); err != nil {
				log.Printf("failed to close ziti management gRPC client: %v", err)
			}
		}()
	}

	var zitiResolver gateway.IdentityResolver
	if zitiMgmtClient != nil {
		zitiResolver = zitiMgmtClient
	}

	cleanup := make([]func(), 0, 10)
	defer func() {
		for _, closeFn := range cleanup {
			closeFn()
		}
	}()

	usersClient := mustClient(config.UsersGRPCTarget, "users", usersv1.NewUsersServiceClient, &cleanup)

	var oidcResolver gateway.BearerTokenResolver
	oidcConfigured := config.OIDCIssuerURL != "" || config.OIDCClientID != ""
	if oidcConfigured {
		if config.OIDCIssuerURL == "" || config.OIDCClientID == "" {
			log.Fatalf("both OIDC issuer URL and client ID are required when OIDC is enabled")
		}
		verifier, err := oidcauth.NewVerifier(ctx, config.OIDCIssuerURL, config.OIDCClientID)
		if err != nil {
			log.Fatalf("failed to create OIDC verifier: %v", err)
		}
		userinfoHTTPClient := &http.Client{Timeout: 10 * time.Second}
		resolver, err := oidcresolver.NewResolver(verifier, usersClient, userinfoHTTPClient,
			oidcresolver.WithProfileSource(oidcresolver.ProfileSource(config.OIDCProfileSource)),
			oidcresolver.WithClaimNames(oidcresolver.ClaimNames{
				Name:              config.OIDCClaimName,
				Email:             config.OIDCClaimEmail,
				Picture:           config.OIDCClaimPicture,
				PreferredUsername: config.OIDCClaimPreferredUser,
			}),
		)
		if err != nil {
			log.Fatalf("failed to create OIDC resolver: %v", err)
		}
		oidcResolver = resolver
	}

	apiTokenResolver := apitokenresolver.NewResolver(usersClient)

	var clusterAdminResolver *clusteradminresolver.Resolver
	if config.ClusterAdminToken != "" {
		resolver, err := clusteradminresolver.NewResolver(config.ClusterAdminToken, config.ClusterAdminIdentityID)
		if err != nil {
			log.Fatalf("failed to create cluster admin resolver: %v", err)
		}
		clusterAdminResolver = resolver
	}

	agentsClient := mustClient(config.AgentsGRPCTarget, "agents", agentsv1.NewAgentsServiceClient, &cleanup)
	appsClient := mustClient(config.AppsGRPCTarget, "apps", appsv1.NewAppsServiceClient, &cleanup)
	threadsClient := mustClient(config.ThreadsGRPCTarget, "threads", threadsv1.NewThreadsServiceClient, &cleanup)
	chatClient := mustClient(config.ChatGRPCTarget, "chat", chatv1.NewChatServiceClient, &cleanup)
	notificationsClient := mustClient(config.NotificationsGRPCTarget, "notifications", notificationsv1.NewNotificationsServiceClient, &cleanup)
	organizationsClient := mustClient(config.OrganizationsGRPCTarget, "organizations", organizationsv1.NewOrganizationsServiceClient, &cleanup)
	runnersClient := mustClient(config.RunnersGRPCTarget, "runners", runnersv1.NewRunnersServiceClient, &cleanup)
	terminalProxyClient := mustClient(config.TerminalProxyGRPCTarget, "terminal proxy", terminalproxyv1.NewTerminalProxyServiceClient, &cleanup)
	filesClient := mustClient(config.FilesGRPCTarget, "files", filesv1.NewFilesServiceClient, &cleanup)
	agentStateClient := mustClient(config.AgentStateGRPCTarget, "agent state", agentstatev1.NewAgentStateServiceClient, &cleanup)
	tokenCountingClient := mustClient(config.TokenCountingGRPCTarget, "token counting", tokencountingv1.NewTokenCountingServiceClient, &cleanup)
	meteringClient := mustClient(config.MeteringGRPCTarget, "metering", meteringv1.NewMeteringServiceClient, &cleanup)
	llmClient := mustClient(config.LLMGRPCTarget, "llm", llmv1.NewLLMServiceClient, &cleanup)
	secretsClient := mustClient(config.SecretsGRPCTarget, "secrets", secretsv1.NewSecretsServiceClient, &cleanup)
	tracingClient := mustClient(config.TracingGRPCTarget, "tracing", tracingv1.NewTracingServiceClient, &cleanup)
	imagesClient := mustClient(config.ImagesGRPCTarget, "images", imagesv1.NewImagesServiceClient, &cleanup)
	exposeClient := mustClient(config.ExposeGRPCTarget, "expose", exposev1.NewExposeServiceClient, &cleanup)
	egressRulesClient := mustClient(config.EgressRulesGRPCTarget, "egress rules", egressv1.NewEgressRulesServiceClient, &cleanup)
	groupsClient := mustClient(config.GroupsGRPCTarget, "groups", groupsv1.NewGroupsServiceClient, &cleanup)
	networksClient := mustClient(config.NetworksGRPCTarget, "networks", networksv1.NewNetworksServiceClient, &cleanup)

	gatewayHandler := gateway.New(
		agentsClient,
		appsClient,
		threadsClient,
		chatClient,
		notificationsClient,
		filesClient,
		agentStateClient,
		tokenCountingClient,
		llmClient,
		secretsClient,
		tracingClient,
		imagesClient,
	)
	threadsGateway := gateway.NewThreadsGateway(gatewayHandler)
	usersGateway := gateway.NewUsersGateway(usersClient)
	organizationsGateway := gateway.NewOrganizationsGateway(organizationsClient)
	runnersGateway := gateway.NewRunnersGateway(runnersClient)
	terminalGateway := gateway.NewTerminalGateway(terminalProxyClient)
	meteringGateway := gateway.NewMeteringGateway(meteringClient)
	exposeGateway := gateway.NewExposeGateway(exposeClient)
	egressRulesGateway := gateway.NewEgressRulesGateway(egressRulesClient)
	groupsGateway := gateway.NewGroupsGateway(groupsClient)
	networksGateway := gateway.NewNetworksGateway(networksClient)

	interceptors := []connect.Interceptor{
		gateway.NewRecoveryInterceptor(),
		gateway.NewLoggingInterceptor(),
	}
	if zitiResolver != nil || oidcResolver != nil || apiTokenResolver != nil || clusterAdminResolver != nil {
		interceptors = append([]connect.Interceptor{
			gateway.NewAuthInterceptor(zitiResolver, oidcResolver, apiTokenResolver, clusterAdminResolver),
		}, interceptors...)
	}
	handlerOptions := connect.WithInterceptors(interceptors...)

	mux := http.NewServeMux()

	var meHandler http.Handler = http.HandlerFunc(gateway.MeHandler)
	if zitiResolver != nil || oidcResolver != nil || apiTokenResolver != nil || clusterAdminResolver != nil {
		meHandler = gateway.NewAuthMiddleware(zitiResolver, oidcResolver, apiTokenResolver, clusterAdminResolver)(meHandler)
	}
	mux.Handle("/me", meHandler)

	registerConnect := func(handlerPath string, handler http.Handler) {
		mux.Handle(handlerPath, handler)
	}

	registerConnect(gatewayv1connect.NewAgentsGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewAppsGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewThreadsGatewayHandler(threadsGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewChatGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewNotificationsGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewFilesGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewAgentStateGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewTokenCountingGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewMeteringGatewayHandler(meteringGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewLLMGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewSecretsGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewImagesGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewTracingGatewayHandler(gatewayHandler, handlerOptions))
	registerConnect(gatewayv1connect.NewUsersGatewayHandler(usersGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewOrganizationsGatewayHandler(organizationsGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewRunnersGatewayHandler(runnersGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewTerminalGatewayHandler(terminalGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewExposeGatewayHandler(exposeGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewEgressRulesGatewayHandler(egressRulesGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewGroupsGatewayHandler(groupsGateway, handlerOptions))
	registerConnect(gatewayv1connect.NewNetworksGatewayHandler(networksGateway, handlerOptions))

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders: []string{"*"},
	})

	addr := defaultAddr
	if v := strings.TrimSpace(os.Getenv("ADDR")); v != "" {
		addr = v
	}

	connContext := func(ctx context.Context, conn net.Conn) context.Context {
		sourceIdentity, ok := ziticonn.SourceIdentityFromConn(conn)
		if !ok {
			return ctx
		}
		return ziticonn.WithSourceIdentity(ctx, sourceIdentity)
	}

	h2cHandler := h2c.NewHandler(corsMiddleware.Handler(mux), &http2.Server{})
	server := &http.Server{
		Addr:        addr,
		Handler:     h2cHandler,
		ConnContext: connContext,
	}

	var zitiMgr *zitimanager.Manager
	var readiness zitiReadiness
	if config.ZitiEnabled {
		serviceName := zitiServiceName()
		listenerFactory := func(zitiCtx ziti.Context, options *ziti.ListenOptions) (net.Listener, error) {
			return zitiCtx.ListenWithOptions(serviceName, options)
		}

		var zitiListenerMu sync.Mutex
		var zitiListener net.Listener
		onNewListener := func(listener net.Listener, established uint) {
			zitiListenerMu.Lock()
			oldListener := zitiListener
			zitiListener = listener
			zitiListenerMu.Unlock()
			// Logged only after a router confirmed a terminator; E2E waits for it.
			log.Printf("gateway listening on ziti service %s (established listeners: %d)", serviceName, established)
			go func() {
				// Replaced or lost listeners end here; the manager's watchdog
				// re-enrolls, so this must not exit the process.
				if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Printf("ziti listener for service %s stopped: %v", serviceName, err)
				}
			}()
			if oldListener != nil {
				if err := oldListener.Close(); err != nil {
					log.Printf("failed to close ziti listener: %v", err)
				}
			}
		}

		mgr, err := zitimanager.New(
			zitiMgmtClient,
			zitimgmtv1.ServiceType_SERVICE_TYPE_GATEWAY,
			config.ZitiEnrollmentTimeout,
			config.ZitiLeaseRenewalInterval,
			config.ZitiBindTimeout,
			listenerFactory,
			onNewListener,
		)
		if err != nil {
			log.Fatalf("failed to initialize ziti manager: %v", err)
		}
		defer func() {
			if zitiCtx := mgr.ZitiContext(); zitiCtx != nil {
				zitiCtx.Close()
			}
		}()

		appProxyHandler := http.Handler(gateway.NewAppProxyHandler(appsClient, agentsClient, mgr, defaultAppProxyCacheTTL))
		if zitiResolver != nil || oidcResolver != nil || apiTokenResolver != nil || clusterAdminResolver != nil {
			appProxyHandler = gateway.NewAuthMiddleware(zitiResolver, oidcResolver, apiTokenResolver, clusterAdminResolver)(appProxyHandler)
		}
		mux.Handle("/apps/", appProxyHandler)

		zitiMgr = mgr
		readiness = mgr
	}
	mux.Handle(readyzPath, newReadyzHandler(readiness))

	if zitiMgr != nil {
		// The TCP server starts without waiting for Ziti so liveness and /readyz
		// answer while binding. Startup fails closed: no terminator within the
		// enrollment timeout exits. After that Run retries re-enrollment
		// indefinitely, so a Ziti outage leaves the TCP API serving while
		// /readyz reports 503.
		go func() {
			if err := zitiMgr.Start(ctx); err != nil {
				log.Fatalf("failed to establish ziti service listener: %v", err)
			}
			zitiMgr.Run(ctx)
		}()
	}

	log.Printf("gateway listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}

func zitiServiceName() string {
	if v := strings.TrimSpace(os.Getenv("ZITI_SERVICE_NAME")); v != "" {
		return v
	}
	return defaultZitiServiceName
}

func mustClient[T any](target, name string, factory func(grpc.ClientConnInterface) T, cleanup *[]func()) T {
	client, err := grpcclient.New(target, factory)
	if err != nil {
		log.Fatalf("failed to create %s gRPC client: %v", name, err)
	}

	if cleanup != nil {
		*cleanup = append(*cleanup, func() {
			if err := client.Close(); err != nil {
				log.Printf("failed to close %s gRPC client: %v", name, err)
			}
		})
	}

	return client.Service()
}
