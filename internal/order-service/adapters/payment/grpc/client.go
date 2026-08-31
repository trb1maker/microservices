package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"

	"github.com/trb1maker/microservices/internal/order-service/app"
	paymentpb "github.com/trb1maker/microservices/internal/platform/proto/payment"
	"github.com/trb1maker/microservices/internal/platform/tlsutil"

	"github.com/trb1maker/microservices/internal/platform/grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

var errPaymentNotServing = errors.New("payment service is not serving")

type ClientConfig struct {
	Addr       string
	CertFile   string
	KeyFile    string
	CAFile     string
	ServerName string
	Insecure   bool
}

type PaymentClient struct {
	conn   *grpc.ClientConn
	client paymentpb.PaymentServiceClient
}

func NewPaymentClient(ctx context.Context, cfg ClientConfig) (*PaymentClient, error) {
	_ = ctx
	if cfg.Insecure {
		return newPaymentClient(cfg.Addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpcx.ClientStatsHandler(),
		)
	}

	tlsConfig, err := loadClientTLS(cfg)
	if err != nil {
		return nil, err
	}
	return newPaymentClient(cfg.Addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpcx.ClientStatsHandler(),
	)
}

func newPaymentClient(addr string, opts ...grpc.DialOption) (*PaymentClient, error) {
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial payment service: %w", err)
	}
	return &PaymentClient{
		conn:   conn,
		client: paymentpb.NewPaymentServiceClient(conn),
	}, nil
}

func (c *PaymentClient) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("close payment connection: %w", err)
	}
	return nil
}

func (c *PaymentClient) Charge(ctx context.Context, orderID, userID string, amount int64) (string, bool, string, error) {
	resp, err := c.client.Charge(ctx, &paymentpb.ChargeRequest{
		OrderId: orderID,
		UserId:  userID,
		Amount:  amount,
	})
	if err != nil {
		return "", false, "", fmt.Errorf("grpc charge: %w", err)
	}
	return resp.GetTransactionId(), resp.GetStatus() == paymentpb.PaymentStatus_SUCCEEDED, resp.GetMessage(), nil
}

func (c *PaymentClient) CheckHealth(ctx context.Context) error {
	healthClient := grpchealth.NewHealthClient(c.conn)
	resp, err := healthClient.Check(ctx, &grpchealth.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("grpc health check: %w", err)
	}
	if resp.GetStatus() != grpchealth.HealthCheckResponse_SERVING {
		return errPaymentNotServing
	}
	return nil
}

func (c *PaymentClient) Refund(ctx context.Context, orderID, userID string, amount int64, originalTransactionID string) (string, bool, string, error) {
	resp, err := c.client.Refund(ctx, &paymentpb.RefundRequest{
		OrderId:               orderID,
		UserId:                userID,
		Amount:                amount,
		OriginalTransactionId: originalTransactionID,
	})
	if err != nil {
		return "", false, "", fmt.Errorf("grpc refund: %w", err)
	}
	return resp.GetTransactionId(), resp.GetStatus() == paymentpb.PaymentStatus_SUCCEEDED, resp.GetMessage(), nil
}

var _ app.PaymentClient = (*PaymentClient)(nil)

func loadClientTLS(cfg ClientConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	caPool, err := tlsutil.LoadClientCAPool(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("load client CA pool: %w", err)
	}

	serverName := cfg.ServerName
	if serverName == "" {
		serverName = "payment-service"
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func PaymentClientConfigFromEnv(addr, certFile, keyFile, caFile string) ClientConfig {
	if certFile == "" {
		certFile = os.Getenv("TLS_CERT_FILE")
	}
	if keyFile == "" {
		keyFile = os.Getenv("TLS_KEY_FILE")
	}
	if caFile == "" {
		caFile = os.Getenv("TLS_CLIENT_CA_FILE")
	}
	return ClientConfig{
		Addr:     addr,
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	}
}
