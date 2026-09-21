package collector

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/book"
	"github.com/ogtrading/overnight-strategy/internal/oracle/live"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
	oraclelighter "github.com/ogtrading/overnight-strategy/internal/oracle/normalize/lighter"
)

type oraclePipeline struct {
	store          interface{ Append(string, any) error }
	eventStore     interface{ Append(string, any) error }
	hub            *live.Hub
	books          map[string]*book.State
	sequences      map[string]uint64
	lastCheckpoint map[string]time.Time
	levels         map[string]map[string]map[string]string
	pendingChanges map[string][]liquidityChange
	changeWindow   map[string]time.Time
}

type liquidityChange struct {
	Timestamp      time.Time `json:"timestamp"`
	Side           string    `json:"side"`
	Price          string    `json:"price"`
	PreviousSize   string    `json:"previous_size"`
	NewSize        string    `json:"new_size"`
	SizeDelta      string    `json:"size_delta"`
	Action         string    `json:"action"`
	VenueSequence  int64     `json:"venue_sequence"`
	OracleSequence uint64    `json:"oracle_sequence"`
}

// MarshalJSON uses a versioned tuple to keep the continuously written hot
// dataset small. Order: time_ms, side, price, previous_size, new_size,
// size_delta, action, venue_sequence, oracle_sequence.
func (c liquidityChange) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{c.Timestamp.UnixMilli(), c.Side, c.Price, c.PreviousSize, c.NewSize, c.SizeDelta, c.Action, c.VenueSequence, c.OracleSequence})
}

type liquidityObservationBatch struct {
	SchemaVersion string            `json:"schema_version"`
	Asset         string            `json:"asset"`
	WindowStart   time.Time         `json:"window_start"`
	WindowEnd     time.Time         `json:"window_end"`
	ConnectionID  string            `json:"connection_id"`
	BookQuality   model.Quality     `json:"book_quality"`
	Observation   string            `json:"observation_type"`
	Changes       []liquidityChange `json:"changes"`
}

func newOraclePipeline(output interface{ Append(string, any) error }) *oraclePipeline {
	return newOraclePipelineWithEventStore(output, output)

}

func newOraclePipelineWithEventStore(_ interface{ Append(string, any) error }, eventOutput interface{ Append(string, any) error }) *oraclePipeline {
	return &oraclePipeline{store: eventOutput, eventStore: eventOutput, hub: live.New(), books: map[string]*book.State{}, sequences: map[string]uint64{}, lastCheckpoint: map[string]time.Time{}, levels: map[string]map[string]map[string]string{}, pendingChanges: map[string][]liquidityChange{}, changeWindow: map[string]time.Time{}}
}

func (p *oraclePipeline) resetBooks() {
	for _, state := range p.books {
		state.Invalidate("collector connection reset")
	}
	p.books = map[string]*book.State{}
	p.levels = map[string]map[string]map[string]string{}
	p.pendingChanges = map[string][]liquidityChange{}
	p.changeWindow = map[string]time.Time{}
	p.hub.ResetBooks()
}

func (p *oraclePipeline) accept(raw []byte, asset string, received time.Time, connectionID string) (int, int, error) {
	if asset == "" {
		return 0, 0, nil
	}
	if connectionID == "" {
		connectionID = "conn_unassigned"
	}
	events, err := oraclelighter.Normalize(raw, oraclelighter.Context{Asset: asset, ReceivedAt: received, ConnectionID: connectionID, FirstSequence: 1, DefaultQuality: model.QualityCertified})
	if err != nil {
		// Empty trade updates are valid transport messages but contain no event.
		if err.Error() == "trade message contained no events" {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	checkpoints := 0
	for i := range events {
		key := asset + ":" + string(events[i].Stream)
		if events[i].Stream == model.StreamBookSnapshot || events[i].Stream == model.StreamBookDelta {
			key = asset + ":book"
		}
		p.sequences[key]++
		events[i].OracleSequence = p.sequences[key]
		events[i].EventID = events[i].DeterministicID()
		if events[i].Stream == model.StreamBookSnapshot || events[i].Stream == model.StreamBookDelta {
			var value model.Book
			if err := json.Unmarshal(events[i].Payload, &value); err != nil {
				return i, checkpoints, fmt.Errorf("decode normalized book: %w", err)
			}
			state := p.books[asset]
			if state == nil {
				state = book.New(asset)
				p.books[asset] = state
			}
			if events[i].Stream == model.StreamBookSnapshot {
				err = state.ApplySnapshot(value, events[i].OracleSequence)
			} else {
				err = state.ApplyDelta(value, events[i].OracleSequence)
			}
			if err != nil {
				return i, checkpoints, err
			}
			if events[i].Stream == model.StreamBookSnapshot {
				p.replaceLevels(asset, value)
			} else {
				p.observeDelta(asset, value, events[i], received)
				if err := p.flushObservations(asset, received, connectionID, false); err != nil {
					return i, checkpoints, err
				}
			}
			now := received.UTC()
			snapshot, snapshotErr := state.Snapshot(0, now, connectionID)
			if snapshotErr != nil {
				return i, checkpoints, snapshotErr
			}
			p.hub.SetBook(snapshot)
			if events[i].Stream == model.StreamBookSnapshot || now.Sub(p.lastCheckpoint[asset]) >= time.Minute {
				if err := p.store.Append("asset="+asset+"/oracle_book_checkpoints", snapshot); err != nil {
					return i, checkpoints, err
				}
				p.lastCheckpoint[asset] = now
				checkpoints++
			}
		}
		p.hub.Publish(events[i])
		if err := p.eventStore.Append("asset="+asset+"/oracle_events", events[i]); err != nil {
			return i, checkpoints, err
		}
	}
	return len(events), checkpoints, nil
}

func (p *oraclePipeline) replaceLevels(asset string, value model.Book) {
	p.levels[asset] = map[string]map[string]string{"BID": {}, "ASK": {}}
	for _, level := range value.Bids {
		if level.Size != "0" {
			p.levels[asset]["BID"][level.Price] = level.Size
		}
	}
	for _, level := range value.Asks {
		if level.Size != "0" {
			p.levels[asset]["ASK"][level.Price] = level.Size
		}
	}
	p.pendingChanges[asset] = nil
	p.changeWindow[asset] = time.Time{}
}

func (p *oraclePipeline) observeDelta(asset string, value model.Book, event model.Envelope, received time.Time) {
	if p.levels[asset] == nil {
		p.levels[asset] = map[string]map[string]string{"BID": {}, "ASK": {}}
	}
	if p.changeWindow[asset].IsZero() {
		p.changeWindow[asset] = received.UTC()
	}
	for _, side := range []struct {
		name   string
		levels []model.Level
	}{{"BID", value.Bids}, {"ASK", value.Asks}} {
		for _, level := range side.levels {
			previous := p.levels[asset][side.name][level.Price]
			if previous == "" {
				previous = "0"
			}
			if previous == level.Size {
				continue
			}
			action := "INCREASED"
			oldValue, newValue := decimalRat(previous), decimalRat(level.Size)
			switch {
			case newValue.Sign() == 0:
				action = "REMOVED"
			case oldValue.Sign() == 0:
				action = "ADDED"
			case newValue.Cmp(oldValue) < 0:
				action = "DECREASED"
			}
			delta := new(big.Rat).Sub(newValue, oldValue)
			p.pendingChanges[asset] = append(p.pendingChanges[asset], liquidityChange{Timestamp: received.UTC(), Side: side.name, Price: level.Price, PreviousSize: previous, NewSize: level.Size, SizeDelta: decimalString(delta), Action: action, VenueSequence: event.VenueSequence, OracleSequence: event.OracleSequence})
			if newValue.Sign() == 0 {
				delete(p.levels[asset][side.name], level.Price)
			} else {
				p.levels[asset][side.name][level.Price] = level.Size
			}
		}
	}
}

func (p *oraclePipeline) flushObservations(asset string, at time.Time, connectionID string, force bool) error {
	changes := p.pendingChanges[asset]
	if len(changes) == 0 || (!force && at.Sub(p.changeWindow[asset]) < time.Second) {
		return nil
	}
	batch := liquidityObservationBatch{SchemaVersion: "oracle-liquidity-observation-v1", Asset: asset, WindowStart: p.changeWindow[asset], WindowEnd: at.UTC(), ConnectionID: connectionID, BookQuality: model.QualityCertified, Observation: "DISPLAYED_BOOK_CHANGE", Changes: append([]liquidityChange(nil), changes...)}
	if err := p.store.Append("asset="+asset+"/liquidity_observations", batch); err != nil {
		return err
	}
	p.pendingChanges[asset] = nil
	p.changeWindow[asset] = at.UTC()
	return nil
}

func decimalRat(value string) *big.Rat {
	result := new(big.Rat)
	if _, ok := result.SetString(value); !ok {
		return new(big.Rat)
	}
	return result
}

func decimalString(value *big.Rat) string {
	raw := strings.TrimRight(strings.TrimRight(value.FloatString(18), "0"), ".")
	if raw == "" || raw == "-0" {
		return "0"
	}
	return raw
}
