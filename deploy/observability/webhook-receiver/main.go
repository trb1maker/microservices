package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := ":5001"
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		addr = v
	}

	http.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("alertmanager webhook (raw): %s", string(body))
		} else {
			encoded, _ := json.Marshal(payload)
			log.Printf("alertmanager webhook: %s", string(encoded))
		}

		w.WriteHeader(http.StatusOK)
	})

	log.Printf("webhook receiver listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
