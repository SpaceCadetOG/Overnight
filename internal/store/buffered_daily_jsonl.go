package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type bufferedRecord struct {
	day    string
	stream string
	data   []byte
}

// BufferedDailyJSONL batches high-volume records and performs one fsync per
// file per batch. Append applies bounded backpressure rather than dropping
// events. Close drains and durably syncs every accepted record.
type BufferedDailyJSONL struct {
	root     string
	location *time.Location
	interval time.Duration
	records  chan bufferedRecord
	done     chan struct{}
	mu       sync.RWMutex
	err      error
	close    sync.Once
}

func NewBufferedDailyJSONL(root string, location *time.Location, interval time.Duration, capacity int) (*BufferedDailyJSONL, error) {
	if root == "" || location == nil || interval <= 0 || capacity <= 0 {
		return nil, fmt.Errorf("valid root, timezone, interval, and capacity are required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	s := &BufferedDailyJSONL{root: root, location: location, interval: interval, records: make(chan bufferedRecord, capacity), done: make(chan struct{})}
	go s.run()
	return s, nil
}

func (s *BufferedDailyJSONL) Append(stream string, value any) error {
	clean := filepath.Clean(stream)
	if stream == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid stream %q", stream)
	}
	if err := s.currentError(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode stream %s: %w", stream, err)
	}
	data = append(data, '\n')
	record := bufferedRecord{day: "date=" + time.Now().In(s.location).Format("2006-01-02"), stream: clean, data: data}
	select {
	case s.records <- record:
		return nil
	case <-s.done:
		if err := s.currentError(); err != nil {
			return err
		}
		return fmt.Errorf("buffered store is closed")
	}
}

func (s *BufferedDailyJSONL) Close() error {
	s.close.Do(func() { close(s.records); <-s.done })
	return s.currentError()
}

func (s *BufferedDailyJSONL) currentError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *BufferedDailyJSONL) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

func (s *BufferedDailyJSONL) run() {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	batch := make([]bufferedRecord, 0, 4096)
	flush := func() {
		if len(batch) == 0 || s.currentError() != nil {
			batch = batch[:0]
			return
		}
		if err := s.writeBatch(batch); err != nil {
			s.fail(err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case record, ok := <-s.records:
			if !ok {
				flush()
				return
			}
			batch = append(batch, record)
			if len(batch) >= cap(batch) {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (s *BufferedDailyJSONL) writeBatch(records []bufferedRecord) error {
	grouped := make(map[string]*bytes.Buffer)
	for _, record := range records {
		path := filepath.Join(s.root, record.day, record.stream+".jsonl")
		if grouped[path] == nil {
			grouped[path] = new(bytes.Buffer)
		}
		_, _ = grouped[path].Write(record.data)
	}
	for path, data := range grouped {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("create stream directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return fmt.Errorf("open stream: %w", err)
		}
		if _, err = file.Write(data.Bytes()); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("append stream: %w", err)
		}
		if closeErr != nil {
			return fmt.Errorf("close stream: %w", closeErr)
		}
	}
	return nil
}
