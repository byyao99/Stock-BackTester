package papertrading

import (
	"fmt"
	"io"
	"os"
	"time"

	"stock-backtester/engine"
)

type EventKind int

const (
	EventTradeFilled EventKind = iota
	EventSignalQueued
)

func (k EventKind) String() string {
	switch k {
	case EventTradeFilled:
		return "TRADE_FILLED"
	case EventSignalQueued:
		return "SIGNAL_QUEUED"
	default:
		return "UNKNOWN"
	}
}

type Event struct {
	Symbol string
	Date   time.Time
	Kind   EventKind
	Side   engine.Side
	Price  float64
	Shares int
}

type Notifier interface {
	Notify(ev Event) error
}

// ConsoleNotifier prints events to stderr so they don't interleave with the
// daily report on stdout.
type ConsoleNotifier struct {
	Out io.Writer // defaults to os.Stderr
}

func (c ConsoleNotifier) Notify(ev Event) error {
	w := c.Out
	if w == nil {
		w = os.Stderr
	}
	switch ev.Kind {
	case EventTradeFilled:
		_, err := fmt.Fprintf(w, "ALERT: %s %s %s filled %d @ %.2f on %s\n",
			ev.Symbol, ev.Kind, ev.Side, ev.Shares, ev.Price, ev.Date.Format("2006-01-02"))
		return err
	case EventSignalQueued:
		_, err := fmt.Fprintf(w, "ALERT: %s %s %s queued (ref close %.2f) on %s, executes next open\n",
			ev.Symbol, ev.Kind, ev.Side, ev.Price, ev.Date.Format("2006-01-02"))
		return err
	}
	return nil
}
