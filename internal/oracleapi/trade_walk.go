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

	"github.com/klauspost/compress/zstd"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
	oraclelighter "github.com/ogtrading/overnight-strategy/internal/oracle/normalize/lighter"
)

// walkAnalyticTrades streams checksum-verified archive trades through consume.
// It deliberately avoids retaining normalized envelopes, allowing rolling profiles
// to operate with memory proportional to distinct price levels rather than trades.
func (s *Server) walkAnalyticTrades(r *http.Request, in analyticsInput, consume func(analyticTrade)) (queryResponse, int, error) {
	dates, err := chicagoDates(in.From, in.To)
	result := queryResponse{Events: []model.Envelope{}, Packages: []string{}, Quality: []string{}}
	if err != nil {
		return result, 500, err
	}
	for _, date := range dates {
		manifest, dir, err := s.find("lighter-" + date)
		if err != nil {
			return result, 404, fmt.Errorf("archive for Chicago date %s is unavailable", date)
		}
		quality, status, err := usableQuality(manifest, in.Allow)
		if err != nil {
			return result, status, err
		}
		if !contains(manifest.Assets, in.Asset) {
			return result, 404, fmt.Errorf("asset %s is absent from %s", in.Asset, manifest.PackageID)
		}
		windows, err := loadCertifiedWindows(dir, in.Asset)
		if err != nil && quality != model.QualityChecksumVerifiedUncertified {
			return result, 422, err
		}
		if quality == model.QualityCertifiedWithExcludedIntervals && len(windows) == 0 {
			quality = model.QualityChecksumVerifiedUncertified
		}
		rel := "asset=" + in.Asset + "/trade_flow.jsonl.zst"
		spec, ok := findManifestFile(manifest, rel)
		if !ok {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := s.verifyFile(path, spec.SHA256); err != nil {
			return result, 422, err
		}
		file, err := os.Open(path)
		if err != nil {
			return result, 500, err
		}
		decoder, err := zstd.NewReader(file)
		if err != nil {
			file.Close()
			return result, 500, err
		}
		scanner := bufio.NewScanner(decoder)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		sequence := uint64(1)
		for scanner.Scan() {
			if err := r.Context().Err(); err != nil {
				decoder.Close()
				file.Close()
				return result, 499, err
			}
			var row archiveRow
			if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
				decoder.Close()
				file.Close()
				return result, 500, err
			}
			events, err := oraclelighter.Normalize(row.Event, oraclelighter.Context{Asset: in.Asset, ReceivedAt: row.ReceivedAt, ConnectionID: row.ConnectionID, FirstSequence: sequence, DefaultQuality: quality})
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
				if len(windows) > 0 && !inCertifiedWindow(event.OracleReceivedAt, windows) {
					result.Excluded++
					continue
				}
				var value model.Trade
				if json.Unmarshal(event.Payload, &value) != nil {
					continue
				}
				price, pe := strconv.ParseFloat(value.Price, 64)
				size, se := strconv.ParseFloat(value.Size, 64)
				if pe != nil || se != nil || price <= 0 || size <= 0 {
					continue
				}
				consume(analyticTrade{At: at, Price: price, Size: size, Notional: price * size, Buy: strings.EqualFold(value.AggressorSide, "BUY")})
			}
		}
		scanErr := scanner.Err()
		decoder.Close()
		file.Close()
		if scanErr != nil {
			return result, 500, scanErr
		}
		result.Packages = append(result.Packages, manifest.PackageID)
		result.Quality = appendUnique(result.Quality, string(quality))
	}
	if len(result.Packages) == 0 {
		return result, 404, fmt.Errorf("trade stream is unavailable in matching archives")
	}
	return result, 200, nil
}
