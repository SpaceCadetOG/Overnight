package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/auction"
)

func TestBuildLiquidityPathLongOrdersLevelsFromEntry(t *testing.T) {
	path := BuildLiquidityPath(auction.AuctionStructure{
		Entry: 100, Stop: 95, TP1: 120,
		POC: 110, VWAP: 105, VAH: 125, VAL: 90,
		OvernightHigh: 130, OvernightLow: 80, Fib618: 115,
	})

	if path.ClearPath || !path.ObstructedPath || path.ObstacleCount != 3 {
		t.Fatalf("unexpected path: %+v", path)
	}
	want := []string{"VWAP", "POC", "FIB618"}
	for i, name := range want {
		if path.Levels[i].Name != name {
			t.Fatalf("level %d = %s, want %s", i, path.Levels[i].Name, name)
		}
	}
	if path.FirstLevel == nil || path.FirstLevel.Name != "VWAP" ||
		path.NearestObstacle == nil || path.NearestObstacle.DistanceR != 1 {
		t.Fatalf("unexpected first/nearest level: %+v", path)
	}
}

func TestBuildLiquidityPathShortAndEndpointExclusion(t *testing.T) {
	path := BuildLiquidityPath(auction.AuctionStructure{
		Entry: 120, Stop: 125, TP1: 100,
		POC: 110, VWAP: 115, VAH: 130, VAL: 95,
		OvernightHigh: 140, OvernightLow: 90, Fib618: 100,
	})
	if path.ObstacleCount != 2 || path.Levels[0].Name != "VWAP" ||
		path.Levels[1].Name != "POC" {
		t.Fatalf("unexpected short path: %+v", path)
	}
}

func TestBuildLiquidityPathClearAndZeroRisk(t *testing.T) {
	path := BuildLiquidityPath(auction.AuctionStructure{
		Entry: 100, Stop: 100, TP1: 110,
		POC: 90, VWAP: 90, VAH: 120, VAL: 80,
		OvernightHigh: 130, OvernightLow: 70, Fib618: 110,
	})
	if !path.ClearPath || path.ObstructedPath || path.ObstacleCount != 0 {
		t.Fatalf("unexpected clear path: %+v", path)
	}
}

func TestAnalyzeLiquidityPathsUsesDualPopulationMetrics(t *testing.T) {
	observations := []AuctionObservation{
		{LiquidityPath: LiquidityPath{ClearPath: true}, Result: featureObservation(true, true, 2, 3, 0.5).Result},
		{LiquidityPath: LiquidityPath{ClearPath: true}, Result: featureObservation(true, false, 0, 0, 0).Result},
		{LiquidityPath: LiquidityPath{ObstructedPath: true}, Result: featureObservation(true, true, -1, 0.2, 1).Result},
	}
	analysis := AnalyzeLiquidityPaths(observations)
	if analysis.Clear.ValidPlans != 2 || analysis.Clear.Filled != 1 ||
		analysis.Clear.NoFill != 1 || analysis.Clear.Wins != 1 ||
		analysis.Clear.AverageRPerPlan != 1 || analysis.Clear.AverageRPerFilled != 2 {
		t.Fatalf("unexpected clear metrics: %+v", analysis.Clear)
	}
	if analysis.Obstructed.Losses != 1 || analysis.Obstructed.ProfitFactor != 0 {
		t.Fatalf("unexpected obstructed metrics: %+v", analysis.Obstructed)
	}
}
