package forensics

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type OpportunityState string

const (
	Planned OpportunityState = "PLANNED"
	Invalid OpportunityState = "INVALIDATED"
	Waiting OpportunityState = "WAITING_FOR_FILL"
	Filled  OpportunityState = "FILLED"
	Closed  OpportunityState = "CLOSED"
	NoFill  OpportunityState = "NO_FILL"
)

type Case struct {
	CaseID, SessionID, OpportunityID, Symbol, StrategyVersion string
	State                                                     OpportunityState
	Outcome                                                   string
	FirstEvent, LastEvent                                     time.Time
	EventCount                                                int
	Sequences                                                 []uint64
	DataQuality                                               []string
}

func BuildCase(events []Envelope) (Case, error) {
	if len(events) == 0 {
		return Case{}, fmt.Errorf("case requires events")
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Sequence == events[j].Sequence {
			return events[i].OccurredAt.Before(events[j].OccurredAt)
		}
		return events[i].Sequence < events[j].Sequence
	})
	first := events[0]
	c := Case{CaseID: first.CaseID, SessionID: first.SessionID, OpportunityID: first.OpportunityID, Symbol: first.Symbol, StrategyVersion: first.StrategyVersion, FirstEvent: first.OccurredAt}
	seen := map[uint64]bool{}
	for _, event := range events {
		if event.OpportunityID != c.OpportunityID {
			c.DataQuality = append(c.DataQuality, "MIXED_OPPORTUNITY_IDS")
		}
		if seen[event.Sequence] {
			c.DataQuality = append(c.DataQuality, "DUPLICATE_SEQUENCE")
		}
		seen[event.Sequence] = true
		c.Sequences = append(c.Sequences, event.Sequence)
		c.LastEvent = event.OccurredAt
		c.EventCount++
		switch event.EventType {
		case PlanCreated:
			c.State = Planned
		case PlanInvalidated:
			c.State = Invalid
		case OrderSubmitted:
			c.State = Waiting
		case OrderFilled, PositionOpened:
			c.State = Filled
		case EntryExpired:
			c.State = NoFill
			c.Outcome = "NO_FILL"
		case PositionClosed:
			c.State = Closed
			var p struct {
				Outcome string `json:"outcome"`
			}
			_ = json.Unmarshal(event.Payload, &p)
			c.Outcome = p.Outcome
		}
	}
	for i := 1; i < len(c.Sequences); i++ {
		if c.Sequences[i] != c.Sequences[i-1]+1 {
			c.DataQuality = append(c.DataQuality, "SEQUENCE_GAP")
			break
		}
	}
	return c, nil
}
