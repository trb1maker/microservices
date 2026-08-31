package grpcx

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func MetricsUnaryInterceptor(counter *prometheus.CounterVec) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		statusLabel := "ok"
		if err != nil {
			statusLabel = "error"
		}
		if counter != nil {
			counter.WithLabelValues(info.FullMethod, statusLabel).Inc()
		}
		return resp, err
	}
}

func ServerStatsHandler() grpc.ServerOption {
	return grpc.StatsHandler(otelgrpc.NewServerHandler())
}

func ClientStatsHandler() grpc.DialOption {
	return grpc.WithStatsHandler(otelgrpc.NewClientHandler())
}
