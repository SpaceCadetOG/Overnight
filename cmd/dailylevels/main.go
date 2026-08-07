package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/live"
	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

const dateLayout = "2006-01-02"

func main() {
	dateFlag := flag.String(
		"date",
		"today",
		"session date: YYYY-MM-DD, today, or yesterday",
	)
	symbolFlag := flag.String(
		"symbol",
		"",
		"optional single symbol such as BTC",
	)
	jsonFlag := flag.Bool(
		"json",
		false,
		"print structured JSON instead of the human-readable report",
	)
	allFlag := flag.Bool(
		"all",
		false,
		"include LINK, HYPE, XAU, and XAG research maps",
	)

	flag.Parse()

	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal(err)
	}

	now := time.Now().In(location)

	sessionDate, isToday, err := resolveDate(
		*dateFlag,
		now,
		location,
	)
	if err != nil {
		fatal(err)
	}

	sessionEnd := time.Date(
		sessionDate.Year(),
		sessionDate.Month(),
		sessionDate.Day(),
		5,
		0,
		0,
		0,
		location,
	)

	// We use the completed 05:00 five-minute candle.
	// A 05:00 candle spans 05:00:00-05:04:59 and is complete
	// at 05:05 CT.
	readyAt := sessionEnd.Add(5 * time.Minute)

	if isToday && now.Before(readyAt) {
		fmt.Println("============================================================")
		fmt.Println("OVERNIGHT STRATEGY DAILY LEVELS")
		fmt.Println("============================================================")
		fmt.Printf("Session date: %s\n", sessionDate.Format(dateLayout))
		fmt.Printf(
			"Current time: %s\n",
			now.Format("2006-01-02 15:04:05 MST"),
		)
		fmt.Println()
		fmt.Printf(
			"Active session: %s 19:00 CT -> %s 05:00 CT\n",
			sessionDate.AddDate(0, 0, -1).Format(dateLayout),
			sessionDate.Format(dateLayout),
		)
		fmt.Printf(
			"Ready after:    %s\n",
			readyAt.Format("2006-01-02 15:04:05 MST"),
		)
		fmt.Println()
		fmt.Println("STATUS: 05:00 CT candle not complete yet.")
		fmt.Println("No partial market maps or trade levels were generated.")
		fmt.Println("Run again after the 05:00 five-minute candle closes at 05:05 CT.")
		return
	}

	assets, err := selectedAssets(
		strings.ToUpper(strings.TrimSpace(*symbolFlag)),
		*allFlag,
	)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Minute,
	)
	defer cancel()

	client := lighterdata.New(
		os.Getenv("LIGHTER_BASE_URL"),
		nil,
	)

	markets, err := client.MarketMap(ctx)
	if err != nil {
		fatal(err)
	}

	// We need the entire previous calendar day for previous-day
	// OHLC/VWAP/profile values, plus the overnight session ending at 05:00.
	start := time.Date(
		sessionDate.Year(),
		sessionDate.Month(),
		sessionDate.Day(),
		0,
		0,
		0,
		0,
		location,
	).AddDate(0, 0, -1)

	end := readyAt

	snapshots := make([]live.MarketSnapshot, 0, len(assets))

	for _, asset := range assets {
		market, ok := markets[asset.Symbol]
		if !ok {
			fatal(fmt.Errorf(
				"Lighter market metadata missing for %s",
				asset.Symbol,
			))
		}

		candles, err := client.Candles(
			ctx,
			market.MarketID,
			"5m",
			start.UTC(),
			end.UTC(),
		)
		if err != nil {
			fatal(fmt.Errorf(
				"%s candle download failed: %w",
				asset.Symbol,
				err,
			))
		}

		snapshot, err := live.BuildMarketSnapshotForDate(
			asset.Symbol,
			candles,
			location,
			sessionDate,
		)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", asset.Symbol, err))
		}

		snapshots = append(snapshots, snapshot)
	}

	if *jsonFlag {
		printJSON(sessionDate, now, snapshots)
		return
	}

	printReport(
		sessionDate,
		now,
		location,
		snapshots,
	)
}

func selectedAssets(
	symbol string,
	includeAll bool,
) ([]universe.Asset, error) {
	// Default DAILY DISPLAY basket.
	//
	// The first five remain the only LIVE_EXECUTION assets.
	// HYPE and LIT are displayed for research/observation only.
	displaySymbols := []string{
		"BTC",
		"ETH",
		"ZEC",
		"BNB",
		"SOL",
		"HYPE",
		"LIT",
		"XAU",
		"XAG",
	}

	if includeAll {
		return universe.All(), nil
	}

	if symbol != "" {
		asset, ok := universe.Find(symbol)
		if !ok {
			return nil, fmt.Errorf("unknown symbol %s", symbol)
		}

		return []universe.Asset{asset}, nil
	}

	assets := make([]universe.Asset, 0, len(displaySymbols))

	for _, wanted := range displaySymbols {
		asset, ok := universe.Find(wanted)
		if !ok {
			return nil, fmt.Errorf(
				"%s is not registered in the asset universe",
				wanted,
			)
		}

		assets = append(assets, asset)
	}

	return assets, nil
}

func resolveDate(
	value string,
	now time.Time,
	location *time.Location,
) (time.Time, bool, error) {
	value = strings.ToLower(strings.TrimSpace(value))

	calendarToday := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0,
		0,
		0,
		0,
		location,
	)

	// Session dates are named by the morning on which the
	// 19:00-05:00 CT overnight session completes.
	//
	// Example:
	//
	// Aug 6 18:00 -> active/completed session date = Aug 6
	// Aug 6 19:00 -> new active session date       = Aug 7
	//
	// Therefore after 19:00 CT the default "today" session
	// is the following calendar date.
	activeSessionDate := calendarToday
	if now.Hour() >= 19 {
		activeSessionDate = calendarToday.AddDate(0, 0, 1)
	}

	switch value {
	case "", "today":
		return activeSessionDate, true, nil

	case "yesterday":
		return activeSessionDate.AddDate(0, 0, -1), false, nil

	default:
		parsed, err := time.ParseInLocation(
			dateLayout,
			value,
			location,
		)
		if err != nil {
			return time.Time{}, false, fmt.Errorf(
				"invalid -date %q; use YYYY-MM-DD, today, or yesterday",
				value,
			)
		}

		// Explicit dates may reference today's calendar date or
		// historical completed sessions. Do not allow a session
		// beyond the currently active overnight session.
		if parsed.After(activeSessionDate) {
			return time.Time{}, false, fmt.Errorf(
				"session date %s is in the future",
				parsed.Format(dateLayout),
			)
		}

		return parsed, parsed.Equal(activeSessionDate), nil
	}
}

func printReport(
	sessionDate time.Time,
	generatedAt time.Time,
	location *time.Location,
	snapshots []live.MarketSnapshot,
) {
	fmt.Println("============================================================")
	fmt.Println("OVERNIGHT STRATEGY DAILY LEVELS")
	fmt.Println("============================================================")
	fmt.Printf("Session date: %s\n", sessionDate.Format(dateLayout))
	fmt.Println("Session:      19:00-05:00 America/Chicago")
	fmt.Printf(
		"Generated:    %s\n",
		generatedAt.Format("2006-01-02 15:04:05 MST"),
	)
	fmt.Printf("Assets:       %d\n", len(snapshots))
	fmt.Println("Status:       COMPLETE")

	for _, snapshot := range snapshots {
		fmt.Println()
		fmt.Println("============================================================")
		fmt.Printf(
			"%s | %s | ORDER AUTHORIZED: %t\n",
			snapshot.Symbol,
			snapshot.Classification,
			snapshot.OrderAuthorized,
		)
		fmt.Println("============================================================")

		fmt.Println("OVERNIGHT SESSION")
		printValue("Overnight high", snapshot.OvernightHigh)
		printValue("Overnight low", snapshot.OvernightLow)
		printValue("Overnight range", snapshot.OvernightRange)
		printValue("Overnight midpoint", snapshot.OvernightMidpoint)
		printValue("Session close", snapshot.SessionClose)

		fmt.Println()
		fmt.Println("FIBONACCI LEVELS")
		printValue("Fib 0.382", snapshot.Fib382)
		printValue("Fib 0.500", snapshot.Fib500)
		printValue("Fib 0.618", snapshot.Fib618)

		fmt.Println()
		fmt.Println("OVERNIGHT VOLUME PROFILE")
		printValue("VWAP", snapshot.VWAP)
		printValue("POC", snapshot.POC)
		printValue("VAH", snapshot.VAH)
		printValue("VAL", snapshot.VAL)

		fmt.Println()
		fmt.Println("PREVIOUS DAY LEVELS")
		printValue("Previous open", snapshot.PreviousDay.Open)
		printValue("Previous high", snapshot.PreviousDay.High)
		printValue("Previous low", snapshot.PreviousDay.Low)
		printValue("Previous close", snapshot.PreviousDay.Close)
		printValue("Previous VWAP", snapshot.PreviousDay.VWAP)
		printValue("Previous POC", snapshot.PreviousDay.POC)
		printValue("Previous VAH", snapshot.PreviousDay.VAH)
		printValue("Previous VAL", snapshot.PreviousDay.VAL)

		fmt.Println()
		fmt.Println("FROZEN BASELINE PLAN")

		if snapshot.Plan == nil {
			fmt.Println(
				"No executable order plan: research or observe-only asset.",
			)
			continue
		}

		plan := snapshot.Plan

		fmt.Printf("%-24s %s\n", "Direction", plan.Direction)
		fmt.Printf(
			"%-24s %s (%s)\n",
			"Entry",
			price(plan.Entry),
			plan.EntrySource,
		)
		fmt.Printf(
			"%-24s %s (%s)\n",
			"Stop",
			price(plan.Stop),
			plan.StopSource,
		)
		fmt.Printf(
			"%-24s %s (%s)\n",
			"TP1",
			price(plan.TP1),
			plan.TP1Source,
		)
		printValue("TP2", plan.TP2)
		printValue("Risk distance", plan.RiskDistance)
		printValue("Reward 1 distance", plan.Reward1Distance)
		printValue("Reward 2 distance", plan.Reward2Distance)
		fmt.Printf("%-24s %.3fR\n", "RR1", plan.RR1)
		fmt.Printf("%-24s %.3fR\n", "RR2", plan.RR2)
		fmt.Printf("%-24s %t\n", "Valid", plan.Valid)

		if plan.InvalidReason != "" {
			fmt.Printf(
				"%-24s %s\n",
				"Invalid reason",
				plan.InvalidReason,
			)
		}
	}
}

func printLiquidity(
	levels []liquidity.Level,
	location *time.Location,
) {
	if len(levels) == 0 {
		fmt.Println("No liquidity levels detected.")
		return
	}

	grouped := make(map[liquidity.Kind][]liquidity.Level)

	for _, level := range levels {
		grouped[level.Kind] = append(
			grouped[level.Kind],
			level,
		)
	}

	kinds := []liquidity.Kind{
		liquidity.SessionHigh,
		liquidity.SessionLow,
		liquidity.SwingHigh,
		liquidity.SwingLow,
		liquidity.EqualHigh,
		liquidity.EqualLow,
	}

	for _, kind := range kinds {
		rows := grouped[kind]

		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Price < rows[j].Price
		})

		if len(rows) == 0 {
			fmt.Printf("%-24s none\n", kind)
			continue
		}

		for _, level := range rows {
			fmt.Printf(
				"%-24s %s | touches=%d | taken=%t | formed=%s\n",
				kind,
				price(level.Price),
				level.Touches,
				level.Taken,
				level.FormedAt.In(location).Format("15:04"),
			)
		}
	}
}

func printValue(label string, value float64) {
	fmt.Printf("%-24s %s\n", label, price(value))
}

func printJSON(
	sessionDate time.Time,
	generatedAt time.Time,
	snapshots []live.MarketSnapshot,
) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(map[string]any{
		"timezone":     "America/Chicago",
		"session_date": sessionDate.Format(dateLayout),
		"generated_at": generatedAt,
		"snapshots":    snapshots,
	})
	if err != nil {
		fatal(err)
	}
}

func price(value float64) string {
	return fmt.Sprintf("%.8f", value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
