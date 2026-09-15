package oracleapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
	oraclelighter "github.com/ogtrading/overnight-strategy/internal/oracle/normalize/lighter"
)

type indexedPackage struct {
	manifest Manifest
	index    IndexManifest
	quality  model.Quality
	windows  []certifiedWindow
}

func (s *Server) scanIndexedArchiveFiles(r *http.Request, in analyticsInput, rel string, consume func([]byte) error) ([]string, []string, bool, error) {
	dates, err := chicagoDates(in.From, in.To)
	if err != nil {
		return nil, nil, true, err
	}
	stream, asset := "", in.Asset
	switch {
	case strings.HasSuffix(rel, "/liquidity_observations.jsonl.zst"):
		stream = "liquidity"
	case rel == "market_stats.jsonl.zst":
		stream, asset = "open_interest", "ALL"
	default:
		return nil, nil, false, nil
	}
	packages := make([]indexedPackage, 0, len(dates))
	for _, date := range dates {
		manifest, _, err := s.find("lighter-" + date)
		if err != nil {
			return nil, nil, false, nil
		}
		quality, _, err := usableQuality(manifest, in.Allow)
		if err != nil {
			return nil, nil, true, err
		}
		spec, ok := findManifestFile(manifest, rel)
		index, indexErr := loadIndexManifest(filepath.Join(s.indexRoot, manifest.PackageID, "INDEX.json"))
		if !ok || indexErr != nil || index.SchemaVersion != indexSchema || index.Sources[rel] != spec.SHA256 {
			return nil, nil, false, nil
		}
		packages = append(packages, indexedPackage{manifest: manifest, index: index, quality: quality})
	}
	var packageIDs, qualities []string
	for _, pkg := range packages {
		for _, partition := range pkg.index.Partitions {
			if partition.Asset != asset || partition.Stream != stream || !partition.Hour.Before(in.To) || !partition.Hour.Add(time.Hour).After(in.From) {
				continue
			}
			path := filepath.Join(s.indexRoot, pkg.manifest.PackageID, filepath.FromSlash(partition.Path))
			if err := s.verifyFile(path, partition.SHA256); err != nil {
				return packageIDs, qualities, true, fmt.Errorf("indexed partition %s: %w", partition.Path, err)
			}
			file, err := os.Open(path)
			if err != nil {
				return packageIDs, qualities, true, err
			}
			decoder, err := zstd.NewReader(file)
			if err != nil {
				file.Close()
				return packageIDs, qualities, true, err
			}
			scanner := bufio.NewScanner(decoder)
			scanner.Buffer(make([]byte, 64<<10), 32<<20)
			for scanner.Scan() {
				if err := r.Context().Err(); err != nil {
					decoder.Close()
					file.Close()
					return packageIDs, qualities, true, err
				}
				if err := consume(scanner.Bytes()); err != nil {
					decoder.Close()
					file.Close()
					return packageIDs, qualities, true, err
				}
			}
			scanErr := scanner.Err()
			decoder.Close()
			file.Close()
			if scanErr != nil {
				return packageIDs, qualities, true, scanErr
			}
		}
		packageIDs = append(packageIDs, pkg.manifest.PackageID)
		qualities = appendUnique(qualities, string(pkg.quality))
	}
	return packageIDs, qualities, true, nil
}

func (s *Server) walkIndexedAnalyticTrades(r *http.Request, in analyticsInput, consume func(analyticTrade)) (queryResponse, int, bool, error) {
	result := queryResponse{Events: []model.Envelope{}, Packages: []string{}, Quality: []string{}, QueryMode: "INDEXED_HOURLY"}
	dates, err := chicagoDates(in.From, in.To)
	if err != nil {
		return result, 500, true, err
	}
	packages := make([]indexedPackage, 0, len(dates))
	for _, date := range dates {
		manifest, dir, err := s.find("lighter-" + date)
		if err != nil {
			return result, 0, false, nil
		}
		quality, status, err := usableQuality(manifest, in.Allow)
		if err != nil {
			return result, status, true, err
		}
		windows, err := loadCertifiedWindows(dir, in.Asset)
		if err != nil && quality != model.QualityChecksumVerifiedUncertified {
			return result, 422, true, err
		}
		index, err := loadIndexManifest(filepath.Join(s.indexRoot, manifest.PackageID, "INDEX.json"))
		rel := "asset=" + in.Asset + "/trade_flow.jsonl.zst"
		spec, sourceExists := findManifestFile(manifest, rel)
		if err != nil || index.SchemaVersion != indexSchema || index.PackageID != manifest.PackageID || !sourceExists || index.Sources[rel] != spec.SHA256 {
			return result, 0, false, nil
		}
		packages = append(packages, indexedPackage{manifest: manifest, index: index, quality: quality, windows: windows})
	}
	for _, pkg := range packages {
		sequence := uint64(1)
		for _, partition := range pkg.index.Partitions {
			// Include adjacent receipt hours because the exchange event timestamp
			// can straddle an hour boundary relative to local receipt time.
			if partition.Asset != in.Asset || partition.Stream != "trade" || !partition.Hour.Before(in.To.Add(time.Hour)) || !partition.Hour.Add(time.Hour).After(in.From.Add(-time.Hour)) {
				continue
			}
			path := filepath.Join(s.indexRoot, pkg.manifest.PackageID, filepath.FromSlash(partition.Path))
			if err := s.verifyFile(path, partition.SHA256); err != nil {
				return result, 422, true, fmt.Errorf("indexed partition %s: %w", partition.Path, err)
			}
			file, err := os.Open(path)
			if err != nil {
				return result, 500, true, err
			}
			decoder, err := zstd.NewReader(file)
			if err != nil {
				file.Close()
				return result, 500, true, err
			}
			scanner := bufio.NewScanner(decoder)
			scanner.Buffer(make([]byte, 64<<10), 32<<20)
			for scanner.Scan() {
				if err := r.Context().Err(); err != nil {
					decoder.Close()
					file.Close()
					return result, 499, true, err
				}
				var row archiveRow
				if json.Unmarshal(scanner.Bytes(), &row) != nil {
					continue
				}
				events, err := oraclelighter.Normalize(row.Event, oraclelighter.Context{Asset: in.Asset, ReceivedAt: row.ReceivedAt, ConnectionID: row.ConnectionID, FirstSequence: sequence, DefaultQuality: pkg.quality})
				if err != nil {
					continue
				}
				sequence += uint64(len(events))
				for _, event := range events {
					if event.Stream != model.StreamTrade {
						continue
					}
					at := eventTime(event)
					if at.Before(in.From) || !at.Before(in.To) {
						continue
					}
					if len(pkg.windows) > 0 && !inCertifiedWindow(event.OracleReceivedAt, pkg.windows) {
						result.Excluded++
						continue
					}
					var value model.Trade
					if json.Unmarshal(event.Payload, &value) != nil {
						continue
					}
					price, pe := strconv.ParseFloat(value.Price, 64)
					size, se := strconv.ParseFloat(value.Size, 64)
					if pe == nil && se == nil && price > 0 && size > 0 {
						consume(analyticTrade{At: at, Price: price, Size: size, Notional: price * size, Buy: strings.EqualFold(value.AggressorSide, "BUY")})
					}
				}
			}
			scanErr := scanner.Err()
			decoder.Close()
			file.Close()
			if scanErr != nil {
				return result, 500, true, scanErr
			}
		}
		result.Packages = append(result.Packages, pkg.manifest.PackageID)
		result.Quality = appendUnique(result.Quality, string(pkg.quality))
	}
	return result, 200, true, nil
}
