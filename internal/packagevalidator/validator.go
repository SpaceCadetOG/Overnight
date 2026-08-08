package packagevalidator

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type File struct {
	Path    string `json:"path"`
	Records uint64 `json:"records"`
	SHA256  string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion    int      `json:"schema_version"`
	PackageID        string   `json:"package_id"`
	CollectorVersion string   `json:"collector_version"`
	CollectorCommit  string   `json:"collector_commit"`
	Date             string   `json:"date"`
	Complete         bool     `json:"complete"`
	Files            []File   `json:"files"`
	Missing          []string `json:"missing_assets"`
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
	if manifest.SchemaVersion < 2 {
		result.Errors = append(result.Errors, "MANIFEST_SCHEMA_UNSUPPORTED")
	}
	if manifest.PackageID == "" {
		result.Errors = append(result.Errors, "PACKAGE_ID_MISSING")
	}
	if manifest.CollectorVersion == "" || manifest.CollectorCommit == "" {
		result.Errors = append(result.Errors, "COLLECTOR_IDENTITY_MISSING")
	}
	if !manifest.Complete {
		result.Errors = append(result.Errors, "PACKAGE_INCOMPLETE")
	}
	if len(manifest.Missing) > 0 {
		result.Errors = append(result.Errors, "ASSETS_MISSING")
	}
	seen := map[string]bool{}
	for _, item := range manifest.Files {
		clean := filepath.Clean(item.Path)
		if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || seen[clean] {
			result.Errors = append(result.Errors, "FILE_PATH_INVALID: "+item.Path)
			continue
		}
		seen[clean] = true
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
	command := exec.Command("zstd", "-q", "-dc", path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := command.Start(); err != nil {
		return 0, err
	}
	var count uint64
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		_ = command.Wait()
		return 0, err
	}
	return count, command.Wait()
}
