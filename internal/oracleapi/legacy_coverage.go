package oracleapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/klauspost/compress/zstd"
)

type observedWindow struct {
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Certified bool      `json:"certified"`
	Connected bool      `json:"connected"`
	Basis     string    `json:"basis"`
}

type connectionMark struct {
	At        time.Time
	Connected bool
}

// legacyObservedWindows reconstructs recorder-level connectivity only. It does
// not certify the market stream or upgrade the package quality. Consumers may
// use a fully enclosed minute as a zero-trade observation only in explicitly
// labeled exploratory research.
func (s *Server) legacyObservedWindows(manifest Manifest, dir string) ([]observedWindow, error) {
	if manifest.SchemaVersion >= 4 {
		return nil, fmt.Errorf("legacy coverage is only available for schema 2/3 packages")
	}
	marks := []connectionMark{}
	if !manifest.FirstEvent.IsZero() {
		marks = append(marks, connectionMark{At: manifest.FirstEvent.UTC(), Connected: true})
	}
	if err := s.scanLegacyCoverageFile(manifest, dir, "collector_events.jsonl.zst", func(line []byte) {
		var row struct {
			Channel    string    `json:"channel"`
			ReceivedAt time.Time `json:"received_at"`
			Event      struct {
				Type string `json:"type"`
			} `json:"event"`
		}
		if json.Unmarshal(line, &row) == nil && !row.ReceivedAt.IsZero() && (row.Channel == "connected" || row.Event.Type == "connected") {
			marks = append(marks, connectionMark{At: row.ReceivedAt.UTC(), Connected: true})
		}
	}); err != nil {
		return nil, err
	}
	if err := s.scanLegacyCoverageFile(manifest, dir, "collector_reconnects.jsonl.zst", func(line []byte) {
		var row struct {
			RecordedAt time.Time `json:"recorded_at"`
		}
		if json.Unmarshal(line, &row) == nil && !row.RecordedAt.IsZero() {
			marks = append(marks, connectionMark{At: row.RecordedAt.UTC(), Connected: false})
		}
	}); err != nil {
		return nil, err
	}
	if len(marks) == 0 || manifest.LastEvent.IsZero() {
		return nil, fmt.Errorf("legacy recorder connection evidence is incomplete")
	}
	sort.SliceStable(marks, func(i, j int) bool {
		if marks[i].At.Equal(marks[j].At) {
			return !marks[i].Connected && marks[j].Connected
		}
		return marks[i].At.Before(marks[j].At)
	})
	connected := false
	var start time.Time
	windows := []observedWindow{}
	for _, mark := range marks {
		if mark.Connected {
			if !connected {
				start, connected = mark.At, true
			}
			continue
		}
		if connected && mark.At.After(start) {
			windows = append(windows, observedWindow{start, mark.At, false, true, "collector_connection_logs"})
		}
		connected = false
	}
	end := manifest.LastEvent.UTC()
	if connected && end.After(start) {
		windows = append(windows, observedWindow{start, end, false, true, "collector_connection_logs"})
	}
	return windows, nil
}

func (s *Server) scanLegacyCoverageFile(manifest Manifest, dir, rel string, consume func([]byte)) error {
	spec, ok := findManifestFile(manifest, rel)
	if !ok {
		return fmt.Errorf("legacy coverage source %s is unavailable", rel)
	}
	path := filepath.Join(dir, rel)
	if err := s.verifyFile(path, spec.SHA256); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return err
	}
	defer decoder.Close()
	scanner := bufio.NewScanner(decoder)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		consume(scanner.Bytes())
	}
	return scanner.Err()
}
