package research

import (
	"math"
	"sort"

	"github.com/ogtrading/overnight-strategy/internal/auction"
)

const liquidityPathTolerance = 1e-9

type LiquidityLevel struct {
	Name          string
	Price         float64
	DistanceR     float64
	DistancePrice float64
}

type LiquidityPath struct {
	Levels          []LiquidityLevel
	ObstacleCount   int
	NearestObstacle *LiquidityLevel
	FirstLevel      *LiquidityLevel
	ClearPath       bool
	ObstructedPath  bool
}

// BuildLiquidityPath identifies structural levels strictly between Entry and
// TP1. Levels are ordered from Entry toward TP1 and distances are normalized
// by absolute planned trade risk. Equal-price levels retain their individual
// identities because they represent distinct structural references.
func BuildLiquidityPath(structure auction.AuctionStructure) LiquidityPath {
	risk := math.Abs(structure.Entry - structure.Stop)

	candidates := []struct {
		name  string
		price float64
	}{
		{"POC", structure.POC},
		{"VWAP", structure.VWAP},
		{"VAH", structure.VAH},
		{"VAL", structure.VAL},
		{"SESSION_HIGH", structure.OvernightHigh},
		{"SESSION_LOW", structure.OvernightLow},
		{"FIB618", structure.Fib618},
	}

	levels := make([]LiquidityLevel, 0, len(candidates))
	low := math.Min(structure.Entry, structure.TP1)
	high := math.Max(structure.Entry, structure.TP1)

	for _, candidate := range candidates {
		if !isFinite(candidate.price) ||
			candidate.price <= low+liquidityPathTolerance ||
			candidate.price >= high-liquidityPathTolerance {
			continue
		}

		distance := math.Abs(candidate.price - structure.Entry)
		level := LiquidityLevel{
			Name:          candidate.name,
			Price:         candidate.price,
			DistancePrice: distance,
		}
		if risk > liquidityPathTolerance {
			level.DistanceR = distance / risk
		}
		levels = append(levels, level)
	}

	sort.SliceStable(levels, func(i, j int) bool {
		if math.Abs(levels[i].DistancePrice-levels[j].DistancePrice) <= liquidityPathTolerance {
			return levels[i].Name < levels[j].Name
		}
		return levels[i].DistancePrice < levels[j].DistancePrice
	})

	path := LiquidityPath{
		Levels:         levels,
		ObstacleCount:  len(levels),
		ClearPath:      len(levels) == 0,
		ObstructedPath: len(levels) > 0,
	}
	if len(levels) > 0 {
		first := levels[0]
		nearest := levels[0]
		path.FirstLevel = &first
		path.NearestObstacle = &nearest
	}

	return path
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type LiquidityPathAnalysis struct {
	Clear      FeatureBucket
	Obstructed FeatureBucket
}

func AnalyzeLiquidityPaths(observations []AuctionObservation) LiquidityPathAnalysis {
	analysis := LiquidityPathAnalysis{}
	for _, observation := range observations {
		bucket := &analysis.Obstructed
		if observation.LiquidityPath.ClearPath {
			bucket = &analysis.Clear
		}
		accumulateFeatureBucket(bucket, observation.Result)
	}
	finalizeFeatureBucket(&analysis.Clear)
	finalizeFeatureBucket(&analysis.Obstructed)
	return analysis
}
