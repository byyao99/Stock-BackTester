package data

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const cacheDir = "data/cache"

const dateFmt = "2006-01-02"

func cachePath(symbol string, start, end time.Time) string {
	safe := strings.ReplaceAll(symbol, "/", "_")
	name := fmt.Sprintf("%s_%s_%s.csv", safe, start.Format(dateFmt), end.Format(dateFmt))
	return filepath.Join(cacheDir, name)
}

func LoadOrFetch(symbol string, start, end time.Time) ([]Bar, bool, error) {
	path := cachePath(symbol, start, end)
	if bars, err := readCache(path); err == nil {
		return bars, true, nil
	}
	bars, err := Fetch(symbol, start, end)
	if err != nil {
		return nil, false, err
	}
	if err := writeCache(path, bars); err != nil {
		return bars, false, fmt.Errorf("write cache %s: %w", path, err)
	}
	return bars, false, nil
}

func readCache(path string) ([]Bar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("cache %s has no data rows", path)
	}
	bars := make([]Bar, 0, len(rows)-1)
	for i, row := range rows[1:] {
		if len(row) < 6 {
			return nil, fmt.Errorf("cache %s row %d malformed", path, i+2)
		}
		t, err := time.Parse(time.RFC3339, row[0])
		if err != nil {
			return nil, fmt.Errorf("cache %s row %d time: %w", path, i+2, err)
		}
		open, _ := strconv.ParseFloat(row[1], 64)
		high, _ := strconv.ParseFloat(row[2], 64)
		low, _ := strconv.ParseFloat(row[3], 64)
		closePx, _ := strconv.ParseFloat(row[4], 64)
		vol, _ := strconv.ParseInt(row[5], 10, 64)
		bars = append(bars, Bar{Time: t, Open: open, High: high, Low: low, Close: closePx, Volume: vol})
	}
	return bars, nil
}

func writeCache(path string, bars []Bar) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"time", "open", "high", "low", "close", "volume"}); err != nil {
		return err
	}
	for _, b := range bars {
		row := []string{
			b.Time.UTC().Format(time.RFC3339),
			strconv.FormatFloat(b.Open, 'f', 6, 64),
			strconv.FormatFloat(b.High, 'f', 6, 64),
			strconv.FormatFloat(b.Low, 'f', 6, 64),
			strconv.FormatFloat(b.Close, 'f', 6, 64),
			strconv.FormatInt(b.Volume, 10),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}
