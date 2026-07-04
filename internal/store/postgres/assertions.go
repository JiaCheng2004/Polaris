// Package postgres implements the store.Store persistence contract on PostgreSQL
// via pgx v5.
package postgres

import "github.com/JiaCheng2004/Polaris/internal/store"

// Store satisfies the full persistence contract.
var _ store.Store = (*Store)(nil)
