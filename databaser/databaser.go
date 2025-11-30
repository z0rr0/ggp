// Package databaser provides SQLite database connection and operations.
package databaser

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

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

// Event represents a load event with a timestamp and load percentage.
type Event struct {
	Timestamp time.Time `db:"timestamp"`
	Load      uint8     `db:"load"`
}

// LogValue implements slog.LogValuer for Event.
func (e Event) LogValue() slog.Value {
	return slog.StringValue(fmt.Sprintf("{timestamp: '%s', load: %d}", e.Timestamp.Format(time.RFC3339), e.Load))
}

// SaveEvent stores an event in the database.
func (db *DB) SaveEvent(ctx context.Context, event Event) error {
	const query = `INSERT INTO events (timestamp, load) VALUES (:timestamp, :load);`

	if _, err := db.NamedExecContext(ctx, query, event); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

// SaveManyEvents stores multiple events in the database.
func (db *DB) SaveManyEvents(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}

	const query = `INSERT OR REPLACE INTO events (timestamp, load) VALUES (:timestamp, :load);`

	if _, err := db.NamedExecContext(ctx, query, events); err != nil {
		return fmt.Errorf("insert events: %w", err)
	}

	return nil
}

// GetEvents retrieves events to the current time minus the given period.
func (db *DB) GetEvents(ctx context.Context, period time.Duration, location *time.Location) ([]Event, error) {
	const query = `SELECT timestamp, load FROM events WHERE timestamp >= ? ORDER BY timestamp;`
	var (
		ts     = time.Now().UTC().Add(-period)
		events []Event
	)

	slog.DebugContext(ctx, "GetEvents", "query", query, "since", ts)
	if err := db.SelectContext(ctx, &events, query, ts); err != nil {
		return nil, fmt.Errorf("failed select events: %w", err)
	}

	n := len(events)
	if n > 0 {
		slog.DebugContext(ctx, "GetEvents", "first", events[0], "last", events[n-1], "count", n)
	}

	for _, event := range events {
		event.Timestamp = event.Timestamp.In(location)
	}

	return events, nil
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

// NewEventFromCSVRecord creates an Event from a CSV record.
func NewEventFromCSVRecord(record []string, location *time.Location) (*Event, error) {
	if len(record) < 2 {
		return nil, fmt.Errorf("invalid record length: %d", len(record))
	}

	timestamp, err := time.ParseInLocation(time.DateTime, record[0], location)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp %q: %w", record[0], err)
	}

	load, err := strconv.ParseUint(record[1], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("parse load %q: %w", record[1], err)
	}

	return &Event{Timestamp: timestamp, Load: uint8(load)}, nil
}

// SaveManyEventsTx stores multiple events in the database within a transaction.
func SaveManyEventsTx(ctx context.Context, tx *sqlx.Tx, events []*Event) error {
	if len(events) == 0 {
		return nil
	}

	const query = `INSERT OR REPLACE INTO events (timestamp, load) VALUES (:timestamp, :load);`

	if _, err := tx.NamedExecContext(ctx, query, events); err != nil {
		return fmt.Errorf("insert events: %w", err)
	}

	return nil
}
