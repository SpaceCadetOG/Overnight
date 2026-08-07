package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/collector"
	"github.com/ogtrading/overnight-strategy/internal/store"
)

func main() {
	root := flag.String("store", "data/live/lighter", "append-only event store")
	port := flag.String("health-port", "8082", "health endpoint port")
	flag.Parse()
	events, err := store.NewJSONL(*root)
	if err != nil {
		fatal(err)
	}
	c := collector.New(os.Getenv("LIGHTER_BASE_URL"), os.Getenv("LIGHTER_WS_URL"), events)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: ":" + *port, Handler: c.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, err)
			stop()
		}
	}()
	fmt.Printf("lighter collector running store=%s health=:%s automated_orders=false\n", *root, *port)
	if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
