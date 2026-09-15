package main

import (
	"flag"
	"log"

	"github.com/ogtrading/overnight-strategy/internal/oracleapi"
)

func main() {
	root := flag.String("root", "/mnt/trading/recorder/lighter", "sealed recorder archive root")
	indexRoot := flag.String("index-root", "/mnt/trading/oracle/index", "derived Oracle index root")
	packageID := flag.String("package", "", "sealed package ID to index")
	flag.Parse()
	if *packageID == "" {
		log.Fatal("-package is required")
	}
	if err := oracleapi.BuildIndexes(*root, *indexRoot, *packageID); err != nil {
		log.Fatal(err)
	}
	log.Printf("Oracle hourly trade index ready package=%s", *packageID)
}
