package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/trb1maker/microservices/tests/ui/internal/catalog"
	"github.com/trb1maker/microservices/tests/ui/internal/config"
	"github.com/trb1maker/microservices/tests/ui/internal/orderapi"
	"github.com/trb1maker/microservices/tests/ui/internal/orderwatch"
	"github.com/trb1maker/microservices/tests/ui/internal/session"
	"github.com/trb1maker/microservices/tests/ui/internal/sse"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	cfg       *config.Config
	sessions  *session.Store
	orders    *orderapi.Client
	watcher   *orderwatch.Client
	templates *template.Template
	sseBridge *sse.Bridge
}

func NewServer(cfg *config.Config) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	orders, err := orderapi.NewClient(orderapi.Config{
		BaseURL:    cfg.OrderHTTPBaseURL,
		CAFile:     cfg.TLSCAFile,
		SkipVerify: cfg.TLSSkipVerify,
	})
	if err != nil {
		return nil, err
	}
	watcher, err := orderwatch.NewClient(orderwatch.Config{
		Addr:       cfg.OrderGRPCAddr,
		CAFile:     cfg.TLSCAFile,
		SkipVerify: cfg.TLSSkipVerify,
	})
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:       cfg,
		sessions:  session.NewStore(cfg.SessionSecret, cfg.SessionMaxAge),
		orders:    orders,
		watcher:   watcher,
		templates: tmpl,
	}
	s.sseBridge = sse.NewBridge(watcher.Watch, s.renderStatusFragment)
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("POST /session/user", s.handleSelectUser)
	mux.HandleFunc("GET /fragments/cart", s.handleCartFragment)
	mux.HandleFunc("POST /cart/items", s.handleAddCartItem)
	mux.HandleFunc("DELETE /cart/items/{productID}", s.handleRemoveCartItem)
	mux.HandleFunc("POST /orders", s.handleCheckout)
	mux.HandleFunc("POST /orders/{id}/pay", s.handlePayOrder)
	mux.HandleFunc("DELETE /orders/{id}", s.handleCancelOrder)
	mux.HandleFunc("GET /orders/{id}/events", s.handleOrderEvents)
	return mux
}

func (s *Server) Close() error {
	return s.watcher.Close()
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (session.Data, bool) {
	data, err := s.sessions.Load(r)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": "Select a demo user first."})
		return session.Data{}, false
	}
	return data, true
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	data, _ := s.sessions.Load(r)
	var cart orderapi.Cart
	if data.AccessToken != "" {
		c, err := s.orders.GetCart(r.Context(), data.AccessToken)
		if err == nil {
			cart = c
		}
	}
	s.render(w, "home.html", map[string]any{
		"Users":         catalog.DemoUsers,
		"Products":      catalog.Products,
		"Session":       data,
		"Cart":          cart,
		"ActiveOrderID": r.URL.Query().Get("order_id"),
	})
}

func (s *Server) handleSelectUser(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	user, ok := catalog.UserByEmail(email)
	if !ok {
		http.Error(w, "unknown user", http.StatusBadRequest)
		return
	}
	login, err := s.orders.Login(r.Context(), user.Email, user.Password)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	if err := s.sessions.Save(w, session.Data{
		AccessToken: login.AccessToken,
		UserEmail:   user.Email,
		UserID:      user.ID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleCartFragment(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	cart, err := s.orders.GetCart(r.Context(), data.AccessToken)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	s.render(w, "cart.html", map[string]any{"Cart": cart})
}

func (s *Server) handleAddCartItem(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	productID := r.FormValue("product_id")
	product, found := catalog.ProductByID(productID)
	if !found {
		http.Error(w, "unknown product", http.StatusBadRequest)
		return
	}
	qty, err := strconv.ParseInt(r.FormValue("quantity"), 10, 64)
	if err != nil || qty <= 0 {
		qty = 1
	}
	cart, err := s.orders.AddCartItem(r.Context(), data.AccessToken, product.ID, qty, product.Price)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	s.render(w, "cart.html", map[string]any{"Cart": cart})
}

func (s *Server) handleRemoveCartItem(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	cart, err := s.orders.RemoveCartItem(r.Context(), data.AccessToken, r.PathValue("productID"))
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	s.render(w, "cart.html", map[string]any{"Cart": cart})
}

func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	address := r.FormValue("delivery_address")
	if address == "" {
		address = "Demo Street 1"
	}
	order, err := s.orders.Checkout(r.Context(), data.AccessToken, address)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	s.render(w, "order_panel.html", map[string]any{"Order": order})
}

func (s *Server) handlePayOrder(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	orderID := r.PathValue("id")
	if _, err := s.orders.GetOrder(r.Context(), data.AccessToken, orderID); err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	order, err := s.orders.PayOrder(r.Context(), data.AccessToken, orderID)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	s.render(w, "order_panel.html", map[string]any{"Order": order})
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	orderID := r.PathValue("id")
	if _, err := s.orders.GetOrder(r.Context(), data.AccessToken, orderID); err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	order, err := s.orders.CancelOrder(r.Context(), data.AccessToken, orderID)
	if err != nil {
		s.render(w, "error.html", map[string]any{"Message": err.Error()})
		return
	}
	s.render(w, "order_panel.html", map[string]any{"Order": order})
}

func (s *Server) handleOrderEvents(w http.ResponseWriter, r *http.Request) {
	data, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	orderID := r.PathValue("id")
	if _, err := s.orders.GetOrder(r.Context(), data.AccessToken, orderID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	s.sseBridge.ServeHTTP(w, r, orderID)
}

func (s *Server) renderStatusFragment(update orderwatch.StatusUpdate) (string, error) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, "status.html", update); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
