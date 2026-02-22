// forged is the Forge server daemon. It runs on the DigitalOcean droplet and
// provides: git hosting, agent message bus, OAuth, the mesh network, and a
// web dashboard for monitoring agent activity.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/api"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/bus"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/store"
)

func main() {
	addr := flag.String("addr", ":8420", "listen address")
	dataDir := flag.String("data", "/var/lib/forge", "data directory for repos and DB")
	flag.Parse()

	// Initialize persistent store.
	db, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer db.Close()

	// Create the message bus.
	hub := bus.NewHub()
	go hub.Run()

	// Build the HTTP server with all routes.
	srv := api.NewServer(hub, db, *dataDir)
	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv.Router(),
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("forge listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hub.Shutdown()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("forge stopped")
}
