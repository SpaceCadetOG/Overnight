package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ogtrading/overnight-strategy/internal/packagevalidator"
)

type notification struct{ Bucket, Name, Generation string }
type push struct {
	Message struct {
		Data string `json:"data"`
	} `json:"message"`
}

func main() {
	client, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	h := &handler{client: client, validationBucket: os.Getenv("VALIDATION_BUCKET"), quarantineBucket: os.Getenv("QUARANTINE_BUCKET")}
	http.HandleFunc("/", h.serve)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type handler struct {
	client                             *storage.Client
	validationBucket, quarantineBucket string
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var envelope push
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&envelope); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var event struct {
		Bucket     string `json:"bucket"`
		Name       string `json:"name"`
		Generation string `json:"generation"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if !strings.HasSuffix(event.Name, "/MANIFEST.json") {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.validate(r.Context(), notification{Bucket: event.Bucket, Name: event.Name, Generation: event.Generation}); err != nil {
		log.Printf("validation failed: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) validate(ctx context.Context, event notification) error {
	prefix := strings.TrimSuffix(event.Name, "MANIFEST.json")
	temp, err := os.MkdirTemp("", "cloud-package-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	manifestPath := filepath.Join(temp, "MANIFEST.json")
	if err := h.download(ctx, event.Bucket, event.Name, manifestPath); err != nil {
		return err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest packagevalidator.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	for _, file := range manifest.Files {
		if err := h.download(ctx, event.Bucket, prefix+file.Path, filepath.Join(temp, filepath.FromSlash(file.Path))); err != nil {
			return err
		}
	}
	result := packagevalidator.Validate(temp)
	output, _ := json.MarshalIndent(map[string]any{"validated_at": time.Now().UTC(), "source_bucket": event.Bucket, "manifest_object": event.Name, "manifest_generation": event.Generation, "result": result}, "", "  ")
	output = append(output, '\n')
	targetBucket := h.validationBucket
	targetPrefix := "validation"
	if !result.Valid {
		targetBucket = h.quarantineBucket
		targetPrefix = "quarantine"
	}
	if targetBucket == "" {
		return fmt.Errorf("target bucket is not configured")
	}
	object := targetPrefix + "/" + manifest.PackageID + ".json"
	w := h.client.Bucket(targetBucket).Object(object).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	if _, err := w.Write(output); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil && !strings.Contains(err.Error(), "conditionNotMet") {
		return err
	}
	return nil
}

func (h *handler) download(ctx context.Context, bucket, name, path string) error {
	reader, err := h.client.Bucket(bucket).Object(name).NewReader(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
