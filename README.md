# stock-backtester

Go 寫的股票策略回測 CLI，使用 Yahoo Finance 日線資料，支援台股（`*.TW`、`*.TWO`）與美股。
內建兩個範例策略：雙均線交叉（SMA Crossover）與 RSI 超買超賣。

---

## 環境需求

- Go 1.21+
- 網路連線（首次抓資料時呼叫 Yahoo Finance API）

依賴只有一個：[`github.com/cinar/indicator/v2`](https://github.com/cinar/indicator)（純 Go 技術指標套件，無 CGo）。

---

## 安裝

```bash
git clone <repo-url> stock-backtester
cd stock-backtester
go mod download
```

確認可以編譯：

```bash
go build ./...
```

---

## 快速開始

跑一次台積電 (2330.TW) 五年回測，雙均線 20/60：

```bash
go run . \
  --symbol 2330.TW \
  --start 2020-01-01 \
  --end 2024-12-31 \
  --strategy sma --short 20 --long 60
```

跑 Apple (AAPL) RSI 14：

```bash
go run . \
  --symbol AAPL \
  --start 2020-01-01 \
  --end 2024-12-31 \
  --strategy rsi --rsi-period 14 --rsi-low 30 --rsi-high 70 \
  --cash 100000
```

執行完後檢查 `./results/` 下的輸出資料夾。

---

## 命令列參數

| 旗標 | 預設 | 說明 |
| --- | --- | --- |
| `--symbol` | *(必填)* | Yahoo ticker，例如 `2330.TW`、`0050.TW`、`AAPL`、`MSFT` |
| `--start` | *(必填)* | 起始日（含），格式 `YYYY-MM-DD` |
| `--end` | *(必填)* | 結束日（含），格式 `YYYY-MM-DD` |
| `--strategy` | *(必填)* | 策略名稱：`sma` 或 `rsi` |
| `--short` | `20` | 【SMA】短均線天數 |
| `--long` | `60` | 【SMA】長均線天數 |
| `--rsi-period` | `14` | 【RSI】回看天數 |
| `--rsi-low` | `30` | 【RSI】超賣門檻（低於則買進） |
| `--rsi-high` | `70` | 【RSI】超買門檻（高於則賣出） |
| `--cash` | `1000000` | 起始資金（台股建議用預設，美股可下調如 `100000`） |
| `--out` | `results` | 結果輸出根目錄 |
| `--odd-lot` | `false` | 啟用零股交易（台股最小單位由 1000 股改為 1 股；美股本來就 1 股，加不加無差） |
| `--mode` | `backtest` | 執行模式：`backtest`（歷史回測）或 `paper`（紙上交易，見下節） |
| `--account-file` | `paper_accounts/{symbol}_{strategy}.json` | 【paper】帳戶狀態檔路徑 |

---

## 紙上交易（Paper Trading）

`--mode paper` 會把策略接到「最新行情」每天執行一次：抓今日收盤、用同一組策略決定訊號、把結果累積在一個帳戶 JSON 檔裡。**沿用回測同一套執行模型** —— 今天的訊號要等明天開盤才成交，手續費、整股、全倉等規則一致。

### 用法

```bash
go run . --mode paper \
  --symbol 2330.TW --strategy sma --short 20 --long 60 \
  --cash 1000000
```

不需要 `--start`/`--end`。可以跑 cron / launchd 每日盤後自動更新。

### 帳戶檔

預設位置：`paper_accounts/2330_TW_sma_20_60.json`（依 `symbol` + 策略命名）。一個檔案對應一組 `(symbol, strategy params)`；想用不同參數請加 `--account-file` 指定新路徑。

帳戶身份不符（symbol 或策略參數變動）會直接報錯，避免亂套導致帳本被汙染。

### 每日流程

1. 載入既有帳戶（不存在則用 `--cash` 初始化）。
2. 抓最近 ≥ 180 個日曆日的 bars。
3. **盤中保護**：若最新 bar 距現在不足 6 小時，視為盤未收 —— 只印當前狀態，不更新。
4. **冪等性**：若該 bar 的日期 ≤ `last_processed_date`，視為已處理 —— 印「already processed」訊息後返回。
5. **成交昨日訊號**：若帳戶有 `pending` 且今日 bar 日期晚於 `pending.decided_on`，按 t+1 規則於今日 bar 開盤成交。
6. Mark-to-market：以今日收盤計算總資產，附加一筆 `daily_equity`。
7. 計算今日訊號：BUY（且空手）或 SELL（且持倉）→ 寫入 `pending` 等明日成交；其他狀況清空 `pending`。
8. 持久化（`tmp + rename` 原子寫入）。
9. 印當日報告。

成交事件與新訊號排隊都會以 `ALERT:` 開頭印到 stderr（未來會接 LINE）。

### 報告範例

```
===== Paper Trading Daily Report =====
  Date           : 2026-04-25
  Symbol         : 2330.TW (TW)
  Strategy       : sma_20_60
  Cash           : 412,300.00
  Shares         : 2,000 @ avg 587.50
  Last Close     : 612.00 → position 1,224,000.00
  Total Assets   : 1,636,300.00
  Today's P&L    : +24,500.00 (+1.52%)
  Total Return   : +63.63% vs initial 1,000,000.00
  Pending Signal : SELL decided 2026-04-25 close (executes next open)
  Recent Trades  : 3 in last 30 sessions (total 12)
```

### 與回測的差異

| | 回測 | 紙上交易 |
|---|---|---|
| 執行頻率 | 一次跑完整段 | 每天一次 |
| 資料來源 | 快取 / Yahoo（指定區間） | Yahoo 最新 N 天 |
| 狀態 | 無，跑完即丟 | 累積在 `paper_account.json` |
| 訊號 → 成交 | t 收盤 → t+1 開盤（同一次計算） | 今日收盤 → 明日開盤（跨兩次執行） |
| 輸出 | `results/` 三個檔 | `paper_accounts/{...}.json` + stdout 報告 |

---

## 輸出檔案

每次執行會在 `./results/{symbol}_{strategy}_{timestamp}/` 底下產生三個檔案：

### `trades.csv`
每一筆成交紀錄。

| 欄位 | 說明 |
| --- | --- |
| `time` | 成交時間（UTC, RFC3339） |
| `side` | `BUY` 或 `SELL` |
| `price` | 成交價（隔日開盤價） |
| `shares` | 股數 |
| `fee` | 手續費 |
| `tax` | 證交稅（僅台股賣方有，美股 0） |
| `cash_after` | 成交後現金餘額 |

### `equity_curve.csv`
每日資產曲線。

| 欄位 | 說明 |
| --- | --- |
| `time` | 收盤日期 |
| `cash` | 現金 |
| `shares` | 持有股數 |
| `position_value` | 部位市值 = `shares × close` |
| `total` | 總資產 = `cash + position_value` |

### `summary.json`
績效摘要。

```json
{
  "symbol": "2330.TW",
  "strategy": "sma_20_60",
  "market": "TW",
  "start": "2020-01-02T01:00:00Z",
  "end": "2024-12-30T01:00:00Z",
  "trading_days": 1214,
  "initial_cash": 1000000,
  "final_equity": 2393379.09,
  "total_return": 1.3934,
  "annual_return": 0.1988,
  "max_drawdown": 0.3718,
  "sharpe_ratio": 0.926,
  "win_rate": 0.375,
  "profit_factor": 3.689,
  "trade_count": 17,
  "round_trip_count": 8,
  "buy_hold_return": 1.4910,
  "alpha": -0.0976
}
```

指標定義：

- **total_return**：`final / initial - 1`
- **annual_return**：`(1 + total_return)^(1/years) - 1`，年化用每年 252 個交易日
- **max_drawdown**：歷史峰值到谷底的最大相對跌幅
- **sharpe_ratio**：日報酬計算後年化（`× √252`），無風險利率 2%
- **win_rate**：獲利的 round trip 數 / 總 round trip 數（一個 BUY→SELL 配對為一個 round trip）
- **profit_factor**：總獲利金額 / 總虧損金額；若沒有虧損 round trip 則為 `null`，CLI 顯示 `n/a`
- **buy_hold_return**：同期間「買進並持有」的對照報酬；`bars[1].Open` 進場、`bars[-1].Close` mark-to-market，使用相同 lot/fee 規則；現金不夠 1 張時為 `null`
- **alpha**：`total_return − buy_hold_return`；正值代表策略贏被動持有，負值代表沒有 alpha（不如躺平）

---

## 資料快取

第一次抓某段資料會打 Yahoo API 並寫入：

```
data/cache/{symbol}_{start}_{end}.csv
```

之後相同 `(symbol, start, end)` 會直接讀 cache，CLI 會印 `Got N bars from cache`。

要強制重抓，刪掉對應的 cache 檔即可：

```bash
rm data/cache/2330.TW_2020-01-01_2024-12-31.csv
```

---

## 設計假設

- **執行模型**：`t` 收盤產生訊號 → `t+1` 開盤成交。避免 look-ahead bias，回測結果更貼近實盤。
- **倉位管理**：All-in。買進訊號用全部現金能買的最大整股；賣出訊號全數出清。
- **整股限制**：台股 1000 股一張，現金不夠一張就不下單；美股 1 股起跳。加 `--odd-lot` 可改為 1 股最小單位（適用想用小資金回測高價股，例如 100 萬測台積電）。
- **手續費**：
  - 台股：買方 0.1425%、賣方 0.1425% + 0.3% 證交稅
  - 美股：0%（簡化假設）
- **不模擬**：滑價、做空、融資、部分成交、股利再投資。
- **K 棒最後一根**：只 mark-to-market，不交易（沒有 t+1 可成交）。

---

## 加新策略

實作 `strategy/strategy.go` 的介面：

```go
type Strategy interface {
    Name() string
    GenerateSignals(bars []data.Bar) []Signal // 與 bars 等長
}
```

`signals[i]` 是看完 `bars[0..i]`（含 `bars[i].Close`）後的決策，回測引擎會以 `bars[i+1].Open` 成交。
寫好後在 `main.go` 的 `buildStrategy` 加一條 case 即可。

---

## 專案結構

```
stock-backtester/
├── main.go               CLI 入口、flag 解析
├── data/
│   ├── bar.go            Bar struct
│   ├── market.go         市場 profile（lot size、手續費、稅）
│   ├── fetcher.go        Yahoo v8 chart endpoint 直接呼叫
│   └── cache.go          CSV 快取讀寫
├── strategy/
│   ├── strategy.go       Strategy interface, Signal enum
│   ├── sma_crossover.go  雙均線交叉
│   └── rsi.go            RSI 超買超賣
├── engine/
│   ├── engine.go         回測主迴圈（t 收盤訊號 / t+1 開盤成交）
│   ├── broker.go         手續費、整股換算
│   ├── trade.go          Trade record
│   └── equity.go         EquityPoint
├── analysis/
│   ├── metrics.go        Total/Annual/MDD/Sharpe/WinRate/PF
│   └── summary.go        Summary struct
├── output/
│   ├── csv.go            trades.csv, equity_curve.csv
│   └── json.go           summary.json
└── paper_trading/
    ├── account.go        Account / StratParams / PendingSignal、Load/Save
    ├── executor.go       ExecutePending（在新 bar 開盤成交）
    ├── session.go        RunDailyUpdate 每日 tick 協調器
    ├── notifier.go       Notifier interface + ConsoleNotifier
    ├── report.go         FormatDailyReport
    └── paths.go          DefaultAccountPath
```

---

## 常見問題

**Q: Yahoo 回 `remote-error` 或 401？**
A: Yahoo 會擋沒有 User-Agent 的請求，本專案的 `data/fetcher.go` 已自帶瀏覽器 UA。如果還是失敗，可能是被速率限制，等幾分鐘再試。

**Q: 為什麼某些日期沒有交易？**
A: 兩個可能：(1) 策略 warm-up 期還沒結束（例如 SMA 60 需要前 60 根 K 棒），(2) 訊號沒有觸發。

**Q: 台股回測為什麼買的股數都是 1000 的倍數？**
A: 規則就是這樣（1 張 = 1000 股）。如果某天現金不夠一張，那天就不會下單。

**Q: Sharpe 為什麼跟其他軟體算的不一樣？**
A: 慣例可能不同。本專案：日報酬，無風險利率 0.02 / 252，年化 × √252。
