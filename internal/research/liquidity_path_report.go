package research

import "fmt"

func PrintLiquidityPathAnalysis(analysis LiquidityPathAnalysis) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" LIQUIDITY PATH ANALYSIS")
	fmt.Println("========================================================")
	fmt.Println("Levels: POC, VWAP, VAH, VAL, session high/low, Fib618")
	fmt.Println("LVN/HVN: unavailable in the current volume-profile API")
	fmt.Println()
	printFeatureBucket("CLEAR PATH (0 LEVELS)", analysis.Clear)
	fmt.Println()
	printFeatureBucket("OBSTRUCTED PATH (1+ LEVELS)", analysis.Obstructed)
}
