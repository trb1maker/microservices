package orderapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Config struct {
	BaseURL     string
	CAFile      string
	SkipVerify  bool
	Timeout     time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.SkipVerify, //nolint:gosec // dev-only toggle via TLS_SKIP_VERIFY
		},
	}
	if cfg.CAFile != "" {
		pool, err := loadCAPool(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path) //nolint:gosec // path from trusted configuration
	if err != nil {
		return nil, fmt.Errorf("read ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("parse ca file")
	}
	return pool, nil
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type CartItem struct {
	ProductID  string `json:"product_id"`
	Quantity   int64  `json:"quantity"`
	UnitPrice  int64  `json:"unit_price"`
	TotalPrice int64  `json:"total_price"`
}

type Cart struct {
	UserID     string     `json:"user_id"`
	Items      []CartItem `json:"items"`
	TotalPrice int64      `json:"total_price"`
	UpdatedAt  string     `json:"updated_at"`
}

type Order struct {
	OrderID         string     `json:"order_id"`
	UserID          string     `json:"user_id"`
	Status          string     `json:"status"`
	TotalPrice      int64      `json:"total_price"`
	DeliveryAddress string     `json:"delivery_address"`
	Items           []CartItem `json:"items"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (c *Client) Login(ctx context.Context, email, password string) (LoginResponse, error) {
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	var resp LoginResponse
	if err := c.doJSON(ctx, http.MethodPost, "/auth/login", "", body, &resp); err != nil {
		return LoginResponse{}, err
	}
	return resp, nil
}

func (c *Client) GetCart(ctx context.Context, token string) (Cart, error) {
	var cart Cart
	if err := c.doJSON(ctx, http.MethodGet, "/cart", token, "", &cart); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

func (c *Client) AddCartItem(ctx context.Context, token, productID string, quantity, unitPrice int64) (Cart, error) {
	body := fmt.Sprintf(`{"product_id":%q,"quantity":%d,"unit_price":%d}`, productID, quantity, unitPrice)
	var cart Cart
	if err := c.doJSON(ctx, http.MethodPost, "/cart/items", token, body, &cart); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

func (c *Client) RemoveCartItem(ctx context.Context, token, productID string) (Cart, error) {
	var cart Cart
	path := fmt.Sprintf("/cart/items/%s", productID)
	if err := c.doJSON(ctx, http.MethodDelete, path, token, "", &cart); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

func (c *Client) Checkout(ctx context.Context, token, deliveryAddress string) (Order, error) {
	body := fmt.Sprintf(`{"delivery_address":%q}`, deliveryAddress)
	var order Order
	if err := c.doJSON(ctx, http.MethodPost, "/orders", token, body, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (c *Client) GetOrder(ctx context.Context, token, orderID string) (Order, error) {
	var order Order
	path := fmt.Sprintf("/orders/%s", orderID)
	if err := c.doJSON(ctx, http.MethodGet, path, token, "", &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (c *Client) PayOrder(ctx context.Context, token, orderID string) (Order, error) {
	var order Order
	path := fmt.Sprintf("/orders/%s/pay", orderID)
	if err := c.doJSON(ctx, http.MethodPost, path, token, "", &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (c *Client) CancelOrder(ctx context.Context, token, orderID string) (Order, error) {
	var order Order
	path := fmt.Sprintf("/orders/%s", orderID)
	if err := c.doJSON(ctx, http.MethodDelete, path, token, "", &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (c *Client) doJSON(
	ctx context.Context,
	method, path, token, body string,
	out any,
) error {
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		var apiErr ErrorResponse
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("order service returned %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
