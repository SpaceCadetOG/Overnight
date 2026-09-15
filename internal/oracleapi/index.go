package oracleapi

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

const indexSchema = "oracle-hourly-index-v1"

type IndexPartition struct {
	Asset   string    `json:"asset"`
	Stream  string    `json:"stream"`
	Hour    time.Time `json:"hour"`
	Path    string    `json:"path"`
	Records uint64    `json:"records"`
	SHA256  string    `json:"sha256"`
	Bytes   int64     `json:"bytes"`
}

type IndexManifest struct {
	SchemaVersion string            `json:"schema_version"`
	PackageID     string            `json:"package_id"`
	Sources       map[string]string `json:"sources"`
	CreatedAt     time.Time         `json:"created_at"`
	Partitions    []IndexPartition  `json:"partitions"`
}

type partitionWriter struct {
	file    *os.File
	encoder *zstd.Encoder
	buffer  *bufio.Writer
	records uint64
}

// BuildIndexes creates checksum-bound, hourly query shards without
// modifying the immutable recorder packages. A completed package is published
// by one atomic rename, so readers never observe a partial index.
func BuildIndexes(archiveRoot, indexRoot, packageID string) error {
	server, err := NewWithIndex(archiveRoot, indexRoot, "indexer", "indexer")
	if err != nil {
		return err
	}
	manifest, packageDir, err := server.find(packageID)
	if err != nil {
		return err
	}
	final := filepath.Join(indexRoot, packageID)
	expectedSources := indexSources(manifest)
	if current, err := loadIndexManifest(filepath.Join(final, "INDEX.json")); err == nil && current.SchemaVersion == indexSchema && reflect.DeepEqual(current.Sources, expectedSources) {
		return nil
	}
	if err := os.MkdirAll(indexRoot, 0o750); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(indexRoot, ".building-"+packageID+"-")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(temporary)
		}
	}()

	index := IndexManifest{SchemaVersion: indexSchema, PackageID: packageID, Sources: expectedSources, CreatedAt: time.Now().UTC()}
	for _, asset := range manifest.Assets {
		for _, source := range []struct{ rel, stream string }{
			{"asset=" + asset + "/trade_flow.jsonl.zst", "trade"},
			{"asset=" + asset + "/liquidity_observations.jsonl.zst", "liquidity"},
		} {
			spec, ok := findManifestFile(manifest, source.rel)
			if !ok {
				continue
			}
			path := filepath.Join(packageDir, filepath.FromSlash(source.rel))
			if err := server.verifyFile(path, spec.SHA256); err != nil {
				return fmt.Errorf("%s: %w", source.rel, err)
			}
			partitions, err := buildHourlyPartitions(path, temporary, asset, source.stream, sourceTimestamp(source.stream))
			if err != nil {
				return fmt.Errorf("%s: %w", source.rel, err)
			}
			index.Partitions = append(index.Partitions, partitions...)
		}
	}
	if spec, ok := findManifestFile(manifest, "market_stats.jsonl.zst"); ok {
		path := filepath.Join(packageDir, "market_stats.jsonl.zst")
		if err := server.verifyFile(path, spec.SHA256); err != nil {
			return fmt.Errorf("market_stats.jsonl.zst: %w", err)
		}
		partitions, err := buildHourlyPartitions(path, temporary, "ALL", "open_interest", sourceTimestamp("open_interest"))
		if err != nil {
			return fmt.Errorf("market_stats.jsonl.zst: %w", err)
		}
		index.Partitions = append(index.Partitions, partitions...)
	}
	if len(index.Partitions) == 0 {
		return errors.New("package contains no trade streams")
	}
	sort.Slice(index.Partitions, func(i, j int) bool {
		if index.Partitions[i].Asset == index.Partitions[j].Asset {
			return index.Partitions[i].Hour.Before(index.Partitions[j].Hour)
		}
		return index.Partitions[i].Asset < index.Partitions[j].Asset
	})
	body, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(temporary, "INDEX.json"), body, 0o640); err != nil {
		return err
	}
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("index destination already exists but is not reusable: %s", final)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(temporary, final); err != nil {
		return err
	}
	committed = true
	return nil
}

func buildHourlyPartitions(source, root, asset, stream string, timestamp func([]byte) time.Time) ([]IndexPartition, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	writers := map[string]*partitionWriter{}
	scanner := bufio.NewScanner(zr)
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	for scanner.Scan() {
		at := timestamp(scanner.Bytes())
		if at.IsZero() {
			continue
		}
		hour := at.UTC().Truncate(time.Hour)
		key := hour.Format("20060102T15")
		writer := writers[key]
		if writer == nil {
			dir := filepath.Join(root, "asset="+asset, stream)
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, err
			}
			file, err := os.OpenFile(filepath.Join(dir, key+".jsonl.zst"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
			if err != nil {
				return nil, err
			}
			encoder, err := zstd.NewWriter(file, zstd.WithEncoderLevel(zstd.SpeedFastest))
			if err != nil {
				file.Close()
				return nil, err
			}
			writer = &partitionWriter{file: file, encoder: encoder, buffer: bufio.NewWriterSize(encoder, 256<<10)}
			writers[key] = writer
		}
		if _, err := writer.buffer.Write(scanner.Bytes()); err != nil {
			return nil, err
		}
		if err := writer.buffer.WriteByte('\n'); err != nil {
			return nil, err
		}
		writer.records++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(writers))
	for key := range writers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writer := writers[key]
		if err := writer.buffer.Flush(); err != nil {
			return nil, err
		}
		if err := writer.encoder.Close(); err != nil {
			return nil, err
		}
		if err := writer.file.Sync(); err != nil {
			return nil, err
		}
		if err := writer.file.Close(); err != nil {
			return nil, err
		}
	}
	partitions := make([]IndexPartition, 0, len(keys))
	for _, key := range keys {
		rel := filepath.ToSlash(filepath.Join("asset="+asset, stream, key+".jsonl.zst"))
		path := filepath.Join(root, filepath.FromSlash(rel))
		digest, size, err := fileDigest(path)
		if err != nil {
			return nil, err
		}
		hour, _ := time.Parse("20060102T15", key)
		partitions = append(partitions, IndexPartition{Asset: asset, Stream: stream, Hour: hour.UTC(), Path: rel, Records: writers[key].records, SHA256: digest, Bytes: size})
	}
	return partitions, nil
}

func sourceTimestamp(stream string) func([]byte) time.Time {
	return func(line []byte) time.Time {
		switch stream {
		case "liquidity":
			var row struct {
				WindowStart time.Time `json:"window_start"`
			}
			_ = json.Unmarshal(line, &row)
			return row.WindowStart
		case "open_interest":
			var row struct {
				ReceivedAt time.Time `json:"received_at"`
				Event      struct {
					Timestamp int64 `json:"timestamp"`
				} `json:"event"`
			}
			_ = json.Unmarshal(line, &row)
			if row.Event.Timestamp > 0 {
				return time.UnixMilli(row.Event.Timestamp).UTC()
			}
			return row.ReceivedAt
		default:
			var row archiveRow
			_ = json.Unmarshal(line, &row)
			return row.ReceivedAt
		}
	}
}

func indexSources(manifest Manifest) map[string]string {
	result := map[string]string{}
	for _, file := range manifest.Files {
		if strings.HasSuffix(file.Path, "/trade_flow.jsonl.zst") || strings.HasSuffix(file.Path, "/liquidity_observations.jsonl.zst") || file.Path == "market_stats.jsonl.zst" {
			result[file.Path] = file.SHA256
		}
	}
	return result
}

func fileDigest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func loadIndexManifest(path string) (IndexManifest, error) {
	var manifest IndexManifest
	body, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	err = json.Unmarshal(body, &manifest)
	return manifest, err
}
