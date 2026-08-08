package packagevalidator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestValidatePackageAndRejectTampering(t *testing.T) {
	if _, err := exec.LookPath("zstd"); err != nil {
		t.Skip("zstd unavailable")
	}
	dir := t.TempDir()
	raw := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(raw, []byte("{\"a\":1}\n{\"a\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	compressed := raw + ".zst"
	if out, err := exec.Command("zstd", "-q", raw, "-o", compressed).CombinedOutput(); err != nil {
		t.Fatalf("zstd: %v %s", err, out)
	}
	data, _ := os.ReadFile(compressed)
	sum := sha256.Sum256(data)
	manifest := Manifest{SchemaVersion: 2, PackageID: "lighter-2026-08-08", CollectorVersion: "v1", CollectorCommit: "abc", Date: "2026-08-08", Complete: true, Files: []File{{Path: "events.jsonl.zst", Records: 2, SHA256: hex.EncodeToString(sum[:])}}}
	body, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "MANIFEST.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if result := Validate(dir); !result.Valid {
		t.Fatalf("valid package rejected: %+v", result)
	}
	if err := os.WriteFile(compressed, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := Validate(dir); result.Valid {
		t.Fatal("tampered package accepted")
	}
}
