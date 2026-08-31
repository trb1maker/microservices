package natsx

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/nats-io/nats.go"
)

type ConnectConfig struct {
	URL      string
	Name     string
	CertFile string
	KeyFile  string
	CAFile   string
}

func Connect(ctx context.Context, cfg ConnectConfig) (*Client, error) {
	opts := []nats.Option{
		nats.Name(cfg.Name),
		nats.Secure(&tls.Config{MinVersion: tls.VersionTLS12}),
	}
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		opts = append(opts,
			nats.ClientCert(cfg.CertFile, cfg.KeyFile),
			nats.RootCAs(cfg.CAFile),
		)
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	client, err := New(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return client, nil
}
