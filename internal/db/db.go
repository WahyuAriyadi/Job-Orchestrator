package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Open creates a pooled *sql.DB for regular query/exec use (repositories,
// handlers). This is separate from the dedicated connection the leader
// elector uses, because advisory locks are session-scoped: the elector needs
// one connection it holds onto for the lifetime of its leadership, not a
// pool that hands out different connections per query.
func Open(dsn string) (*sql.DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(30 * time.Minute)

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return conn, nil
}
