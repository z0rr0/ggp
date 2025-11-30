// Package importer provides functionality to import data from various sources.
package importer

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/z0rr0/ggp/databaser"
)

const chunkSize = 250

type importReader struct {
	db       *databaser.DB
	reader   io.Reader
	location *time.Location
	err      error
}

// ImportCSV imports events from a CSV file into the database.
func ImportCSV(db *databaser.DB, importPath string, timeout time.Duration, location *time.Location) error {
	f, err := os.Open(importPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Error("failed to close import file", "error", closeErr)
		}
	}()

	r := &importReader{
		db:       db,
		reader:   f,
		location: location,
	}
	return r.InsertEvents(context.Background(), timeout)
}

// Read reads events from the CSV file and yields them as a sequence.
func (r *importReader) Read() iter.Seq[*databaser.Event] {
	return func(yield func(*databaser.Event) bool) {
		csvReader := csv.NewReader(r.reader)
		if _, headerErr := csvReader.Read(); headerErr != nil {
			r.err = fmt.Errorf("header read: %w", headerErr)
			return
		}

		i := 1
		for {
			record, err := csvReader.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				r.err = fmt.Errorf("csv read line %d: %w", i, err)
				return
			}

			event, err := databaser.NewEventFromCSVRecord(record, r.location)
			if err != nil {
				r.err = fmt.Errorf("parse record %v: %w", record, err)
				return
			}
			event.Timestamp = event.Timestamp.In(time.UTC) // save in UTC

			if !yield(event) {
				return
			}
			i++
		}
	}
}

func (r *importReader) ReadChunk(size int) iter.Seq[[]*databaser.Event] {
	return func(yield func([]*databaser.Event) bool) {
		var i int
		batch := make([]*databaser.Event, 0, size)

		for event := range r.Read() {
			i++
			batch = append(batch, event)

			if i%size == 0 {
				if !yield(batch) {
					return
				}
				// reset batch
				i, batch = 0, make([]*databaser.Event, 0, size)
			}
		}

		if len(batch) > 0 {
			if !yield(batch) {
				return
			}
		}
	}
}

// InsertEvents inserts events into the database within a specified timeout.
func (r *importReader) InsertEvents(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	count := 0
	err := databaser.InTransaction(ctx, r.db, func(tx *sqlx.Tx) error {
		for rows := range r.ReadChunk(chunkSize) {
			if err := databaser.SaveManyEventsTx(ctx, tx, rows); err != nil {
				return fmt.Errorf("save events: %w", err)
			}
			n := len(rows)
			slog.Info("chunk imported events", "count", n)
			count += n
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("insert events: %w", err)
	}

	slog.Info("total imported events", "count", count)
	return nil
}
