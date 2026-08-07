package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JSONL is an append-only durable event store. Each stream is isolated in its
// own file so collectors can be replayed and migrated into a database later.
type JSONL struct {
	root string
	mu   sync.Mutex
}

func NewJSONL(root string) (*JSONL, error) {
	if root == "" {
		return nil, fmt.Errorf("store root is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	return &JSONL{root: root}, nil
}

func (s *JSONL) Append(stream string, value any) error {
	if stream == "" || filepath.Base(stream) != stream {
		return fmt.Errorf("invalid stream %q", stream)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(filepath.Join(s.root, stream+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("open stream %s: %w", stream, err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(value); err != nil {
		return fmt.Errorf("append stream %s: %w", stream, err)
	}
	return file.Sync()
}

func ReadAll[T any](root, stream string) ([]T, error) {
	file, err := os.Open(filepath.Join(root, stream+".jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := []T{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var value T
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, scanner.Err()
}
