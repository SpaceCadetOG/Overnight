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

	"github.com/ogtrading/overnight-strategy/internal/buildinfo"
	"github.com/ogtrading/overnight-strategy/internal/collector"
	"github.com/ogtrading/overnight-strategy/internal/notify"
	"github.com/ogtrading/overnight-strategy/internal/store"
)

func main() {
	root := flag.String("store", "data/live/lighter", "append-only event store")
	port := flag.String("health-port", "8082", "health endpoint port")
	flag.Parse()
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal(err)
	}
	events, err := store.NewDailyJSONL(*root, location)
	if err != nil {
		fatal(err)
	}
	if err := events.Append("collector_metadata", map[string]any{
		"recorded_at": time.Now().UTC(), "schema_version": 1,
		"collector_version": buildinfo.Version, "git_commit": buildinfo.Commit,
		"built_at": buildinfo.BuiltAt,
	}); err != nil {
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
	notify.FromEnvironment().BestEffort("Market Recorder Online", "TradePi Lighter recorder started\nExpected coverage: 12/12 markets", "high", "satellite,white_check_mark")
	if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
