# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Build / run / test:

```bash
go build ./...                              # compile everything
go run . --symbol 2330.TW --start 2020-01-01 --end 2024-12-31 --strategy sma --short 20 --long 60
go run . --mode paper --symbol 2330.TW --strategy sma --short 20 --long 60   # paper trading
go test ./...                               # all tests
go test ./paper_trading -run TestName       # single-package / single-test form
go vet ./...                                # static checks
```

Force re-fetch from Yahoo (otherwise cached): delete the matching file under `data/cache/{symbol}_{start}_{end}.csv`.

Required CLI flags (backtest): `--symbol --start --end --strategy`. Paper mode drops `--start --end`. See `README.md` for the full flag table.

## Architecture

The pipeline is strictly linear and lives almost entirely in `main.go`'s `run()`:

```
data.LoadOrFetch ─→ Strategy.GenerateSignals ─→ engine.Run ─→ analysis.Compute ─→ output.Write*
```

Packages and their roles:

- `data/` — `Bar` struct, market profile (`MarketOf` picks TW vs US by suffix), Yahoo v8 chart fetch, CSV cache. `LoadOrFetch` is the only entry point callers should use; `FetchLatest` is for the post-backtest "current market" lookup that bypasses cache.
- `strategy/` — `Strategy` interface (`Name`, `GenerateSignals`, `MinBars`). Indicators are computed via `github.com/cinar/indicator/v2`; helper funcs in each strategy file pad the leading idle window with `NaN` so output length matches input.
- `engine/` — `Run` is the backtest loop. `broker.go` has fee/lot math. `buyhold.go` computes the passive baseline using the *same* lot/fee rules.
- `analysis/` — pure functions over the engine's output (`EquityCurve`, `Trades`).
- `output/` — writes `trades.csv`, `equity_curve.csv`, `summary.json` into a fresh `results/{symbol}_{strategy}_{timestamp}/` directory per run.
- `paper_trading/` — daily simulated trading (`--mode paper`). Reuses `engine.MaxBuyShares/BuyCost/SellProceeds` for fills, `data.FetchLatest` for live bars, and any `strategy.Strategy` for signals. State lives in `paper_accounts/{symbol}_{strategy}.json`; one file per `(symbol, strategy params)`.

### Critical invariants (don't break these)

- **t-close signal / t+1-open execution.** `engine.Run` reads `signals[i]` and fills at `bars[i+1].Open`. The last bar is mark-to-market only — never trade on it. Any new strategy must conform: `signals[i]` may only use info up to and including `bars[i].Close`. This avoids look-ahead bias.
- **`signals` must be the same length as `bars`.** Pad with `SignalHold` during the indicator warm-up window.
- **All-in position sizing.** Buy = max whole lots cash can afford (see `MaxBuyShares`); sell = full liquidation. No partial sizing, no pyramiding, no shorting.
- **Lot size is market-dependent.** TW = 1000 shares/lot, US = 1 share. `--odd-lot` overrides TW to 1; `MaxBuyShares` already respects `Market.LotSize` so strategies don't need to know.
- **Fees:** TW 0.1425% both sides + 0.3% sell tax; US zero. Defined once in `data/market.go`; engine and `BuyHold` both consume it.
- **Round trips** (used for win rate / profit factor) pair sequential BUY→SELL only — see `analysis.roundTripStats`. An open BUY at the end of the run is *not* counted as a round trip.

### Adding a strategy

1. Implement `strategy.Strategy` in a new file under `strategy/`. Set `MinBars()` to "first index where an executable signal is possible" + 1 (because execution happens on the *next* bar). The existing strategies show the pattern: pad with `NaN` until the indicator warms up, then emit `SignalBuy`/`SignalSell`/`SignalHold`.
2. Add a `case` in `buildStrategy` in `main.go` and any new flags it needs.

### Sharpe / annualization conventions

`analysis/metrics.go`: 252 trading days/year, risk-free rate 2%, daily returns annualized via `× √252`. If you change these constants, downstream `summary.json` numbers will diverge from prior runs — note it.

### Paper trading specifics

The same t-close/t+1-open invariant applies, but signal generation and execution are split across two daily runs:

- **Run on day N (after close):** `signals[len-1]` decided from latest bar's close → stored as `account.pending` (BUY only when flat; SELL only when long; HOLD/redundant signals clear it). Daily equity point appended at day-N close.
- **Run on day N+1 (after close):** if `pending != nil` and the new bar's date is strictly after `pending.decided_on`, fill at the new bar's *open* via `engine.MaxBuyShares` + `BuyCost`/`SellProceeds`. Then mark-to-market on day-(N+1) close, then redo signal computation.

Two guards prevent ledger corruption / look-ahead:

- **In-session guard** (`session.go:settledMinAge = 6h`): if `now - latestBar.Time < 6h` the run prints status only and writes nothing. 6h covers normal TW (4.5h session) and US (6.5h session) days post-close.
- **Idempotency**: `account.last_processed_date` blocks double-processing on the same trading day. UTC-truncated, so TW bars (Time at UTC 01:00) and US bars (Time at UTC 13:30) both round to one day per session.

Strategy identity is part of the account (`StratParams` deep-equal check on every load) — running with different `--short`/`--long` against an existing file is a hard error. Tell users to pass a different `--account-file` instead of overwriting.

Notifications go through the `Notifier` interface in `paper_trading/notifier.go`; `ConsoleNotifier` writes to stderr. LINE / other transports plug in via the same interface — no other call site changes needed.
