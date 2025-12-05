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
		"PRAGMA busy_timeout=5000",  // 5 sec busy timeout
		"PRAGMA foreign_keys=ON",    // enable foreign key constraints
	}

	for _, pragma := range pragmas {
		_, err = db.ExecContext(ctx, pragma)
		if err != nil {
			return nil, fmt.Errorf("set pragma %q: %w", pragma, err)
		}
	}

	db.SetMaxOpenConns(1) // SQLite doesn't support multiple writers

	if err = db.PingContext(ctx); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			slog.ErrorContext(ctx, "failed to close database after ping error", "error", closeErr)
		}
		return nil, fmt.Errorf("ping database: %w", err)
	}

	result := &DB{DB: db}
	err = result.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	return result, nil
}

// Init initializes the database schema.
func (db *DB) Init(ctx context.Context) error {
	_, err := db.ExecContext(ctx, initSQL)
	if err != nil {
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

	err = f(tx)
	if err != nil {
		err = fmt.Errorf("transaction function error: %w", err)
		rbErr := tx.Rollback()
		if rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback error: %w", rbErr))
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
