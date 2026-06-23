package main

import (
	"log"
	"order-service/internal/adapters/input/http"
	"order-service/internal/adapters/output/memory"
	"order-service/internal/app"
)

func main() {
	cartRepo := memory.NewCartRepository()
	orderRepo := memory.NewOrderRepository()

	cartService := app.NewCartService(cartRepo)
	orderService := app.NewOrderService(cartRepo, orderRepo)

	handler := http.NewHandler(cartService, orderService)
	server := http.NewServer(handler)

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
