# Overnight Strategy Production Architecture

                    Mac Development
                           │
                           ▼
                      GitHub Repository
                           │
                           ▼
                    GitHub Actions (CI)
                           │
                           ▼
                    Raspberry Pi Server
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
  Lighter Collector   Daily Levels     BTC Executor
        │                  │                  │
        └──────────────┬───┴──────────────┬───┘
                       ▼
               Health Monitor
                       │
                       ▼
                  Recorder (SSD)
                       │
      ┌────────────────┼─────────────────┐
      │                │                 │
      ▼                ▼                 ▼
 Market Data      Reports          Executions
      │                │                 │
      └────────────────┼─────────────────┘
                       ▼
                 Notification Service
                       │
                       ▼
                   Mobile Phone

Persistent Storage

/mnt/trading

archive/
backtests/
candles/
executions/
fills/
health/
logs/
marketdata/
models/
orderbook/
recorder/
reports/
snapshots/
state/
trade_windows/

Current live trading scope:

• Basket report generated for all configured assets.
• Automated execution restricted to BTC.
• Recorder captures all market data.
• Daily reports archived.
• Notifications sent to mobile.
