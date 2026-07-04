// Package sqlite implements the store.Store persistence contract on a pure-Go
// SQLite database (modernc.org/sqlite, no CGo).
package sqlite

import "github.com/JiaCheng2004/Polaris/internal/store"

// Store satisfies the full persistence contract.
var _ store.Store = (*Store)(nil)
