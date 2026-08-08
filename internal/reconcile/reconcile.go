package reconcile

import (
	"fmt"

	lighter "github.com/ogtrading/overnight-strategy/internal/adapters/lighter"
)

type State string

const (
	Flat  State = "FLAT"
	Long  State = "LONG"
	Short State = "SHORT"
)

type Snapshot struct {
	State    State
	Position *lighter.Position
}

func Build(
	client *lighter.Client,
	accountIndex int64,
) (*Snapshot, error) {

	pos, err := client.GetPosition(
		accountIndex,
		"BTC",
	)

	if err != nil {
		return nil, err
	}

	state := Flat

	if pos.Side == "LONG" {
		state = Long
	}

	if pos.Side == "SHORT" {
		state = Short
	}

	return &Snapshot{
		State:    state,
		Position: pos,
	}, nil
}

func Print(s *Snapshot) {

	fmt.Println("RECONCILIATION")
	fmt.Println("================")
	fmt.Println("STATE:", s.State)
	fmt.Println("POSITION:", s.Position.Size)
	fmt.Println("ENTRY:", s.Position.EntryPrice)

}
