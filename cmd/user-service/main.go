// Package main implements the user service for the Tiny URL shortener.
//
// The user service is a gRPC microservice that owns all user-related
// operations: registration, login (with bcrypt password hashing), and JWT
// token issuance/validation. The API gateway delegates every authentication
// and profile request to this service, keeping auth logic centralized and
// the gateway stateless.
//
// User records are stored in PostgreSQL via the storage layer. JWTs are
// signed with a shared secret so the API gateway's auth middleware can
// verify tokens by calling ValidateToken on this service.
//
// Dependency injection is managed by Uber FX.
package main

import (
	"context"
	"net"
	"os"

	"github.com/Varun5711/shorternit/internal/auth"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/service"
	"github.com/Varun5711/shorternit/internal/storage"
	"github.com/Varun5711/shorternit/internal/tracing"
	pb "github.com/Varun5711/shorternit/proto/user"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

// provideConfig loads the unified application configuration from environment
// variables and config files.
func provideConfig() (*config.Config, error) {
	return config.Load()
}

// provideLogger creates a structured logger tagged with "user-service" so
// log output is identifiable in centralized logging.
func provideLogger() *logger.Logger {
	return logger.New("user-service")
}

// provideDBManager sets up a PostgreSQL connection pool with primary/replica
// topology. User writes (registration) go to the primary; reads (login
// lookups, profile fetches) can be served by replicas.
func provideDBManager(cfg *config.Config) (*database.DBManager, error) {
	return database.NewDBManager(context.Background(), database.Config{
		PrimaryDSN:      cfg.Database.PrimaryDSN,
		ReplicaDSNs:     cfg.Database.ReplicaDSNs,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	})
}

// provideJWTManager creates the JWT token manager used to sign and verify
// authentication tokens. The secret is required at startup because the
// entire auth flow depends on it; a missing secret is a fatal configuration
// error rather than a runtime surprise.
func provideJWTManager(cfg *config.Config, log *logger.Logger) *auth.JWTManager {
	if cfg.JWT.Secret == "" {
		log.Fatal("JWT_SECRET must be set")
	}
	return auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.TokenDuration)
}

// provideUserStorage creates the PostgreSQL-backed user storage layer. All
// SQL queries for user CRUD are encapsulated here.
func provideUserStorage(db *database.DBManager) *storage.UserStorage {
	return storage.NewUserStorage(db)
}

// provideUserService assembles the core user business logic. It combines
// persistent storage with JWT management to implement the Register, Login,
// and ValidateToken RPCs defined in proto/user.
func provideUserService(us *storage.UserStorage, jwt *auth.JWTManager) *service.UserService {
	return service.NewUserService(us, jwt)
}

// provideTracerProvider initializes OpenTelemetry distributed tracing and
// exports spans to Jaeger. Trace context propagates from the API gateway
// through gRPC metadata so auth requests appear as child spans of the
// original HTTP request.
func provideTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	return tracing.InitTracer(tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		JaegerEndpoint: cfg.Tracing.JaegerEndpoint,
		ServiceName:    "user-service",
		ServiceVersion: "1.0.0",
		SampleRate:     cfg.Tracing.SampleRate,
	})
}

// provideGRPCServer creates a gRPC server with OpenTelemetry instrumentation.
// The otelgrpc stats handler automatically creates spans for every inbound
// RPC.
func provideGRPCServer() *grpc.Server {
	return grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
}

// provideListener binds a TCP listener on the port specified by
// USER_SERVICE_PORT (default 50052). The API gateway's gRPC client
// connects to this port for auth operations.
func provideListener() (net.Listener, error) {
	port := os.Getenv("USER_SERVICE_PORT")
	if port == "" {
		port = "50052"
	}
	return net.Listen("tcp", ":"+port)
}

// registerLifecycle wires the gRPC server into the FX lifecycle. On start,
// it registers the UserService implementation and serves RPCs in a
// background goroutine. On stop, it drains in-flight RPCs via GracefulStop,
// then shuts down tracing and the database pool.
func registerLifecycle(
	lc fx.Lifecycle,
	grpcServer *grpc.Server,
	userService *service.UserService,
	listener net.Listener,
	tp *sdktrace.TracerProvider,
	dbManager *database.DBManager,
	log *logger.Logger,
) {
	pb.RegisterUserServiceServer(grpcServer, userService)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Listening on %s", listener.Addr().String())
			go func() {
				if err := grpcServer.Serve(listener); err != nil {
					log.Error("Server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Shutting down user-service...")
			grpcServer.GracefulStop()
			_ = tracing.ShutdownTracer(ctx, tp)
			dbManager.Close()
			return nil
		},
	})
}

// main assembles the complete FX dependency graph for the user service.
//
// The graph flows from infrastructure (config, logging, tracing, Postgres)
// through auth components (JWT manager, user storage) up to the
// UserService that implements the gRPC proto/user interface.
// fx.Invoke(registerLifecycle) triggers graph construction and starts the
// gRPC server. Run() blocks until a termination signal is received.
func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
			provideTracerProvider,
			provideDBManager,
			provideJWTManager,
			provideUserStorage,
			provideUserService,
			provideGRPCServer,
			provideListener,
		),
		fx.Invoke(registerLifecycle),
	).Run()
}
