package databaser

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
)

// Event represents a load event with a timestamp and load percentage.
type Event struct {
	Timestamp time.Time `db:"timestamp"`
	Load      uint8     `db:"load"`
}

// FloatLoad returns the load as a float64.
func (e *Event) FloatLoad() float64 {
	return float64(e.Load)
}

// LogValue implements slog.LogValuer for Event.
func (e *Event) LogValue() slog.Value {
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
func (db *DB) GetEvents(ctx context.Context, period time.Duration) ([]Event, error) {
	const query = `SELECT timestamp, load FROM events WHERE timestamp >= ? ORDER BY timestamp;`
	var (
		ts     = time.Now().UTC().Add(-period)
		events []Event
	)

	slog.DebugContext(ctx, "GetEvents", "query", query, "since", ts)
	if err := db.SelectContext(ctx, &events, query, ts); err != nil {
		return nil, fmt.Errorf("failed select events: %w", err)
	}

	return events, nil
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
