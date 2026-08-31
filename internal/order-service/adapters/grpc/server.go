package grpc

import (
	"context"
	"fmt"
	"net"
	"uuid"

	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/domain"
	orderpb "github.com/trb1maker/microservices/internal/platform/proto/order"
	"github.com/trb1maker/microservices/internal/platform/tlsutil"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type ServerConfig struct {
	Addr         string
	CertFile     string
	KeyFile      string
	ClientCAFile string
}

type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
}

func NewServer(
	cfg ServerConfig,
	orders *app.OrderService,
	hub *StatusHub,
) (*Server, error) {
	tlsConfig, err := tlsutil.LoadServerTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("load server tls: %w", err)
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	orderpb.RegisterOrderServiceServer(grpcServer, &OrderServer{
		orders: orders,
		hub:    hub,
	})

	return &Server{grpcServer: grpcServer, listener: listener}, nil
}

func (s *Server) Serve() error {
	if err := s.grpcServer.Serve(s.listener); err != nil {
		return fmt.Errorf("serve grpc: %w", err)
	}
	return nil
}

func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
}

type OrderServer struct {
	orderpb.UnimplementedOrderServiceServer
	orders *app.OrderService
	hub    *StatusHub
}

func (s *OrderServer) WatchOrderStatus(
	req *orderpb.WatchOrderStatusRequest,
	stream orderpb.OrderService_WatchOrderStatusServer,
) error {
	orderID, err := uuid.Parse(req.GetOrderId())
	if err != nil {
		//nolint:wrapcheck // gRPC status must be returned directly.
		return status.Error(codes.InvalidArgument, "invalid order_id")
	}

	ctx := stream.Context()
	order, err := s.orders.GetOrderForService(ctx, domain.OrderID(orderID))
	if err != nil {
		//nolint:wrapcheck // gRPC status must be returned directly.
		return status.Error(codes.Internal, "get order failed")
	}
	if order == nil {
		//nolint:wrapcheck // gRPC status must be returned directly.
		return status.Error(codes.NotFound, "order not found")
	}

	for _, entry := range order.StatusHistory() {
		if err := stream.Send(toStatusUpdate(order, entry)); err != nil {
			return fmt.Errorf("send status update: %w", err)
		}
	}

	if isTerminalStatus(order.Status()) {
		return nil
	}

	updates, cancel := s.hub.Subscribe(req.GetOrderId())
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("watch order status context: %w", ctx.Err())
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if err := stream.Send(update); err != nil {
				return fmt.Errorf("send status update: %w", err)
			}
			if isTerminalStatus(domain.OrderStatus(update.GetStatus())) {
				return nil
			}
		}
	}
}

func toStatusUpdate(order *domain.Order, entry domain.StatusHistoryEntry) *orderpb.StatusUpdate {
	return &orderpb.StatusUpdate{
		OrderId:   uuid.UUID(order.OrderID()).String(),
		Status:    string(entry.Status),
		Reason:    entry.Reason,
		Timestamp: entry.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func isTerminalStatus(status domain.OrderStatus) bool {
	return status == domain.OrderStatusConfirmed || status == domain.OrderStatusCancelled
}
