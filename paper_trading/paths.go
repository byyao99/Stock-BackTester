package papertrading

import (
	"path/filepath"
	"strings"
)

// DefaultAccountPath builds the canonical path for a (symbol, strategy) pair.
// Dots and slashes in symbol are replaced so the result is filesystem-safe.
func DefaultAccountPath(root, symbol, strategyName string) string {
	safeSymbol := strings.NewReplacer(".", "_", "/", "_", " ", "_").Replace(symbol)
	return filepath.Join(root, safeSymbol+"_"+strategyName+".json")
}
