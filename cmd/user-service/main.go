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
	pb "github.com/Varun5711/shorternit/proto/user"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

func provideConfig() (*config.Config, error) {
	return config.Load()
}

func provideLogger() *logger.Logger {
	return logger.New("user-service")
}

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

func provideJWTManager(cfg *config.Config, log *logger.Logger) *auth.JWTManager {
	if cfg.JWT.Secret == "" {
		log.Fatal("JWT_SECRET must be set")
	}
	return auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.TokenDuration)
}

func provideUserStorage(db *database.DBManager) *storage.UserStorage {
	return storage.NewUserStorage(db)
}

func provideUserService(us *storage.UserStorage, jwt *auth.JWTManager) *service.UserService {
	return service.NewUserService(us, jwt)
}

func provideGRPCServer() *grpc.Server {
	return grpc.NewServer()
}

func provideListener() (net.Listener, error) {
	port := os.Getenv("USER_SERVICE_PORT")
	if port == "" {
		port = "50052"
	}
	return net.Listen("tcp", ":"+port)
}

func registerLifecycle(
	lc fx.Lifecycle,
	grpcServer *grpc.Server,
	userService *service.UserService,
	listener net.Listener,
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
			dbManager.Close()
			return nil
		},
	})
}

func main() {
	fx.New(
		fx.Provide(
			provideConfig,
			provideLogger,
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
