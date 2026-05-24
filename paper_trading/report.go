package papertrading

import (
	"fmt"
	"strings"

	"stock-backtester/data"
	"stock-backtester/engine"
)

const reportDateLayout = "2006-01-02"

// FormatDailyReport renders the human-readable daily snapshot. latestBar is
// the bar that was just processed (or the most recent known bar if the run
// was a no-op due to in-session / already-processed guard).
func FormatDailyReport(acc *Account, latestBar data.Bar) string {
	var b strings.Builder
	positionValue := float64(acc.Shares) * latestBar.Close
	totalAssets := acc.Cash + positionValue

	fmt.Fprintln(&b, "===== Paper Trading Daily Report =====")
	fmt.Fprintf(&b, "  Date           : %s\n", latestBar.Time.Format(reportDateLayout))
	fmt.Fprintf(&b, "  Symbol         : %s (%s)\n", acc.Symbol, acc.Market)
	fmt.Fprintf(&b, "  Strategy       : %s\n", strategyDisplay(acc.Strategy))
	fmt.Fprintf(&b, "  Cash           : %s\n", money(acc.Cash))

	if acc.Shares > 0 {
		fmt.Fprintf(&b, "  Shares         : %s @ avg %.2f\n", thousands(acc.Shares), acc.AvgCost)
		fmt.Fprintf(&b, "  Last Close     : %.2f → position %s\n", latestBar.Close, money(positionValue))
	} else {
		fmt.Fprintf(&b, "  Shares         : 0 (flat)\n")
		fmt.Fprintf(&b, "  Last Close     : %.2f\n", latestBar.Close)
	}
	fmt.Fprintf(&b, "  Total Assets   : %s\n", money(totalAssets))

	if pnl, pct, ok := todaysPnL(acc.DailyEquity); ok {
		fmt.Fprintf(&b, "  Today's P&L    : %+s (%+.2f%%)\n", money(pnl), pct*100)
	} else {
		fmt.Fprintf(&b, "  Today's P&L    : n/a (need ≥2 daily snapshots)\n")
	}
	totalReturn := (totalAssets - acc.InitialCash) / acc.InitialCash
	fmt.Fprintf(&b, "  Total Return   : %+.2f%% vs initial %s\n", totalReturn*100, money(acc.InitialCash))

	if acc.Pending != nil {
		fmt.Fprintf(&b, "  Pending Signal : %s decided %s close (executes next open)\n",
			acc.Pending.Side, acc.Pending.DecidedOn.Format(reportDateLayout))
	} else {
		fmt.Fprintln(&b, "  Pending Signal : none")
	}

	cutoff := latestBar.Time.AddDate(0, 0, -30)
	recent := 0
	for _, t := range acc.Trades {
		if !t.Time.Before(cutoff) {
			recent++
		}
	}
	fmt.Fprintf(&b, "  Recent Trades  : %d in last 30 sessions (total %d)\n", recent, len(acc.Trades))
	return b.String()
}

func todaysPnL(equity []engine.EquityPoint) (float64, float64, bool) {
	if len(equity) < 2 {
		return 0, 0, false
	}
	curr := equity[len(equity)-1].Total
	prev := equity[len(equity)-2].Total
	if prev == 0 {
		return curr - prev, 0, true
	}
	return curr - prev, (curr - prev) / prev, true
}

func strategyDisplay(p StratParams) string {
	switch p.Name {
	case "sma":
		return fmt.Sprintf("sma_%d_%d", p.Short, p.Long)
	case "rsi":
		return fmt.Sprintf("rsi_%d_%g_%g", p.RSIPeriod, p.RSILow, p.RSIHigh)
	default:
		return p.Name
	}
}

func money(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := int64(v)
	frac := v - float64(whole)
	out := thousands64(whole) + fmt.Sprintf(".%02d", int64(frac*100+0.5))
	if neg {
		return "-" + out
	}
	return out
}

func thousands(n int) string  { return thousands64(int64(n)) }
func thousands64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte(',')
		}
	}
	for i := rem; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}
