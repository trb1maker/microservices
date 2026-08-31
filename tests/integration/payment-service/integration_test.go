//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	natscontainer "github.com/testcontainers/testcontainers-go/modules/nats"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/trb1maker/microservices/internal/payment-service/adapters/eventpublisher"
	pgadapter "github.com/trb1maker/microservices/internal/payment-service/adapters/postgres"
	"github.com/trb1maker/microservices/internal/payment-service/app"
	"github.com/trb1maker/microservices/internal/payment-service/domain"
	"github.com/trb1maker/microservices/internal/payment-service/migrations"
	"github.com/trb1maker/microservices/tests/internal/natstest"
)

func TestPaymentServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	pgContainer, err := pgcontainer.Run(ctx,
		"postgres:18.4-alpine",
		pgcontainer.WithDatabase("payments"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("terminate postgres: %v", err)
		}
	}()

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, pgConnStr)
	require.NoError(t, err)
	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	err = migrations.Up(db)
	require.NoError(t, err)
	if err := db.Close(); err != nil {
		t.Logf("close migration db: %v", err)
	}

	natsContainer, err := natscontainer.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	defer func() {
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("terminate nats: %v", err)
		}
	}()

	natsURI, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	natsClient := natstest.NewClient(t, natsURI)
	defer natsClient.Conn().Close()
	nc := natsClient.Conn()

	_, err = pool.Exec(ctx, `INSERT INTO accounts (user_id, balance, version) VALUES ($1, $2, $3)`,
		"test-user-1", 100000, 1)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO accounts (user_id, balance, version) VALUES ($1, $2, $3)`,
		"test-user-2", 0, 1)
	require.NoError(t, err)

	accountRepo := pgadapter.NewAccountRepository(pool)
	txRepo := pgadapter.NewTransactionRepository(pool)
	eventPub := eventpublisher.NewNATSEventPublisher(natsClient, "payment.succeeded", "payment.failed", "payment.refund_succeeded", "payment.refund_failed")
	svc := app.NewPaymentService(accountRepo, txRepo, eventPub)

	t.Run("Charge_Success", func(t *testing.T) {
		result, err := svc.Charge(ctx, "order-int-1", "test-user-1", 5000)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusSucceeded, result.Status)
		assert.NotEmpty(t, result.TransactionID)

		acc, err := accountRepo.Get(ctx, "test-user-1")
		require.NoError(t, err)
		assert.Equal(t, int64(95000), acc.Balance)
	})

	t.Run("Charge_InsufficientFunds", func(t *testing.T) {
		result, err := svc.Charge(ctx, "order-int-2", "test-user-2", 5000)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusFailed, result.Status)
	})

	t.Run("Charge_AccountNotFound", func(t *testing.T) {
		result, err := svc.Charge(ctx, "order-int-3", "nonexistent", 5000)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusFailed, result.Status)
	})

	t.Run("Refund_Success", func(t *testing.T) {
		chargeResult, err := svc.Charge(ctx, "order-int-4", "test-user-1", 10000)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusSucceeded, chargeResult.Status)

		refundResult, err := svc.Refund(ctx, "order-int-4", "test-user-1", 10000, chargeResult.TransactionID)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusSucceeded, refundResult.Status)

		acc, err := accountRepo.Get(ctx, "test-user-1")
		require.NoError(t, err)
		assert.Equal(t, int64(95000), acc.Balance)
	})

	t.Run("Refund_Idempotency", func(t *testing.T) {
		chargeResult, err := svc.Charge(ctx, "order-int-5", "test-user-1", 5000)
		require.NoError(t, err)

		refundResult, err := svc.Refund(ctx, "order-int-5", "test-user-1", 5000, chargeResult.TransactionID)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusSucceeded, refundResult.Status)

		refundResult2, err := svc.Refund(ctx, "order-int-5", "test-user-1", 5000, chargeResult.TransactionID)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusFailed, refundResult2.Status)
		assert.Equal(t, "transaction has already been refunded", refundResult2.Message)
	})

	t.Run("NATSEventPublished", func(t *testing.T) {
		sub, err := nc.SubscribeSync("payment.succeeded")
		require.NoError(t, err)
		defer func() {
			if err := sub.Unsubscribe(); err != nil {
				t.Logf("unsubscribe: %v", err)
			}
		}()

		result, err := svc.Charge(ctx, "order-int-6", "test-user-1", 1000)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionStatusSucceeded, result.Status)

		msg, err := sub.NextMsg(5 * time.Second)
		require.NoError(t, err)
		assert.Contains(t, string(msg.Data), "order-int-6")
		assert.Contains(t, string(msg.Data), "test-user-1")
		assert.Contains(t, string(msg.Data), "1000")
	})

	t.Run("ConcurrentCharge_OptimisticLock", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO accounts (user_id, balance, version) VALUES ($1, $2, $3)`,
			"concurrent-user", 50000, 1)
		require.NoError(t, err)

		const numGoroutines = 10
		results := make(chan *app.ChargeResult, numGoroutines)
		for i := range numGoroutines {
			go func(orderN int) {
				orderID := fmt.Sprintf("order-concurrent-%d", orderN)
				result, err := svc.Charge(ctx, orderID, "concurrent-user", 10000)
				if err != nil {
					results <- &app.ChargeResult{Status: domain.TransactionStatusFailed}
					return
				}
				results <- result
			}(i)
		}

		successCount := 0
		failCount := 0
		for range numGoroutines {
			result := <-results
			if result.Status == domain.TransactionStatusSucceeded {
				successCount++
			} else {
				failCount++
			}
		}

		t.Logf("successful: %d, failed: %d", successCount, failCount)
		assert.Equal(t, 5, successCount, "exactly 5 charges should succeed")
		assert.Equal(t, 5, failCount, "exactly 5 charges should fail")

		acc, err := accountRepo.Get(ctx, "concurrent-user")
		require.NoError(t, err)
		assert.Equal(t, int64(0), acc.Balance)
	})
}

func TestDemoUserAccountsMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	pgContainer, err := pgcontainer.Run(ctx,
		"postgres:18.4-alpine",
		pgcontainer.WithDatabase("payments"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("terminate postgres: %v", err)
		}
	}()

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, pgConnStr)
	require.NoError(t, err)
	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	require.NoError(t, migrations.Up(db))
	require.NoError(t, migrations.Up(db))
	if err := db.Close(); err != nil {
		t.Logf("close migration db: %v", err)
	}

	demoUsers := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}

	for _, userID := range demoUsers {
		var balance int64
		var version int
		err := pool.QueryRow(ctx,
			`SELECT balance, version FROM accounts WHERE user_id = $1`,
			userID,
		).Scan(&balance, &version)
		require.NoError(t, err, "demo account %s", userID)
		assert.Equal(t, int64(100000), balance)
		assert.Equal(t, 1, version)
	}

	accountRepo := pgadapter.NewAccountRepository(pool)

	natsContainer, err := natscontainer.Run(ctx, natstest.Image, natstest.ContainerOptions()...)
	require.NoError(t, err)
	defer func() {
		if err := natsContainer.Terminate(ctx); err != nil {
			t.Logf("terminate nats: %v", err)
		}
	}()

	natsURI, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	natsClient := natstest.NewClient(t, natsURI)
	defer natsClient.Conn().Close()

	txRepo := pgadapter.NewTransactionRepository(pool)
	eventPub := eventpublisher.NewNATSEventPublisher(natsClient, "payment.succeeded", "payment.failed", "payment.refund_succeeded", "payment.refund_failed")
	svc := app.NewPaymentService(accountRepo, txRepo, eventPub)

	result, err := svc.Charge(ctx, "ui-demo-order-1", demoUsers[0], 2500)
	require.NoError(t, err)
	assert.Equal(t, domain.TransactionStatusSucceeded, result.Status)
}

var _ = testcontainers.Container(nil)
