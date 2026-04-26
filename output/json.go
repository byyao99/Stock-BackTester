package output

import (
	"encoding/json"
	"os"

	"stock-backtester/analysis"
)

func WriteSummary(path string, s analysis.Summary) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
