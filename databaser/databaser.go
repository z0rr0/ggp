// Package databaser provides SQLite database connection and operations.
package databaser

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // SQLite driver
)

//go:embed init.sql
var initSQL string

// DB wraps sqlx.DB for database operations.
type DB struct {
	*sqlx.DB
}

// New creates a new database connection.
func New(ctx context.Context, path string) (*DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",   // write-ahead logging
		"PRAGMA synchronous=NORMAL", // balance between performance and safety
		"PRAGMA cache_size=-32000",  // 32 mb cache
		"PRAGMA busy_timeout=5000",  // 5 сек cache wait
		"PRAGMA foreign_keys=ON",    // enable foreign key constraints
	}

	for _, pragma := range pragmas {
		if _, err = db.ExecContext(ctx, pragma); err != nil {
			return nil, fmt.Errorf("set pragma %q: %w", pragma, err)
		}
	}

	db.SetMaxOpenConns(1) // SQLite doesn't support multiple writers

	if err = db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "failed to close database after ping error", "error", closeErr)
		}
		return nil, fmt.Errorf("ping database: %w", err)
	}

	result := &DB{DB: db}
	if err = result.Init(ctx); err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	return result, nil
}

// Init initializes the database schema.
func (db *DB) Init(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, initSQL); err != nil {
		return fmt.Errorf("create schema error: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// InTransaction executes the given function within a database transaction.
func InTransaction(ctx context.Context, db *DB, f func(tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err = f(tx); err != nil {
		err = fmt.Errorf("transaction function error: %w", err)
		if rbErr := tx.Rollback(); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback error: %w", rbErr))
		}
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
