package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

var ErrParseClientCA = errors.New("parse client ca")

func LoadServerTLSConfig(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.RequestClientCert,
	}

	if clientCAFile != "" {
		pool, err := LoadClientCAPool(clientCAFile)
		if err != nil {
			return nil, err
		}

		cfg.ClientCAs = pool
	}

	return cfg, nil
}

func LoadClientCAPool(clientCAFile string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(clientCAFile) //nolint:gosec // path comes from trusted service configuration
	if err != nil {
		return nil, fmt.Errorf("read client ca: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, ErrParseClientCA
	}

	return pool, nil
}
