package research

import (
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/auction"
	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

// AuctionObservation pairs a completed trade result with the market
// structure that existed before entry.
//
// TradeResult is intentionally left unchanged. Keeping this research-only
// wrapper avoids coupling the core execution model to the auction package
// and prevents a package import cycle.
type AuctionObservation struct {
	Date          time.Time
	Result        models.TradeResult
	Structure     auction.AuctionStructure
	LiquidityPath LiquidityPath
	Liquidity     liquidity.Context
	BuildError    string
}

// PrintAuctionObservation displays one completed trade beside its pre-entry
// auction structure.
func PrintAuctionObservation(observation AuctionObservation) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Printf(
		"TRADE DATE: %s\n",
		observation.Date.Format("2006-01-02"),
	)
	fmt.Printf("Direction:  %s\n", observation.Result.Direction)
	fmt.Printf("Outcome:    %s\n", observation.Result.Outcome)
	fmt.Printf("Result:     %.2fR\n", observation.Result.RealizedR)
	fmt.Printf("Fill:       %.2f\n", observation.Result.FillPrice)
	fmt.Printf("Exit:       %.2f\n", observation.Result.ExitPrice)

	if observation.BuildError != "" {
		fmt.Println()
		fmt.Println("AUCTION STRUCTURE")
		fmt.Println("--------------------------------------------------------")
		fmt.Printf(
			"Unable to build auction structure: %s\n",
			observation.BuildError,
		)
		fmt.Println("========================================================")
		return
	}

	fmt.Println()
	fmt.Print(auction.FormatStructure(observation.Structure))
	printLiquidityPath(observation.LiquidityPath)
	printStructuralLiquidity(observation.Liquidity)
	fmt.Println("========================================================")
}

func printStructuralLiquidity(context liquidity.Context) {
	fmt.Println()
	fmt.Println("STRUCTURAL LIQUIDITY")
	fmt.Println("--------------------------------------------------------")
	fmt.Printf("Sequence:             %s\n", context.Sequence)
	fmt.Printf("Buy-side taken:       %t\n", context.BuySideTaken)
	fmt.Printf("Sell-side taken:      %t\n", context.SellSideTaken)
	fmt.Printf("Opposing present:     %t\n", context.OpposingPresent)
	fmt.Printf("Directional target:  %t\n", context.DirectionalTarget)
	fmt.Printf("Path score:           %d/10\n", context.PathScore)
	fmt.Printf("Detected levels:      %d\n", len(context.Levels))
}

func printLiquidityPath(path LiquidityPath) {
	fmt.Println()
	fmt.Println("LIQUIDITY PATH")
	fmt.Println("--------------------------------------------------------")
	fmt.Printf("Obstacle count:       %d\n", path.ObstacleCount)
	fmt.Printf("Clear path:           %t\n", path.ClearPath)
	fmt.Printf("Obstructed path:      %t\n", path.ObstructedPath)

	if path.FirstLevel == nil {
		fmt.Println("First level:          NONE")
		fmt.Println("Nearest obstacle:     NONE")
		fmt.Println("Encountered levels:   NONE")
		return
	}

	fmt.Printf(
		"First level:          %s @ %.2f (%.3fR)\n",
		path.FirstLevel.Name,
		path.FirstLevel.Price,
		path.FirstLevel.DistanceR,
	)
	fmt.Printf(
		"Nearest obstacle:     %s @ %.2f (%.3fR)\n",
		path.NearestObstacle.Name,
		path.NearestObstacle.Price,
		path.NearestObstacle.DistanceR,
	)
	fmt.Println("Encountered levels:")
	for index, level := range path.Levels {
		fmt.Printf(
			"  %d. %-12s %.2f | distance %.2f | %.3fR\n",
			index+1,
			level.Name,
			level.Price,
			level.DistancePrice,
			level.DistanceR,
		)
	}
}
