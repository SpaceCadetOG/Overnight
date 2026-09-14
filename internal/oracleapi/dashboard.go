package oracleapi

import (
	"html/template"
	"net/http"
	"strings"
	"time"
)

var dashboardTemplate = template.Must(template.New("oracle-dashboard").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="30"><title>Market Data Oracle</title>
<style>:root{color-scheme:dark}body{margin:0;background:#071014;color:#d9e6e8;font:14px ui-monospace,SFMono-Regular,Menlo,monospace}.wrap{max-width:1180px;margin:auto;padding:28px}.top{display:flex;justify-content:space-between;align-items:center}.ok{color:#32db9d}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:14px;margin-top:22px}.card{border:1px solid #24373c;background:#0b171b;padding:18px}.card h2{font-size:13px;color:#59c8e8;letter-spacing:.12em;text-transform:uppercase}.card a{display:block;color:#d9e6e8;text-decoration:none;padding:7px 0;border-bottom:1px solid #18272b}.card a:hover{color:#32db9d}.muted{color:#819499}.version{color:#e4b849}</style></head><body><main class="wrap">
<div class="top"><div><h1>Market Data Oracle</h1><p class="muted">Read-only market memory, analytics, quality and replay</p></div><div><span class="ok">● SERVICE ONLINE</span><br><span class="version">{{.Version}} · {{.Commit}}</span></div></div>
<div class="grid">
<section class="card"><h2>Service</h2><a href="/v1/health">Health and version</a><a href="/v1/readiness">Live readiness</a><a href="/v1/datasets">Archive catalog</a><a href="/v1/session-definitions">Session definitions</a></section>
<section class="card"><h2>Tape and books</h2><a href="/v1/trades?asset=BTC&amp;from={{.From}}&amp;to={{.To}}&amp;limit=100">BTC trade tape</a><a href="/v1/events?asset=BTC&amp;stream=trade,book_snapshot,book_delta,ticker&amp;from={{.From}}&amp;to={{.To}}&amp;limit=100">Normalized events</a><a href="/v1/books/BTC">Live BTC book</a><a href="/v1/books/BTC/at?at={{.At}}">BTC book at timestamp</a></section>
<section class="card"><h2>Trade analytics</h2><a href="/v1/candles?asset=BTC&amp;from={{.From}}&amp;to={{.To}}&amp;interval=5m">Candles</a><a href="/v1/profiles?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">Volume profile</a><a href="/v1/footprints?asset=BTC&amp;from={{.From}}&amp;to={{.To}}&amp;interval=5m">Footprint</a><a href="/v1/order-flow?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">CVD and order flow</a></section>
<section class="card"><h2>Market structure</h2><a href="/v1/sessions?at={{.At}}">Canonical sessions</a><a href="/v1/levels?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">Named levels</a><a href="/v1/zones?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">Structural zones</a></section>
<section class="card"><h2>Microstructure</h2><a href="/v1/heatmap?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">Persistent heatmap</a><a href="/v1/liquidity?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">Liquidity correlation</a><a href="/v1/open-interest?asset=BTC&amp;from={{.From}}&amp;to={{.To}}">Open interest</a></section>
<section class="card"><h2>Streaming</h2><a href="/v1/live?asset=BTC&amp;stream=trade,book_state">Live WebSocket endpoint</a><a href="/v1/replay?asset=BTC&amp;stream=trade&amp;from={{.From}}&amp;to={{.To}}&amp;speed=10">Historical WebSocket replay</a><p class="muted">WebSocket links require a compatible client.</p></section>
</div></main></body></html>`))

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	from, to, at := r.URL.Query().Get("from"), r.URL.Query().Get("to"), r.URL.Query().Get("at")
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		end := time.Now().UTC()
		if manifests, err := s.catalog(); err == nil && len(manifests) > 0 {
			latest := manifests[len(manifests)-1]
			if !latest.LastEvent.IsZero() {
				end = latest.LastEvent.UTC()
			}
		}
		if strings.TrimSpace(to) == "" {
			to = end.Format(time.RFC3339Nano)
		}
		if strings.TrimSpace(from) == "" {
			from = end.Add(-5 * time.Minute).Format(time.RFC3339Nano)
		}
	}
	if strings.TrimSpace(at) == "" {
		at = to
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	_ = dashboardTemplate.Execute(w, map[string]string{"Version": s.version, "Commit": s.commit, "From": from, "To": to, "At": at})
}
