package parser

import (
	"crypto/sha256"
	"sync"

	"github.com/maggisk/rexlang/internal/ast"
)

// parseCache caches parse results keyed by a SHA-256 hash of the source.
// Within a single compilation, the same module source may be parsed multiple
// times (once in the typechecker, once in the IR resolver). This avoids
// redundant lexing and parsing.
var (
	parseCache   = map[[32]byte]parseCacheEntry{}
	parseCacheMu sync.Mutex
)

type parseCacheEntry struct {
	exprs []ast.Expr
	err   error
}

// ClearParseCache resets the parse cache. Intended for tests.
func ClearParseCache() {
	parseCacheMu.Lock()
	parseCache = map[[32]byte]parseCacheEntry{}
	parseCacheMu.Unlock()
}

func cacheKey(source string) [32]byte {
	return sha256.Sum256([]byte(source))
}

func cacheLookup(key [32]byte) ([]ast.Expr, error, bool) {
	parseCacheMu.Lock()
	defer parseCacheMu.Unlock()
	if entry, ok := parseCache[key]; ok {
		return entry.exprs, entry.err, true
	}
	return nil, nil, false
}

func cacheStore(key [32]byte, exprs []ast.Expr, err error) {
	parseCacheMu.Lock()
	parseCache[key] = parseCacheEntry{exprs, err}
	parseCacheMu.Unlock()
}
