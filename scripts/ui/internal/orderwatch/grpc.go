package orderwatch

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"

	orderpb "github.com/trb1maker/microservices/pkg/proto/order"
	"github.com/trb1maker/microservices/pkg/tlsutil"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type StatusUpdate struct {
	OrderID   string
	Status    string
	Reason    string
	Timestamp string
}

type Client struct {
	conn *grpc.ClientConn
}

type Config struct {
	Addr       string
	CAFile     string
	SkipVerify bool
}

func NewClient(cfg Config) (*Client, error) {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.SkipVerify, //nolint:gosec // dev-only toggle via TLS_SKIP_VERIFY
	}
	if cfg.CAFile != "" {
		pool, err := tlsutil.LoadClientCAPool(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}
	conn, err := grpc.NewClient(cfg.Addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	if err != nil {
		return nil, fmt.Errorf("dial order grpc: %w", err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Watch(ctx context.Context, orderID string) (<-chan StatusUpdate, error) {
	client := orderpb.NewOrderServiceClient(c.conn)
	stream, err := client.WatchOrderStatus(ctx, &orderpb.WatchOrderStatusRequest{OrderId: orderID})
	if err != nil {
		return nil, fmt.Errorf("watch order status: %w", err)
	}

	out := make(chan StatusUpdate, 8)
	go func() {
		defer close(out)
		for {
			update, recvErr := stream.Recv()
			if recvErr != nil {
				if recvErr != io.EOF && ctx.Err() == nil {
					out <- StatusUpdate{OrderID: orderID, Status: "ERROR", Reason: recvErr.Error()}
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- StatusUpdate{
				OrderID:   update.GetOrderId(),
				Status:    update.GetStatus(),
				Reason:    update.GetReason(),
				Timestamp: update.GetTimestamp(),
			}:
			}
			if isTerminal(update.GetStatus()) {
				return
			}
		}
	}()
	return out, nil
}

func isTerminal(status string) bool {
	return status == "CONFIRMED" || status == "CANCELLED"
}
