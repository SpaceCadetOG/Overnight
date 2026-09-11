package oracle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Oracle is a read-only product boundary. Keep account signing and execution
// imports out even if those packages live in the same repository.
func TestOracleSourceDoesNotImportExecution(t *testing.T) {
	root := "."
	for depth := 0; depth < 4; depth++ {
		if _, err := os.Stat(filepath.Join(root, "model")); err == nil {
			break
		}
		root = filepath.Join("..", root)
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, forbidden := range []string{"internal/execution", "internal/reconcile", "lighter-adapter"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("Oracle source %s imports forbidden boundary %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
