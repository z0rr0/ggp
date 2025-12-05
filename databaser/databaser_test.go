package databaser

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()
	db, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close test database: %v", err)
		}
	})
	return db
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "in-memory database",
			path:    ":memory:",
			wantErr: false,
		},
		{
			name:    "invalid path",
			path:    "/non/existent/directory/test.db",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := New(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if db != nil {
				if err := db.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}
		})
	}
}

func TestInit_CreatesTablesIdempotently(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Init is called in New(), calling again should not fail
	if err := db.Init(ctx); err != nil {
		t.Errorf("Init() second call error = %v", err)
	}

	// Verify tables exist
	tables := []string{"users", "events", "holidays"}
	for _, table := range tables {
		var count int
		err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table)
		if err != nil {
			t.Errorf("failed to check %s table: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s not created", table)
		}
	}
}

func TestSaveEvent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{
			name: "valid event",
			event: Event{
				Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				Load:      75,
			},
			wantErr: false,
		},
		{
			name: "zero load",
			event: Event{
				Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
				Load:      0,
			},
			wantErr: false,
		},
		{
			name: "max load",
			event: Event{
				Timestamp: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
				Load:      255,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.SaveEvent(ctx, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveEvent_DuplicateTimestamp(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	event := Event{Timestamp: ts, Load: 50}

	if err := db.SaveEvent(ctx, event); err != nil {
		t.Fatalf("first SaveEvent() error = %v", err)
	}

	// Duplicate timestamp should fail (PRIMARY KEY constraint)
	event.Load = 60
	if err := db.SaveEvent(ctx, event); err == nil {
		t.Error("SaveEvent() expected error for duplicate timestamp")
	}
}

func TestSaveManyEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		events  []Event
		wantErr bool
	}{
		{
			name:    "empty slice",
			events:  []Event{},
			wantErr: false,
		},
		{
			name:    "nil slice",
			events:  nil,
			wantErr: false,
		},
		{
			name: "multiple events",
			events: []Event{
				{Timestamp: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), Load: 50},
				{Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC), Load: 60},
				{Timestamp: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC), Load: 70},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.SaveManyEvents(ctx, tt.events)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveManyEvents() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveManyEvents_ReplacesDuplicates(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	events := []Event{{Timestamp: ts, Load: 50}}

	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("first SaveManyEvents() error = %v", err)
	}

	// INSERT OR REPLACE should update existing
	events[0].Load = 99
	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("second SaveManyEvents() error = %v", err)
	}

	var load uint8
	if err := db.GetContext(ctx, &load, "SELECT load FROM events WHERE timestamp = ?", ts); err != nil {
		t.Fatalf("failed to get load: %v", err)
	}
	if load != 99 {
		t.Errorf("load = %d, want 99", load)
	}
}

func TestGetEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	events := []Event{
		{Timestamp: now.Add(-2 * time.Hour), Load: 50},
		{Timestamp: now.Add(-1 * time.Hour), Load: 60},
		{Timestamp: now.Add(-30 * time.Minute), Load: 70},
	}

	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("SaveManyEvents() error = %v", err)
	}

	tests := []struct {
		name      string
		period    time.Duration
		wantCount int
	}{
		{
			name:      "get all within 3 hours",
			period:    3 * time.Hour,
			wantCount: 3,
		},
		{
			name:      "get last hour only",
			period:    time.Hour + time.Minute,
			wantCount: 2,
		},
		{
			name:      "get none - period too short",
			period:    time.Minute,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GetEvents(ctx, tt.period)
			if err != nil {
				t.Errorf("GetEvents() error = %v", err)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("GetEvents() returned %d events, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestGetEvents_OrderedByTimestamp(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	// Insert in reverse order
	events := []Event{
		{Timestamp: now.Add(-30 * time.Minute), Load: 70},
		{Timestamp: now.Add(-2 * time.Hour), Load: 50},
		{Timestamp: now.Add(-1 * time.Hour), Load: 60},
	}

	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("SaveManyEvents() error = %v", err)
	}

	got, err := db.GetEvents(ctx, 3*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	// Should be ordered ascending by timestamp
	for i := 1; i < len(got); i++ {
		if !got[i].Timestamp.After(got[i-1].Timestamp) {
			t.Errorf("events not ordered: [%d]=%v >= [%d]=%v",
				i-1, got[i-1].Timestamp, i, got[i].Timestamp)
		}
	}
}

func TestGetAllEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert 10 events
	var events []Event
	base := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		events = append(events, Event{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Load:      uint8(i * 10),
		})
	}

	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("SaveManyEvents() error = %v", err)
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
		wantFirst uint8
	}{
		{
			name:      "first page",
			limit:     3,
			offset:    0,
			wantCount: 3,
			wantFirst: 0,
		},
		{
			name:      "second page",
			limit:     3,
			offset:    3,
			wantCount: 3,
			wantFirst: 30,
		},
		{
			name:      "last partial page",
			limit:     5,
			offset:    8,
			wantCount: 2,
			wantFirst: 80,
		},
		{
			name:      "offset beyond data",
			limit:     5,
			offset:    100,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GetAllEvents(ctx, tt.limit, tt.offset)
			if err != nil {
				t.Errorf("GetAllEvents() error = %v", err)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("GetAllEvents() returned %d events, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 && got[0].Load != tt.wantFirst {
				t.Errorf("first event load = %d, want %d", got[0].Load, tt.wantFirst)
			}
		})
	}
}

func TestNewEventFromCSVRecord(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name      string
		record    []string
		wantTime  time.Time
		wantLoad  uint8
		wantErr   bool
	}{
		{
			name:     "valid record",
			record:   []string{"2024-01-15 10:30:00", "75"},
			wantTime: time.Date(2024, 1, 15, 10, 30, 0, 0, loc),
			wantLoad: 75,
			wantErr:  false,
		},
		{
			name:     "zero load",
			record:   []string{"2024-01-15 00:00:00", "0"},
			wantTime: time.Date(2024, 1, 15, 0, 0, 0, 0, loc),
			wantLoad: 0,
			wantErr:  false,
		},
		{
			name:     "max uint8 load",
			record:   []string{"2024-12-31 23:59:59", "255"},
			wantTime: time.Date(2024, 12, 31, 23, 59, 59, 0, loc),
			wantLoad: 255,
			wantErr:  false,
		},
		{
			name:    "empty record",
			record:  []string{},
			wantErr: true,
		},
		{
			name:    "single element",
			record:  []string{"2024-01-15 10:30:00"},
			wantErr: true,
		},
		{
			name:    "invalid timestamp",
			record:  []string{"not-a-timestamp", "50"},
			wantErr: true,
		},
		{
			name:    "invalid load - not a number",
			record:  []string{"2024-01-15 10:30:00", "abc"},
			wantErr: true,
		},
		{
			name:    "invalid load - negative",
			record:  []string{"2024-01-15 10:30:00", "-1"},
			wantErr: true,
		},
		{
			name:    "invalid load - overflow",
			record:  []string{"2024-01-15 10:30:00", "256"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewEventFromCSVRecord(tt.record, loc)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEventFromCSVRecord() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if !got.Timestamp.Equal(tt.wantTime) {
				t.Errorf("timestamp = %v, want %v", got.Timestamp, tt.wantTime)
			}
			if got.Load != tt.wantLoad {
				t.Errorf("load = %d, want %d", got.Load, tt.wantLoad)
			}
		})
	}
}

func TestNewEventFromCSVRecord_WithLocation(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skip("Europe/Moscow timezone not available")
	}

	record := []string{"2024-01-15 10:30:00", "50"}
	got, err := NewEventFromCSVRecord(record, loc)
	if err != nil {
		t.Fatalf("NewEventFromCSVRecord() error = %v", err)
	}

	if got.Timestamp.Location() != loc {
		t.Errorf("location = %v, want %v", got.Timestamp.Location(), loc)
	}
}

func TestSaveManyEventsTx(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	events := []*Event{
		{Timestamp: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), Load: 50},
		{Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC), Load: 60},
	}

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyEventsTx(ctx, tx, events)
	})
	if err != nil {
		t.Fatalf("SaveManyEventsTx() error = %v", err)
	}

	got, err := db.GetAllEvents(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetAllEvents() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d events, want 2", len(got))
	}
}

func TestSaveManyEventsTx_EmptySlice(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyEventsTx(ctx, tx, nil)
	})
	if err != nil {
		t.Errorf("SaveManyEventsTx(nil) error = %v", err)
	}
}

func TestInTransaction_Commit(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO events (timestamp, load) VALUES (?, ?)",
			time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), 50)
		return err
	})
	if err != nil {
		t.Fatalf("InTransaction() error = %v", err)
	}

	var count int
	if err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM events"); err != nil {
		t.Fatalf("failed to count events: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestInTransaction_Rollback(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	testErr := driver.ErrBadConn // use existing error type
	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO events (timestamp, load) VALUES (?, ?)",
			time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), 50)
		if err != nil {
			return err
		}
		return testErr
	})

	if err == nil {
		t.Error("InTransaction() expected error")
	}

	var count int
	if err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM events"); err != nil {
		t.Fatalf("failed to count events: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (rollback should have occurred)", count)
	}
}

func TestGetHolidays(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	loc := time.UTC

	// Insert holidays via transaction
	holidays := []Holiday{
		{Day: dateOnly(2024, 1, 1, loc), Title: "New Year"},
		{Day: dateOnly(2024, 5, 1, loc), Title: "Labor Day"},
		{Day: dateOnly(2024, 12, 31, loc), Title: "New Year Eve"},
		{Day: dateOnly(2025, 1, 1, loc), Title: "Next Year"},
	}

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyHolidaysTx(ctx, tx, holidays)
	})
	if err != nil {
		t.Fatalf("SaveManyHolidaysTx() error = %v", err)
	}

	tests := []struct {
		name      string
		year      int
		wantCount int
		wantFirst string
	}{
		{
			name:      "year 2024",
			year:      2024,
			wantCount: 3,
			wantFirst: "New Year",
		},
		{
			name:      "year 2025",
			year:      2025,
			wantCount: 1,
			wantFirst: "Next Year",
		},
		{
			name:      "year with no holidays",
			year:      2020,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GetHolidays(ctx, tt.year, loc)
			if err != nil {
				t.Errorf("GetHolidays() error = %v", err)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("GetHolidays() returned %d holidays, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 && got[0].Title != tt.wantFirst {
				t.Errorf("first holiday title = %q, want %q", got[0].Title, tt.wantFirst)
			}
		})
	}
}

func TestGetHolidays_SetsLocation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Save with UTC
	holidays := []Holiday{
		{Day: dateOnly(2024, 1, 1, time.UTC), Title: "New Year"},
	}

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyHolidaysTx(ctx, tx, holidays)
	})
	if err != nil {
		t.Fatalf("SaveManyHolidaysTx() error = %v", err)
	}

	// Retrieve with different location
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("America/New_York timezone not available")
	}

	got, err := db.GetHolidays(ctx, 2024, loc)
	if err != nil {
		t.Fatalf("GetHolidays() error = %v", err)
	}

	if len(got) == 0 {
		t.Fatal("expected at least one holiday")
	}

	if got[0].Day.Time().Location() != loc {
		t.Errorf("location = %v, want %v", got[0].Day.Time().Location(), loc)
	}
}

func TestSaveManyHolidaysTx_DeletesExistingYearRange(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	loc := time.UTC

	// Insert initial holidays
	initial := []Holiday{
		{Day: dateOnly(2024, 1, 1, loc), Title: "Old Holiday 1"},
		{Day: dateOnly(2024, 6, 1, loc), Title: "Old Holiday 2"},
	}

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyHolidaysTx(ctx, tx, initial)
	})
	if err != nil {
		t.Fatalf("first SaveManyHolidaysTx() error = %v", err)
	}

	// Insert new holidays for same year - should replace
	updated := []Holiday{
		{Day: dateOnly(2024, 3, 1, loc), Title: "New Holiday"},
	}

	err = InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyHolidaysTx(ctx, tx, updated)
	})
	if err != nil {
		t.Fatalf("second SaveManyHolidaysTx() error = %v", err)
	}

	got, err := db.GetHolidays(ctx, 2024, loc)
	if err != nil {
		t.Fatalf("GetHolidays() error = %v", err)
	}

	// Only the new holiday should exist
	if len(got) != 1 {
		t.Errorf("got %d holidays, want 1", len(got))
	}
	if len(got) > 0 && got[0].Title != "New Holiday" {
		t.Errorf("holiday title = %q, want %q", got[0].Title, "New Holiday")
	}
}

func TestSaveManyHolidaysTx_EmptySlice(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return SaveManyHolidaysTx(ctx, tx, nil)
	})
	if err != nil {
		t.Errorf("SaveManyHolidaysTx(nil) error = %v", err)
	}
}

// DateOnly tests

func TestDateOnly_Value(t *testing.T) {
	tests := []struct {
		name    string
		date    *DateOnly
		want    string
		wantErr bool
	}{
		{
			name: "valid date",
			date: dateOnly(2024, 6, 15, time.UTC),
			want: "2024-06-15",
		},
		{
			name:    "nil date",
			date:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.date.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateOnly_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    string
		wantErr bool
	}{
		{
			name:  "nil value",
			value: nil,
			want:  "0001-01-01",
		},
		{
			name:  "time.Time value",
			value: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
			want:  "2024-06-15",
		},
		{
			name:  "string DateOnly format",
			value: "2024-06-15",
			want:  "2024-06-15",
		},
		{
			name:  "string RFC3339 format",
			value: "2024-06-15T10:30:00Z",
			want:  "2024-06-15",
		},
		{
			name:  "[]byte DateOnly format",
			value: []byte("2024-06-15"),
			want:  "2024-06-15",
		},
		{
			name:  "[]byte RFC3339 format",
			value: []byte("2024-06-15T10:30:00Z"),
			want:  "2024-06-15",
		},
		{
			name:    "invalid string",
			value:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "invalid []byte",
			value:   []byte("not-a-date"),
			wantErr: true,
		},
		{
			name:    "unsupported type",
			value:   12345,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d DateOnly
			err := d.Scan(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got := d.String(); got != tt.want {
				t.Errorf("Scan() result = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateOnly_BeforeAfter(t *testing.T) {
	d1 := dateOnly(2024, 1, 15, time.UTC)
	d2 := dateOnly(2024, 6, 15, time.UTC)
	d3 := dateOnly(2024, 1, 15, time.UTC) // same as d1

	if !d1.Before(d2) {
		t.Error("d1 should be before d2")
	}
	if d2.Before(d1) {
		t.Error("d2 should not be before d1")
	}
	if d1.Before(d3) {
		t.Error("d1 should not be before d3 (same date)")
	}

	if !d2.After(d1) {
		t.Error("d2 should be after d1")
	}
	if d1.After(d2) {
		t.Error("d1 should not be after d2")
	}
	if d1.After(d3) {
		t.Error("d1 should not be after d3 (same date)")
	}
}

func TestDateOnly_StartOfYear(t *testing.T) {
	tests := []struct {
		name string
		date *DateOnly
		want string
	}{
		{
			name: "middle of year",
			date: dateOnly(2024, 6, 15, time.UTC),
			want: "2024-01-01",
		},
		{
			name: "start of year",
			date: dateOnly(2024, 1, 1, time.UTC),
			want: "2024-01-01",
		},
		{
			name: "end of year",
			date: dateOnly(2024, 12, 31, time.UTC),
			want: "2024-01-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.StartOfYear(); got != tt.want {
				t.Errorf("StartOfYear() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateOnly_EndOfYear(t *testing.T) {
	tests := []struct {
		name string
		date *DateOnly
		want string
	}{
		{
			name: "middle of year",
			date: dateOnly(2024, 6, 15, time.UTC),
			want: "2024-12-31",
		},
		{
			name: "start of year",
			date: dateOnly(2024, 1, 1, time.UTC),
			want: "2024-12-31",
		},
		{
			name: "end of year",
			date: dateOnly(2024, 12, 31, time.UTC),
			want: "2024-12-31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.EndOfYear(); got != tt.want {
				t.Errorf("EndOfYear() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateOnly_SetLocation(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skip("Europe/Moscow timezone not available")
	}

	d := dateOnly(2024, 6, 15, time.UTC)
	d.SetLocation(loc)

	if d.Time().Location() != loc {
		t.Errorf("location = %v, want %v", d.Time().Location(), loc)
	}

	// Date components should remain the same
	year, month, day := d.Date()
	if year != 2024 || month != 6 || day != 15 {
		t.Errorf("date = %d-%d-%d, want 2024-6-15", year, month, day)
	}
}

func TestDateOnly_Format(t *testing.T) {
	d := dateOnly(2024, 6, 15, time.UTC)

	tests := []struct {
		layout string
		want   string
	}{
		{time.DateOnly, "2024-06-15"},
		{"02 Jan 2006", "15 Jun 2024"},
		{"2006/01/02", "2024/06/15"},
	}

	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			if got := d.Format(tt.layout); got != tt.want {
				t.Errorf("Format(%q) = %v, want %v", tt.layout, got, tt.want)
			}
		})
	}
}

// Event tests

func TestEvent_FloatLoad(t *testing.T) {
	tests := []struct {
		load uint8
		want float64
	}{
		{0, 0.0},
		{50, 50.0},
		{100, 100.0},
		{255, 255.0},
	}

	for _, tt := range tests {
		e := Event{Load: tt.load}
		if got := e.FloatLoad(); got != tt.want {
			t.Errorf("FloatLoad() for load=%d = %v, want %v", tt.load, got, tt.want)
		}
	}
}

func TestEvent_LogValue(t *testing.T) {
	e := Event{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Load:      75,
	}

	got := e.LogValue().String()
	if got == "" {
		t.Error("LogValue() returned empty string")
	}
	// Should contain timestamp and load
	if !contains(got, "2024-01-15") || !contains(got, "75") {
		t.Errorf("LogValue() = %q, should contain date and load", got)
	}
}

func TestHoliday_LogValue(t *testing.T) {
	h := Holiday{
		Day:   dateOnly(2024, 1, 1, time.UTC),
		Title: "New Year",
	}

	got := h.LogValue().String()
	if got == "" {
		t.Error("LogValue() returned empty string")
	}
	if !contains(got, "2024-01-01") || !contains(got, "New Year") {
		t.Errorf("LogValue() = %q, should contain date and title", got)
	}
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	db, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Operations after close should fail
	if err := db.Ping(); err == nil {
		t.Error("Ping() after Close() should fail")
	}
}

// Helper functions

func dateOnly(year int, month time.Month, day int, loc *time.Location) *DateOnly {
	d := DateOnly(time.Date(year, month, day, 0, 0, 0, 0, loc))
	return &d
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
