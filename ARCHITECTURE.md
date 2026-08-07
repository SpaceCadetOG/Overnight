# Overnight Strategy Architecture

## Operating principle

The production system runs five independent instances of the same frozen
baseline strategy:

- BTC
- ETH
- ZEC
- BNB
- SOL

The basket is a combined research view, not an asset-selection signal. Each
asset produces its own market map, trade plan, order, lifecycle, and result.
There is no rotation, ranking, shared signal, or adaptive allocation in the
baseline.

The market-intelligence system also observes four non-trading markets:

- LINK
- HYPE
- XAU
- XAG

These instruments receive the same daily market-map calculations and research
records, but they can never create an order. Observation does not imply
eligibility for capital.

## Asset universes

| Symbol | Asset class | Market maps | Order authority |
|---|---|---:|---:|
| BTC | Crypto | Yes | Yes |
| ETH | Crypto | Yes | Yes |
| ZEC | Crypto | Yes | Yes |
| BNB | Crypto | Yes | Yes |
| SOL | Crypto | Yes | Yes |
| LINK | Crypto | Yes | No — research |
| HYPE | Crypto | Yes | No — research |
| XAU | Metals | Yes | No — valid research market |
| XAG | Metals | Yes | No — observe only |

The trading universe and research universe must be represented explicitly in
configuration and persisted metadata. Research-only status is a hard execution
constraint, not a scanner preference.

## Daily boundary

The overnight session contains the 120 five-minute candles from 19:00 through
04:55 America/Chicago. The 05:00 candle belongs to the execution period. At the
05:00 boundary, the system freezes the overnight auction and builds a separate
map for every production asset.

```text
                         Overnight engine
                                |
          +----------+----------+----------+----------+
          |          |          |          |          |
         BTC        ETH        ZEC        BNB        SOL
          |          |          |          |          |
        Map +      Map +      Map +      Map +      Map +
        plan       plan       plan       plan       plan
          |          |          |          |          |
        Order      Order      Order      Order      Order
          +----------+----------+----------+----------+
                                |
                         Lighter executor
                                |
                             Exchange
```

The daily output is five independent market maps, five independent trade plans,
up to five resting orders, four additional observation-only market maps, and one
shared research database.

```text
                              05:00 CT
                                  |
                    +-------------+-------------+
                    |                           |
            Live trading maps              Research maps
                    |                           |
          BTC ETH ZEC BNB SOL           LINK HYPE XAU XAG
                    |                           |
          Plans and possible orders          Data only
                    +-------------+-------------+
                                  |
                         Research database
                                  |
                      Weekly/monthly analysis
```

## Per-asset 05:00 market map

### Overnight range

Record:

- Overnight high and low
- Range size and midpoint
- Session close and VWAP

The session close relative to VWAP determines the frozen baseline direction.

### Fibonacci structure

Build the 0.382, 0.500, and 0.618 levels from the overnight high and low. The
frozen baseline uses:

- Entry: midpoint of Fib 0.382 and Fib 0.500
- TP1: Fib 0.618
- TP2: overnight high for longs and overnight low for shorts

### Volume profile

Record the complete overnight distribution, including:

- POC
- VAH
- VAL
- Value area
- Volume distribution

The frozen baseline already has one validated profile dependency: its
`PROFILE_FIB` stop uses `min(VAL, Fib382)` for longs and
`max(VAH, Fib382)` for shorts, followed by the configured stop buffer. That
behavior must not be removed or expanded without a controlled A/B test. POC and
all other profile relationships remain research metadata unless explicitly
validated and promoted.

### Previous-day map

Record independently for every asset:

- Previous-day high, low, open, and close
- Previous-day VWAP
- Previous-day POC, VAH, and VAL

These levels are research context and do not modify baseline orders.

### Structural liquidity map

Record internal liquidity:

- Minor highs and lows
- Equal highs and lows

Record external liquidity:

- Overnight high and low
- Current and previous daily high and low
- Session highs and lows

Liquidity is descriptive research metadata. It cannot filter a plan, change its
direction, move an entry or stop, replace a target, or create a re-entry.

### Lighter market context

Record market and execution context where available:

- Order-book snapshots and events
- Spread, depth, and imbalance
- Trades and trade flow
- Funding and open interest
- Order latency, fills, slippage, and cancellations

This layer explains execution and outcomes. It has no production authority.

## Per-asset order lifecycle

Every valid daily plan can create its own resting order. A filled order is
managed and recorded independently. An order that never reaches its entry is
canceled at expiry and retained as a no-fill observation with miss distance and
subsequent market behavior.

One asset's fill, no-fill, position, or result must not suppress or alter another
asset's plan.

LINK, HYPE, XAU, and XAG stop at the market-map and shadow-analysis boundary.
They must not enter the order lifecycle, produce strategy intents for the
executor, consume portfolio risk, or be included in live fill/no-fill
statistics.

## Current research classification

The frozen baseline has been applied unchanged to the metals markets, allowing
direct comparison with the crypto results.

| Market | Valid plans | Filled | Win rate | Total R | Avg R/fill | PF | Max DD | Classification |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| XAU | 285 | 194 | 56.7% | +14.86R | +0.08R | 1.18 | 20.94R | Valid research market |
| XAG | 286 | 185 | 53.0% | +7.13R | +0.04R | 1.09 | 11.73R | Observe only |

XAU shows positive but thin expectancy that may not survive spread, slippage,
fees, and execution latency. XAG's edge is too close to break-even for promotion
consideration. These results authorize continued data collection only; they do
not authorize execution.

## Research data model

The asset registry must distinguish eligibility from observation:

```text
assets
  symbol
  asset_class
  exchange
  tradable
  research_only
```

Every trading and research asset receives a daily snapshot with, at minimum:

```text
market_snapshot
  timestamp
  symbol
  session
  overnight_high
  overnight_low
  fib_382
  fib_500
  fib_618
  vwap
  poc
  vah
  val
  previous_day_high
  previous_day_low
  previous_day_open
  previous_day_close
  previous_day_vwap
  previous_day_poc
  previous_day_vah
  previous_day_val
  internal_liquidity
  external_liquidity
```

The shared schema allows controlled comparison across asset classes while the
`tradable` and `research_only` fields prevent observed instruments from leaking
into production execution.

## Production and research boundary

Production authority is limited to the frozen overnight strategy and its
existing execution rules. Previous-day levels, additional profile
relationships, structural liquidity, order-book features, funding, and open
interest are observational.

Research may calculate shadow outcomes and compare them with the control group,
but it must never mutate the production plan. Promotion requires controlled A/B
testing, out-of-sample validation, realistic execution testing, paper trading,
and live shadow evidence.

## Thirty-day question

The first live experiment asks whether five independent baseline instances
reproduce their historical expectancy under real Lighter execution. Only after
that period may research evaluate asset allocation, filters, or adaptive risk.

In parallel, LINK and HYPE expand crypto observation, while XAU and XAG test
whether overnight-auction relationships are specific to crypto perpetuals or
transfer to session-driven metals. Their results remain separate from live
portfolio performance until they independently pass realistic-cost,
out-of-sample, paper-trading, and live-shadow validation.

The market-intelligence layer may continuously rank observed instruments by
expectancy, execution quality, liquidity, drawdown, and stability. Ranking is a
research output only and cannot rotate the production basket automatically.
