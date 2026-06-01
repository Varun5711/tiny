// Package grpc provides factory functions for creating instrumented gRPC
// client connections to the Tiny microservices. Every connection is wired
// with OpenTelemetry tracing out of the box so that distributed traces
// propagate automatically across service boundaries without callers
// needing to configure anything.
package grpc

import (
	pb "github.com/Varun5711/shorternit/proto/url"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NewURLServiceClient creates a gRPC client for the URL shortening service.
//
// The connection uses insecure (plaintext) transport because the services
// communicate over an internal Docker network where TLS termination happens
// at the API-gateway level. The otelgrpc StatsHandler is attached so that
// every outgoing RPC automatically creates a child span linked to the
// caller's trace context, enabling end-to-end distributed tracing in Jaeger.
//
// Note: the returned client holds an open connection. Callers that need to
// shut down gracefully should keep a reference to the underlying *grpc.ClientConn
// (currently encapsulated) and close it. A future refactor may return the
// conn alongside the client for this purpose.
func NewURLServiceClient(address string) (pb.URLServiceClient, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}

	return pb.NewURLServiceClient(conn), nil
}
