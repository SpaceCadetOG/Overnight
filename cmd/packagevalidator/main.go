package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ogtrading/overnight-strategy/internal/packagevalidator"
)

func main() {
	dir := flag.String("package", "", "sealed daily package directory")
	flag.Parse()
	if *dir == "" { fmt.Fprintln(os.Stderr, "package is required"); os.Exit(2) }
	result := packagevalidator.Validate(*dir)
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if !result.Valid { os.Exit(1) }
}
