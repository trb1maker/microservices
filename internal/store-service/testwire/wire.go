package testwire

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/trb1maker/microservices/internal/platform/inbox"
	inboxmongo "github.com/trb1maker/microservices/internal/platform/inbox/mongostore"
	"github.com/trb1maker/microservices/internal/platform/natsx"
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
	confirmGate chan struct{}
	subCount    int
}

func SetupStore(
	ctx context.Context,
	db *mongo.Database,
	client *natsx.Client,
	subjects Subjects,
	opts Options,
) (*Worker, error) {
	if err := mongoadapter.EnsureStoreIndexes(ctx, db); err != nil {
		return nil, fmt.Errorf("ensure store indexes: %w", err)
	}
	if err := mongoadapter.SeedProducts(ctx, db); err != nil {
		return nil, fmt.Errorf("seed products: %w", err)
	}
	if err := seedE2EProduct(ctx, db); err != nil {
		return nil, fmt.Errorf("seed e2e product: %w", err)
	}

	productRepo := mongoadapter.NewProductRepository(db)
	stockRepo := mongoadapter.NewStockRepository(db)
	eventPub := natsadapter.NewEventPublisher(
		client,
		subjects.ItemsReserved,
		subjects.ReservationFailed,
		subjects.OrderConfirmed,
		subjects.ReservationReleased,
	)
	storeSvc := app.NewStoreServiceWithReservations(productRepo, stockRepo, eventPub, nil, mongoadapter.NewReservationStore(db))
	worker := natsadapter.NewWorker(storeSvc)
	worker.SetInbox(inbox.ForConsumer(inboxmongo.New(db), "store-service"))

	w := &Worker{worker: worker}
	if opts.GateConfirm {
		w.confirmGate = make(chan struct{})
	}

	if err := w.subscribe(ctx, client, subjects, opts.GateConfirm); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Worker) subscribe(ctx context.Context, client *natsx.Client, subjects Subjects, gateConfirm bool) error {
	reserveHandler := natsx.Handler(w.worker.HandleReserveItems)
	releaseHandler := natsx.Handler(w.worker.HandleReleaseReservation)
	confirmHandler := natsx.Handler(w.worker.HandleConfirmOrder)

	if gateConfirm {
		gate := w.confirmGate
		confirmHandler = func(ctx context.Context, msg *nats.Msg) error {
			<-gate
			return w.worker.HandleConfirmOrder(ctx, msg)
		}
	}

	handlers := []struct {
		subject string
		durable string
		handler natsx.Handler
	}{
		{subjects.ReserveItems, "test-store-reserve-items", reserveHandler},
		{subjects.ConfirmOrder, "test-store-confirm-order", confirmHandler},
		{subjects.ReleaseReservation, "test-store-release-reservation", releaseHandler},
	}

	for _, item := range handlers {
		stream := natsx.StreamForSubject(item.subject)
		if _, err := client.ConsumeDurable(context.WithoutCancel(ctx), stream, item.durable, item.subject, item.handler, natsx.DurableConsumerConfig{
			Inbox: w.worker.Inbox(),
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", item.subject, err)
		}
		w.subCount++
	}

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
	return w.subCount
}

func (w *Worker) Close() {
	if w == nil {
		return
	}
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
