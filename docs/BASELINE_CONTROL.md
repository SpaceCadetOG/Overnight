# Cycle 1 Baseline Control

## Frozen contract

`baseline-v1-20260810` is the permanent Cycle 1 control. Its direction, entry,
stop, targets, sizing, expiry, and management rules must not be changed by ML,
forensics, research output, asset ranking, or shadow-strategy performance.

Any future production change requires a new strategy version and must be
evaluated as a challenger. Historical baseline records are append-only and are
never rewritten under the new version.

The machine-readable contract is
`config/baseline-control-cycle-1.json`.

## Authoritative universe

| Asset | Cycle 1 authority |
|---|---|
| BTC | Live control plus identical paper control |
| ETH | Live control plus identical paper control |
| SOL | Paper/research only |
| HYPE | Paper/research only |
| LIT | Paper/research only |
| XAU | Paper/research only |
| XAG | Paper/research only |
| LINK | Paper/research only |
| AAVE | Paper/research only |
| UNI | Paper/research only |
| ZEC | Paper/research only |
| BNB | Paper/research only |

The authoritative runtime list remains `internal/universe/assets.go`. All 12
assets generate independent daily maps and baseline observations. Research
classification never grants execution authority.

## Reproduced ideal-execution statistics

These results were reproduced on 2026-08-08 using the frozen default strategy
and `cmd/backtest -execution ideal`. They describe the control on the historical
files currently present in the repository. They are not realistic-cost or live
performance claims.

| Asset | Valid plans | Filled | No fill | Fill rate | Win rate | Total R | Avg R/plan | Avg R/fill | PF | Max DD |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| BTC | 1,668 | 1,223 | 445 | 73.3% | 56.1% | +135.81R | +0.08R | +0.11R | 1.26 | 22.55R |
| ETH | 1,488 | 1,059 | 429 | 71.2% | 55.9% | +119.17R | +0.08R | +0.11R | 1.26 | 23.61R |
| SOL | 1,663 | 1,148 | 515 | 69.0% | 52.5% | +81.22R | +0.05R | +0.07R | 1.15 | 23.44R |
| HYPE | 429 | 295 | 134 | 68.8% | 54.9% | +13.41R | +0.03R | +0.05R | 1.10 | 16.06R |
| LIT | — | — | — | — | — | — | — | — | — | — |
| XAU | 285 | 194 | 91 | 68.1% | 56.7% | +14.86R | +0.05R | +0.08R | 1.18 | 20.94R |
| XAG | 286 | 185 | 101 | 64.7% | 53.0% | +7.13R | +0.02R | +0.04R | 1.09 | 11.73R |
| LINK | 1,668 | 1,133 | 535 | 67.9% | 55.4% | +107.98R | +0.06R | +0.10R | 1.22 | 34.90R |
| AAVE | 1,668 | 1,112 | 556 | 66.7% | 54.0% | +54.20R | +0.03R | +0.05R | 1.11 | 23.31R |
| UNI | 1,668 | 1,114 | 554 | 66.8% | 50.4% | -3.39R | -0.00R | -0.00R | 0.99 | 49.01R |
| ZEC | 1,663 | 1,134 | 529 | 68.2% | 57.1% | +146.84R | +0.09R | +0.13R | 1.31 | 22.52R |
| BNB | 1,668 | 1,141 | 527 | 68.4% | 55.6% | +109.27R | +0.07R | +0.10R | 1.22 | 18.15R |

LIT is intentionally retained in the frozen universe, but no statistic is
reported because no LIT historical candle file was available. A result may be
added only after the input data, date range, source, and checksum are recorded.

The crypto files cover 2022-01-01 through 2026-08-04, except HYPE, which begins
2025-05-30. The Lighter metals files cover 2025-10-20 through 2026-08-05.
Different histories mean raw totals must not be used to rank assets without
aligned-period analysis.

## Independent ML shadow strategy

The ML shadow system is a separate challenger. At each daily map freeze it may
produce its own direction, entry, stop, TP1, TP2, TP3, expiry, and runner policy.
Its primary plan must be published before the entry window with an information
cutoff, feature version, dataset version, generator/model version, confidence,
uncertainty, and `shadow_only=true`.

The first full dataset starts progressive learning:

1. Deterministic candidate levels and unsupervised regime discovery.
2. Supervised scoring after labeled daily outcomes accumulate.
3. Learned level selection after walk-forward evidence exists.
4. Independent continuous level generation after stable calibration.
5. Reinforcement-learning management research only after the execution
   simulator is validated.

The shadow system uses the same virtual execution rules, fees, spread, latency,
partial-fill assumptions, and forensic pipeline as paper research. It can learn
from every outcome, but it cannot write to baseline plans or production order
streams.

## Required daily comparison

For every asset, preserve both precommitted plans and compare fill, result,
MFE, MAE, realized or virtual R, drawdown, execution cost, and data quality.
Alternative shadow candidates may be retained for research, but only the
predeclared primary challenger counts in the official baseline comparison.
