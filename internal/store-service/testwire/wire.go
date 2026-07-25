package testwire

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	mongoadapter "github.com/trb1maker/microservices/internal/store-service/adapters/mongodb"
	natsadapter "github.com/trb1maker/microservices/internal/store-service/adapters/nats"
	"github.com/trb1maker/microservices/internal/store-service/app"
)

const (
	E2EProductID         = "22222222-2222-4222-8222-222222222222"
	E2EProductPrice      = int64(2500)
	e2eStockAvailability = 100
)

type Subjects struct {
	ReserveItems        string
	ConfirmOrder        string
	ReleaseReservation  string
	ItemsReserved       string
	ReservationFailed   string
	OrderConfirmed      string
	ReservationReleased string
}

type Options struct {
	GateConfirm bool
}

type Worker struct {
	worker      *natsadapter.Worker
	subs        []*nats.Subscription
	confirmGate chan struct{}
}

func SetupStore(
	ctx context.Context,
	db *mongo.Database,
	nc *nats.Conn,
	subjects Subjects,
	opts Options,
) (*Worker, error) {
	if err := mongoadapter.SeedProducts(ctx, db); err != nil {
		return nil, fmt.Errorf("seed products: %w", err)
	}
	if err := seedE2EProduct(ctx, db); err != nil {
		return nil, fmt.Errorf("seed e2e product: %w", err)
	}

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(
		nc,
		subjects.ItemsReserved,
		subjects.ReservationFailed,
		subjects.OrderConfirmed,
		subjects.ReservationReleased,
	)
	storeSvc := app.NewStoreService(productRepo, stockRepo, eventPub)
	worker := natsadapter.NewWorker(storeSvc)

	w := &Worker{worker: worker}
	if err := w.subscribe(ctx, nc, subjects, opts.GateConfirm); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Worker) subscribe(ctx context.Context, nc *nats.Conn, subjects Subjects, gateConfirm bool) error {
	_ = ctx
	reserveSub, err := nc.Subscribe(subjects.ReserveItems, w.worker.HandleReserveItems)
	if err != nil {
		return fmt.Errorf("subscribe reserve items: %w", err)
	}

	confirmHandler := nats.MsgHandler(w.worker.HandleConfirmOrder)
	if gateConfirm {
		w.confirmGate = make(chan struct{})
		gate := w.confirmGate
		confirmHandler = func(msg *nats.Msg) { //nolint:contextcheck // store NATS worker manages its own context
			<-gate
			w.worker.HandleConfirmOrder(msg)
		}
	}

	confirmSub, err := nc.Subscribe(subjects.ConfirmOrder, confirmHandler)
	if err != nil {
		_ = reserveSub.Unsubscribe()
		return fmt.Errorf("subscribe confirm order: %w", err)
	}

	releaseSub, err := nc.Subscribe(subjects.ReleaseReservation, w.worker.HandleReleaseReservation)
	if err != nil {
		_ = reserveSub.Unsubscribe()
		_ = confirmSub.Unsubscribe()
		return fmt.Errorf("subscribe release reservation: %w", err)
	}

	w.subs = []*nats.Subscription{reserveSub, confirmSub, releaseSub}
	return nil
}

func (w *Worker) AllowConfirm() {
	if w == nil || w.confirmGate == nil {
		return
	}
	close(w.confirmGate)
	w.confirmGate = nil
}

func (w *Worker) ActiveSubscriptions() int {
	if w == nil {
		return 0
	}
	return len(w.subs)
}

func (w *Worker) Close() {
	if w == nil {
		return
	}
	for _, sub := range w.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
	w.subs = nil
	if w.worker != nil {
		w.worker.Close()
	}
}

func seedE2EProduct(ctx context.Context, db *mongo.Database) error {
	productColl := db.Collection("products")
	_, err := productColl.UpdateOne(
		ctx,
		bson.M{"_id": E2EProductID},
		bson.M{"$setOnInsert": bson.M{
			"_id":   E2EProductID,
			"name":  "E2E Widget",
			"sku":   "E2E-001",
			"price": E2EProductPrice,
		}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("seed e2e product: %w", err)
	}

	stockColl := db.Collection("stock")
	_, err = stockColl.UpdateOne(
		ctx,
		bson.M{"product_id": E2EProductID},
		bson.M{"$setOnInsert": bson.M{
			"product_id": E2EProductID,
			"available":  e2eStockAvailability,
			"reserved":   0,
		}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("seed e2e stock: %w", err)
	}
	return nil
}
