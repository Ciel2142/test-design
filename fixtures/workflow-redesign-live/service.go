// A real, deliberately small service subject for the redesign's live journey.
// Change unitPriceCents in a disposable Git subject to distinguish revisions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const unitPriceCents = 125

var revision string

func main() {
	if len(os.Args) != 2 || revision == "" {
		log.Fatal("usage: revision-stamped service <owned-directory>")
	}
	root := os.Args[1]
	var state struct {
		Token string `json:"token"`
	}
	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Token == "" {
		log.Fatal("missing private run token")
	}
	listener, err := net.Listen("unix", filepath.Join(root, "service.sock"))
	if err != nil {
		log.Fatal(err)
	}
	stop := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ready":true}`))
	})
	mux.HandleFunc("POST /quote", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Quantity int `json:"quantity"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil || input.Quantity < 1 || input.Quantity > 1000 {
			http.Error(w, "quantity must be an integer from 1 to 1000", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Quantity   int    `json:"quantity"`
			TotalCents int    `json:"totalCents"`
			Revision   string `json:"revision"`
		}{input.Quantity, input.Quantity * unitPriceCents, revision})
	})
	mux.HandleFunc("POST /shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		select {
		case stop <- struct{}{}:
		default:
		}
	})
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+state.Token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			mux.ServeHTTP(w, r)
		}),
	}
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.Serve(listener) }()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	select {
	case <-stop:
	case <-ctx.Done():
	case err := <-errorsCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
	shutdown, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	if err := server.Shutdown(shutdown); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stopped"), []byte("stopped\n"), 0600); err != nil {
		log.Fatal(err)
	}
}
