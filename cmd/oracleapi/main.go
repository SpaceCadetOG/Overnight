package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/buildinfo"
	"github.com/ogtrading/overnight-strategy/internal/oracleapi"
)

func main() {
	root := flag.String("root", "/mnt/trading/recorder/lighter", "sealed recorder archive root")
	address := flag.String("listen", "127.0.0.1:8083", "read-only API listen address")
	flag.Parse()
	server, err := oracleapi.New(*root, buildinfo.Version, buildinfo.Commit)
	if err != nil {
		log.Fatal(err)
	}
	httpServer := &http.Server{Addr: *address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Market Data Oracle read-only API listening on %s", *address)
	log.Fatal(httpServer.ListenAndServe())
}
