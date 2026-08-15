package packagevalidator

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type File struct {
	Path       string `json:"path"`
	Compressed int64  `json:"compressed_bytes"`
	Records    uint64 `json:"records"`
	SHA256     string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion     int       `json:"schema_version"`
	PackageID         string    `json:"package_id"`
	CollectorVersion  string    `json:"collector_version"`
	CollectorCommit   string    `json:"collector_commit"`
	Date              string    `json:"date"`
	Complete          bool      `json:"complete"`
	Files             []File    `json:"files"`
	Missing           []string  `json:"missing_assets"`
	Assets            []string  `json:"assets"`
	FirstEvent        time.Time `json:"first_event"`
	LastEvent         time.Time `json:"last_event"`
	RecorderCertified bool      `json:"recorder_certified"`
	NonceGaps         uint64    `json:"nonce_gaps"`
	WSErrors          uint64    `json:"websocket_errors"`
}

type Result struct {
	Valid     bool     `json:"valid"`
	PackageID string   `json:"package_id"`
	Files     int      `json:"files"`
	Records   uint64   `json:"records"`
	Errors    []string `json:"errors,omitempty"`
}

func Validate(dir string) Result {
	result := Result{}
	data, err := os.ReadFile(filepath.Join(dir, "MANIFEST.json"))
	if err != nil {
		result.Errors = append(result.Errors, "MANIFEST_UNREADABLE: "+err.Error())
		return result
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		result.Errors = append(result.Errors, "MANIFEST_INVALID_JSON: "+err.Error())
		return result
	}
	result.PackageID = manifest.PackageID
	if manifest.SchemaVersion < 3 {
		result.Errors = append(result.Errors, "MANIFEST_SCHEMA_UNSUPPORTED")
	}
	if manifest.PackageID == "" {
		result.Errors = append(result.Errors, "PACKAGE_ID_MISSING")
	}
	if manifest.CollectorVersion == "" || manifest.CollectorCommit == "" {
		result.Errors = append(result.Errors, "COLLECTOR_IDENTITY_MISSING")
	}
	if manifest.CollectorCommit == "unknown" {
		result.Errors = append(result.Errors, "COLLECTOR_COMMIT_UNKNOWN")
	}
	if manifest.FirstEvent.IsZero() || manifest.LastEvent.IsZero() || !manifest.LastEvent.After(manifest.FirstEvent) {
		result.Errors = append(result.Errors, "COVERAGE_INVALID")
	}
	if len(manifest.Assets) != 12 {
		result.Errors = append(result.Errors, fmt.Sprintf("ASSET_COUNT_INVALID: %d", len(manifest.Assets)))
	}
	if manifest.NonceGaps != 0 || manifest.WSErrors != 0 {
		result.Errors = append(result.Errors, "RECORDER_STREAM_ERRORS")
	}
	if !manifest.RecorderCertified {
		result.Errors = append(result.Errors, "RECORDER_NOT_CERTIFIED")
	}
	if !manifest.Complete {
		result.Errors = append(result.Errors, "PACKAGE_INCOMPLETE")
	}
	if len(manifest.Missing) > 0 {
		result.Errors = append(result.Errors, "ASSETS_MISSING")
	}
	seen := map[string]bool{}
	certificateListed := false
	for _, item := range manifest.Files {
		clean := filepath.Clean(item.Path)
		if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || seen[clean] {
			result.Errors = append(result.Errors, "FILE_PATH_INVALID: "+item.Path)
			continue
		}
		seen[clean] = true
		if clean == "RECORDER_CERTIFICATE.json" {
			certificateListed = true
		}
		path := filepath.Join(dir, clean)
		digest, err := checksum(path)
		if err != nil {
			result.Errors = append(result.Errors, "FILE_UNREADABLE: "+item.Path)
			continue
		}
		if !strings.EqualFold(digest, item.SHA256) {
			result.Errors = append(result.Errors, "CHECKSUM_MISMATCH: "+item.Path)
			continue
		}
		rows, err := countRows(path)
		if err != nil {
			result.Errors = append(result.Errors, "DECOMPRESSION_FAILED: "+item.Path)
			continue
		}
		if rows != item.Records {
			result.Errors = append(result.Errors, fmt.Sprintf("RECORD_COUNT_MISMATCH: %s expected=%d actual=%d", item.Path, item.Records, rows))
			continue
		}
		result.Files++
		result.Records += rows
	}
	if !certificateListed {
		result.Errors = append(result.Errors, "RECORDER_CERTIFICATE_MISSING")
	} else {
		body, err := os.ReadFile(filepath.Join(dir, "RECORDER_CERTIFICATE.json"))
		var certificate struct {
			Pass bool `json:"pass"`
		}
		if err != nil || json.Unmarshal(body, &certificate) != nil || !certificate.Pass {
			result.Errors = append(result.Errors, "RECORDER_CERTIFICATE_INVALID")
		}
	}
	result.Valid = len(result.Errors) == 0 && result.Files == len(manifest.Files) && result.Files > 0
	return result
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

func countRows(path string) (uint64, error) {
	if !strings.HasSuffix(path, ".zst") {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return 0, err
		}
		return 1, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader, err := zstd.NewReader(file)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	var count uint64
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var value any
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return count, err
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}
