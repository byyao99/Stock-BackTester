package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	yahooBase = "https://query1.finance.yahoo.com/v8/finance/chart/"
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	httpTimeout = 30 * time.Second
)

type yahooResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func Fetch(symbol string, start, end time.Time) ([]Bar, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("end (%s) must be after start (%s)",
			end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	u, err := url.Parse(yahooBase + url.PathEscape(symbol))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("period1", fmt.Sprintf("%d", start.Unix()))
	q.Set("period2", fmt.Sprintf("%d", end.Unix()))
	q.Set("interval", "1d")
	q.Set("events", "history")
	q.Set("includeAdjustedClose", "true")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("yahoo read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var yr yahooResponse
	if err := json.Unmarshal(body, &yr); err != nil {
		return nil, fmt.Errorf("yahoo decode: %w (body: %s)", err, truncate(string(body), 200))
	}
	if yr.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error %s: %s", yr.Chart.Error.Code, yr.Chart.Error.Description)
	}
	if len(yr.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo returned no result for %s", symbol)
	}

	r := yr.Chart.Result[0]
	if len(r.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("yahoo returned no quote indicators for %s", symbol)
	}
	q0 := r.Indicators.Quote[0]
	bars := make([]Bar, 0, len(r.Timestamp))
	for i, ts := range r.Timestamp {
		if i >= len(q0.Open) || i >= len(q0.Close) {
			break
		}
		open := derefFloat(q0.Open, i)
		closePx := derefFloat(q0.Close, i)
		if open == 0 || closePx == 0 {
			continue
		}
		bars = append(bars, Bar{
			Time:   time.Unix(ts, 0).UTC(),
			Open:   open,
			High:   derefFloat(q0.High, i),
			Low:    derefFloat(q0.Low, i),
			Close:  closePx,
			Volume: derefInt(q0.Volume, i),
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no bars returned for %s between %s and %s",
			symbol, start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
	return bars, nil
}

func derefFloat(s []*float64, i int) float64 {
	if i >= len(s) || s[i] == nil {
		return 0
	}
	return *s[i]
}

func derefInt(s []*int64, i int) int64 {
	if i >= len(s) || s[i] == nil {
		return 0
	}
	return *s[i]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
