package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ogtrading/overnight-strategy/internal/recordercert"
	"github.com/ogtrading/overnight-strategy/internal/universe"
	"os"
)

func main() {
	dir := flag.String("dir", "", "closed recorder date directory")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}
	symbols := []string{}
	for _, a := range universe.All() {
		symbols = append(symbols, a.Symbol)
	}
	cert, err := recordercert.Certify(*dir, recordercert.SymbolsSorted(symbols))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(cert)
	if !cert.Pass {
		os.Exit(1)
	}
}
