package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/buildinfo"
	"github.com/ogtrading/overnight-strategy/internal/recordercert"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type fileManifest struct {
	Path          string    `json:"path"`
	Compressed    int64     `json:"compressed_bytes"`
	RawBytes      int64     `json:"raw_bytes"`
	Records       uint64    `json:"records"`
	FirstReceived time.Time `json:"first_received,omitempty"`
	LastReceived  time.Time `json:"last_received,omitempty"`
	SHA256        string    `json:"sha256"`
	WSErrors      uint64    `json:"websocket_errors,omitempty"`
	FirstSequence int64     `json:"first_sequence,omitempty"`
	LastSequence  int64     `json:"last_sequence,omitempty"`
}

type manifest struct {
	Schema            int            `json:"schema_version"`
	PackageID         string         `json:"package_id"`
	PriorPackageID    string         `json:"prior_package_id,omitempty"`
	CollectorVersion  string         `json:"collector_version"`
	CollectorCommit   string         `json:"collector_commit"`
	Exchange          string         `json:"exchange"`
	Date              string         `json:"date"`
	Timezone          string         `json:"timezone"`
	GeneratedAt       time.Time      `json:"generated_at"`
	Files             []fileManifest `json:"files"`
	Records           uint64         `json:"records"`
	RawBytes          int64          `json:"raw_bytes"`
	NonceGaps         uint64         `json:"nonce_gaps"`
	WSErrors          uint64         `json:"websocket_errors"`
	Reconnects        uint64         `json:"reconnects"`
	FirstEvent        time.Time      `json:"first_event,omitempty"`
	LastEvent         time.Time      `json:"last_event,omitempty"`
	Assets            []string       `json:"assets"`
	Missing           []string       `json:"missing_assets,omitempty"`
	Complete          bool           `json:"complete"`
	RecorderCertified bool           `json:"recorder_certified"`
}

func expectedAssets() []string {
	result := make([]string, 0, len(universe.All()))
	for _, asset := range universe.All() {
		result = append(result, asset.Symbol)
	}
	sort.Strings(result)
	return result
}

func main() {
	root := flag.String("root", "/mnt/trading/recorder/lighter", "collector storage root")
	day := flag.String("date", "", "Chicago date to archive (YYYY-MM-DD)")
	removeRaw := flag.Bool("remove-raw", true, "remove JSONL after verified compression")
	flag.Parse()
	if *day == "" {
		fatal(fmt.Errorf("date is required"))
	}
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal(err)
	}
	parsed, err := time.ParseInLocation("2006-01-02", *day, location)
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if err != nil || !parsed.Before(today) {
		fatal(fmt.Errorf("date must be a closed Chicago calendar day"))
	}
	dir := filepath.Join(*root, "date="+*day)
	entries, err := collectJSONL(dir)
	if err != nil {
		fatal(err)
	}
	if len(entries) == 0 {
		if _, err := os.Stat(filepath.Join(dir, "MANIFEST.json")); err == nil {
			fmt.Printf("already archived date=%s\n", *day)
			return
		}
		fatal(fmt.Errorf("no JSONL files in %s", dir))
	}
	m := manifest{Schema: 3, PackageID: "lighter-" + *day, Exchange: "lighter", Date: *day, Timezone: location.String(), GeneratedAt: time.Now().UTC(), CollectorVersion: buildinfo.Version, CollectorCommit: buildinfo.Commit}
	seenAssets := map[string]bool{}
	seenBooks := map[string]bool{}
	for _, path := range entries {
		info, err := inspect(path, dir)
		if err != nil {
			fatal(err)
		}
		compressed := path + ".zst"
		if err := compress(path, compressed); err != nil {
			fatal(err)
		}
		stat, err := os.Stat(compressed)
		if err != nil {
			fatal(err)
		}
		info.Compressed = stat.Size()
		info.SHA256, err = checksum(compressed)
		if err != nil {
			fatal(err)
		}
		m.Records += info.Records
		m.RawBytes += info.RawBytes
		m.WSErrors += info.WSErrors
		if !info.FirstReceived.IsZero() && (m.FirstEvent.IsZero() || info.FirstReceived.Before(m.FirstEvent)) {
			m.FirstEvent = info.FirstReceived
		}
		if info.LastReceived.After(m.LastEvent) {
			m.LastEvent = info.LastReceived
		}
		if strings.HasSuffix(info.Path, "collector_gaps.jsonl") {
			m.NonceGaps += info.Records
		}
		if strings.HasSuffix(info.Path, "collector_reconnects.jsonl") {
			m.Reconnects += info.Records
		}
		info.Path += ".zst"
		m.Files = append(m.Files, info)
		parts := strings.Split(filepath.ToSlash(info.Path), "/")
		if len(parts) > 1 && strings.HasPrefix(parts[0], "asset=") {
			asset := strings.TrimPrefix(parts[0], "asset=")
			seenAssets[asset] = true
			if strings.HasSuffix(info.Path, "orderbook_events.jsonl.zst") {
				seenBooks[asset] = true
			}
		}
		if *removeRaw {
			if err := os.Remove(path); err != nil {
				fatal(err)
			}
		}
	}
	for _, asset := range expectedAssets() {
		if seenAssets[asset] && seenBooks[asset] {
			m.Assets = append(m.Assets, asset)
		} else {
			m.Missing = append(m.Missing, asset)
		}
	}
	dayStart := parsed
	dayEnd := parsed.AddDate(0, 0, 1)
	coverageComplete := !m.FirstEvent.IsZero() && !m.FirstEvent.After(dayStart.Add(5*time.Minute)) && !m.LastEvent.Before(dayEnd.Add(-5*time.Minute))
	certificate, certErr := recordercert.Certify(dir, expectedAssets())
	if certErr == nil {
		body, _ := json.MarshalIndent(certificate, "", "  ")
		body = append(body, '\n')
		certPath := filepath.Join(dir, "RECORDER_CERTIFICATE.json")
		if err := os.WriteFile(certPath, body, 0640); err != nil {
			fatal(err)
		}
		digest, _ := checksum(certPath)
		m.Files = append(m.Files, fileManifest{Path: "RECORDER_CERTIFICATE.json", Compressed: int64(len(body)), RawBytes: int64(len(body)), Records: 1, SHA256: digest})
		m.RecorderCertified = certificate.Pass
	}
	m.Complete = len(m.Missing) == 0 && m.NonceGaps == 0 && m.WSErrors == 0 && coverageComplete && m.RecorderCertified && certErr == nil && m.CollectorCommit != "" && m.CollectorCommit != "unknown"
	if err := writeOutputs(dir, m); err != nil {
		fatal(err)
	}
	fmt.Printf("archived date=%s files=%d records=%d raw_bytes=%d nonce_gaps=%d\n", m.Date, len(m.Files), m.Records, m.RawBytes, m.NonceGaps)
}

func collectJSONL(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".jsonl") {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func inspect(path, root string) (fileManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileManifest{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return fileManifest{}, err
	}
	rel, _ := filepath.Rel(root, path)
	result := fileManifest{Path: filepath.ToSlash(rel), RawBytes: stat.Size()}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		result.Records++
		var row struct {
			ReceivedAt time.Time `json:"received_at"`
			RecordedAt time.Time `json:"recorded_at"`
			Sequence   int64     `json:"sequence"`
			Event      struct {
				Error json.RawMessage `json:"error"`
			} `json:"event"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) == nil {
			if row.ReceivedAt.IsZero() {
				row.ReceivedAt = row.RecordedAt
			}
			if row.Sequence > 0 {
				if result.FirstSequence == 0 {
					result.FirstSequence = row.Sequence
				}
				result.LastSequence = row.Sequence
			}
		}
		if !row.ReceivedAt.IsZero() {
			if result.FirstReceived.IsZero() {
				result.FirstReceived = row.ReceivedAt
			}
			result.LastReceived = row.ReceivedAt
			if len(row.Event.Error) > 0 && string(row.Event.Error) != "null" {
				result.WSErrors++
			}
		}
	}
	return result, scanner.Err()
}

func compress(source, destination string) error {
	temporary := destination + ".tmp"
	command := exec.Command("zstd", "-T0", "-3", "-q", "-f", source, "-o", temporary)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("compress %s: %w: %s", source, err, output)
	}
	return os.Rename(temporary, destination)
}

func checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeOutputs(dir string, value manifest) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "MANIFEST.json.tmp"), data, 0o640); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(dir, "MANIFEST.json.tmp"), filepath.Join(dir, "MANIFEST.json")); err != nil {
		return err
	}
	var lines strings.Builder
	for _, file := range value.Files {
		fmt.Fprintf(&lines, "%s  %s\n", file.SHA256, file.Path)
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(lines.String()), 0o640)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
