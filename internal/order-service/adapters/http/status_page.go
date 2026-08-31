package http

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	statusOK           = "ok"
	statusDown         = "down"
	statusDegraded     = "degraded"
	statusUnknown      = "unknown"
	statusReady        = "ready"
	checkOK            = "ok"
	checkPostgres      = "postgres"
	remoteCheckCount   = 2
	healthCheckTimeout = 2 * time.Second
)

//go:embed templates/health.html.tmpl
var healthTemplateFS embed.FS

type StatusItem struct {
	Name        string
	Status      string
	StatusClass string
	Detail      string
	Endpoint    string
}

type StatusPageData struct {
	GeneratedAt string
	Items       []StatusItem
}

type RemoteHealthChecker interface {
	CheckPayment(ctx context.Context) StatusItem
	CheckStore(ctx context.Context) StatusItem
}

type StatusDashboard struct {
	readiness ReadinessChecker
	remote    RemoteHealthChecker
	timeout   time.Duration
	template  *template.Template
}

func NewStatusDashboard(readiness ReadinessChecker, remote RemoteHealthChecker) (*StatusDashboard, error) {
	tmpl, err := template.ParseFS(healthTemplateFS, "templates/health.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse health template: %w", err)
	}
	return &StatusDashboard{
		readiness: readiness,
		remote:    remote,
		timeout:   healthCheckTimeout,
		template:  tmpl,
	}, nil
}

func (d *StatusDashboard) Build(ctx context.Context) StatusPageData {
	if d.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.timeout)
		defer cancel()
	}

	items := []StatusItem{
		{Name: "Order Service", Status: statusOK, StatusClass: statusOK, Detail: "process running", Endpoint: "order-service"},
	}

	if d.readiness != nil {
		ready, checks := d.readiness.Check(ctx)
		for name, detail := range checks {
			status := statusOK
			class := statusOK
			if detail != checkOK {
				status = statusDown
				class = statusDown
			}
			label := name
			endpoint := name
			switch name {
			case checkPostgres:
				label = "PostgreSQL (orders)"
				endpoint = "postgres:5432"
			case "redis":
				label = "Redis (cart)"
				endpoint = "redis:6379"
			case "nats":
				label = "NATS"
				endpoint = "nats:4222"
			}
			items = append(items, StatusItem{
				Name:        label,
				Status:      status,
				StatusClass: class,
				Detail:      detail,
				Endpoint:    endpoint,
			})
		}
		if !ready {
			items[0].Status = statusDegraded
			items[0].StatusClass = statusDegraded
			items[0].Detail = "local dependencies not ready"
		}
	}

	if d.remote != nil {
		items = append(items, d.checkRemote(ctx)...)
	}

	return StatusPageData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Items:       items,
	}
}

func (d *StatusDashboard) checkRemote(ctx context.Context) []StatusItem {
	var wg sync.WaitGroup
	results := make([]StatusItem, remoteCheckCount)
	wg.Add(remoteCheckCount)
	go func() {
		defer wg.Done()
		results[0] = d.remote.CheckPayment(ctx)
	}()
	go func() {
		defer wg.Done()
		results[1] = d.remote.CheckStore(ctx)
	}()
	wg.Wait()
	return results
}

func (d *StatusDashboard) RenderHTML(w http.ResponseWriter, data StatusPageData) {
	var buf bytes.Buffer
	if err := d.template.Execute(&buf, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

type HTTPRemoteChecker struct {
	storeHealthURL string
	paymentChecker PaymentHealthChecker
	client         *http.Client
}

type PaymentHealthChecker interface {
	CheckHealth(ctx context.Context) error
}

func NewHTTPRemoteChecker(storeHealthURL string, paymentChecker PaymentHealthChecker) *HTTPRemoteChecker {
	return &HTTPRemoteChecker{
		storeHealthURL: storeHealthURL,
		paymentChecker: paymentChecker,
		client:         &http.Client{Timeout: healthCheckTimeout},
	}
}

func (c *HTTPRemoteChecker) CheckPayment(ctx context.Context) StatusItem {
	item := StatusItem{
		Name:     "Payment Service",
		Endpoint: "payment-service:50051",
	}
	if c.paymentChecker == nil {
		item.Status = statusUnknown
		item.StatusClass = statusDegraded
		item.Detail = "payment health checker not configured"
		return item
	}
	if err := c.paymentChecker.CheckHealth(ctx); err != nil {
		item.Status = statusDown
		item.StatusClass = statusDown
		item.Detail = err.Error()
		return item
	}
	item.Status = statusOK
	item.StatusClass = statusOK
	item.Detail = "grpc health SERVING"
	return item
}

func (c *HTTPRemoteChecker) CheckStore(ctx context.Context) StatusItem {
	item := StatusItem{
		Name:     "Store Service",
		Endpoint: c.storeHealthURL,
	}
	if c.storeHealthURL == "" {
		item.Status = statusUnknown
		item.StatusClass = statusDegraded
		item.Detail = "store health url not configured"
		return item
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.storeHealthURL, nil) //nolint:gosec // operator-configured health endpoint
	if err != nil {
		item.Status = statusDown
		item.StatusClass = statusDown
		item.Detail = err.Error()
		return item
	}
	resp, err := c.client.Do(req) //nolint:gosec // operator-configured health endpoint
	if err != nil {
		item.Status = statusDown
		item.StatusClass = statusDown
		item.Detail = err.Error()
		return item
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		item.Status = statusDown
		item.StatusClass = statusDown
		item.Detail = strings.TrimSpace(string(body))
		return item
	}
	var ready struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	_ = json.Unmarshal(body, &ready)
	item.Status = statusOK
	item.StatusClass = statusOK
	item.Detail = formatChecks(ready.Checks)
	return item
}

func formatChecks(checks map[string]string) string {
	if len(checks) == 0 {
		return statusReady
	}
	parts := make([]string, 0, len(checks))
	for name, value := range checks {
		parts = append(parts, fmt.Sprintf("%s=%s", name, value))
	}
	return strings.Join(parts, ", ")
}

var _ RemoteHealthChecker = (*HTTPRemoteChecker)(nil)
