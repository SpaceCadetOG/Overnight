package research

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

const liquidityV22MinimumSample = 20

type LiquidityV22Analysis struct {
	Sequences          map[liquidity.Sequence]FeatureBucket
	TargetAvailability map[string]FeatureBucket
	TP1Paths           map[string]FeatureBucket
	TP2Paths           map[string]FeatureBucket
	InternalTaken      map[string]FeatureBucket
	ExternalTarget     map[string]FeatureBucket
	InternalToExternal map[string]FeatureBucket
	Events             map[liquidity.Event]FeatureBucket
	ValueLocations     map[liquidity.ValueLocationState]FeatureBucket
	Combinations       map[string]FeatureBucket
}

func AnalyzeLiquidityV22(observations []AuctionObservation) LiquidityV22Analysis {
	a := LiquidityV22Analysis{
		Sequences: map[liquidity.Sequence]FeatureBucket{}, TargetAvailability: map[string]FeatureBucket{},
		TP1Paths: map[string]FeatureBucket{}, TP2Paths: map[string]FeatureBucket{}, InternalTaken: map[string]FeatureBucket{},
		ExternalTarget: map[string]FeatureBucket{}, InternalToExternal: map[string]FeatureBucket{}, Events: map[liquidity.Event]FeatureBucket{},
		ValueLocations: map[liquidity.ValueLocationState]FeatureBucket{}, Combinations: map[string]FeatureBucket{},
	}
	for _, observation := range observations {
		if !observation.Result.Filled {
			continue
		}
		context, result := observation.Liquidity, observation.Result
		accumulateMap(a.Sequences, context.Sequence, result)
		availability := string(context.TargetAvailability)
		accumulateMap(a.TargetAvailability, availability, result)
		tp1, tp2 := obstacleClass(context.TP1ObstacleCount), obstacleClass(context.TP2ObstacleCount)
		accumulateMap(a.TP1Paths, tp1, result)
		accumulateMap(a.TP2Paths, tp2, result)
		accumulateMap(a.InternalTaken, boolLabel(context.InternalTakenBeforeEntry, "TAKEN", "NOT_TAKEN"), result)
		accumulateMap(a.ExternalTarget, boolLabel(context.ExternalTarget, "EXTERNAL_TARGET", "INTERNAL_OR_NONE"), result)
		accumulateMap(a.InternalToExternal, boolLabel(context.InternalToExternal, "INTERNAL_TO_EXTERNAL", "OTHER"), result)
		accumulateMap(a.Events, context.Event, result)
		accumulateMap(a.ValueLocations, context.ValueLocation, result)
		key := strings.Join([]string{string(context.ValueTransition), availability, "TP1_" + tp1, string(context.Event), boolLabel(context.ExternalTarget, "EXTERNAL_TARGET", "INTERNAL_TARGET")}, " + ")
		accumulateMap(a.Combinations, key, result)
	}
	finalizeMap(a.Sequences)
	finalizeMap(a.TargetAvailability)
	finalizeMap(a.TP1Paths)
	finalizeMap(a.TP2Paths)
	finalizeMap(a.InternalTaken)
	finalizeMap(a.ExternalTarget)
	finalizeMap(a.InternalToExternal)
	finalizeMap(a.Events)
	finalizeMap(a.ValueLocations)
	finalizeMap(a.Combinations)
	return a
}

func accumulateMap[K comparable](buckets map[K]FeatureBucket, key K, result models.TradeResult) {
	bucket := buckets[key]
	accumulateFeatureBucket(&bucket, result)
	buckets[key] = bucket
}
func finalizeMap[K comparable](buckets map[K]FeatureBucket) {
	for key, bucket := range buckets {
		finalizeFeatureBucket(&bucket)
		buckets[key] = bucket
	}
}
func obstacleClass(count int) string {
	if count == 0 {
		return "CLEAR"
	}
	if count == 1 {
		return "1_LEVEL"
	}
	return "2+_LEVELS"
}
func boolLabel(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func PrintLiquidityV22Analysis(a LiquidityV22Analysis) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" LIQUIDITY V2.2 FULL-HISTORY CONDITION MATRIX")
	fmt.Println("========================================================")
	printV22Map("LIQUIDITY SEQUENCE", a.Sequences)
	printV22Map("TARGET AVAILABILITY", a.TargetAvailability)
	printV22Map("PATH ENTRY TO TP1", a.TP1Paths)
	printV22Map("PATH ENTRY TO TP2", a.TP2Paths)
	printV22Map("INTERNAL LIQUIDITY TAKEN", a.InternalTaken)
	printV22Map("EXTERNAL LIQUIDITY TARGET", a.ExternalTarget)
	printV22Map("INTERNAL TO EXTERNAL", a.InternalToExternal)
	printV22Map("SWEEP / GRAB / RUN", a.Events)
	printV22Map("VALUE LOCATION", a.ValueLocations)
	printRankedCombinations(a.Combinations)
}

func printV22Map[K ~string](title string, buckets map[K]FeatureBucket) {
	fmt.Println()
	fmt.Println(title)
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, key := range keys {
		printLiquidityV21Bucket(key, buckets[K(key)])
	}
}

type rankedCondition struct {
	key    string
	bucket FeatureBucket
}

func printRankedCombinations(buckets map[string]FeatureBucket) {
	rows := make([]rankedCondition, 0)
	for key, bucket := range buckets {
		if bucket.Filled >= liquidityV22MinimumSample {
			rows = append(rows, rankedCondition{key, bucket})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].bucket.AverageRPerFilled == rows[j].bucket.AverageRPerFilled {
			return rows[i].bucket.ProfitFactor > rows[j].bucket.ProfitFactor
		}
		return rows[i].bucket.AverageRPerFilled > rows[j].bucket.AverageRPerFilled
	})
	fmt.Println()
	fmt.Printf("BEST CONDITIONS (minimum %d trades)\n", liquidityV22MinimumSample)
	for i := 0; i < len(rows) && i < 5; i++ {
		printLiquidityV21Bucket(rows[i].key, rows[i].bucket)
	}
	fmt.Println()
	fmt.Printf("WORST CONDITIONS (minimum %d trades)\n", liquidityV22MinimumSample)
	for i, shown := len(rows)-1, 0; i >= 0 && shown < 5; i, shown = i-1, shown+1 {
		printLiquidityV21Bucket(rows[i].key, rows[i].bucket)
	}
}
