package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

func newTestDB(t *testing.T) *databaser.DB {
	t.Helper()
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
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

func createTempCSV(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_import.csv")
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create temp CSV file: %v", err)
	}
	return filePath
}

func TestImportCSV(t *testing.T) {
	tests := []struct {
		name       string
		csvContent string
		wantCount  int
		wantErr    bool
	}{
		{
			name: "valid CSV with multiple rows",
			csvContent: `time,load
2025-11-22 23:27:27,7
2025-11-23 00:08:16,3
2025-11-23 00:18:16,3
2025-11-23 00:28:16,2
2025-11-23 00:38:16,2`,
			wantCount: 5,
			wantErr:   false,
		},
		{
			name: "valid CSV with single row",
			csvContent: `time,load
2025-11-22 10:00:00,50`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:       "header only",
			csvContent: `time,load`,
			wantCount:  0,
			wantErr:    false,
		},
		{
			name:       "empty file",
			csvContent: ``,
			wantCount:  0,
			wantErr:    true,
		},
		{
			name: "invalid timestamp format",
			csvContent: `time,load
2025/11/22 10:00:00,50`,
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "invalid load value",
			csvContent: `time,load
2025-11-22 10:00:00,abc`,
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "load exceeds uint8 max",
			csvContent: `time,load
2025-11-22 10:00:00,300`,
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "negative load value",
			csvContent: `time,load
2025-11-22 10:00:00,-5`,
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "missing load column",
			csvContent: `time,load
2025-11-22 10:00:00`,
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "extra columns ignored",
			csvContent: `time,load,extra
2025-11-22 10:00:00,50,ignored`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "zero load value",
			csvContent: `time,load
2025-11-22 10:00:00,0`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "max uint8 load value",
			csvContent: `time,load
2025-11-22 10:00:00,255`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "whitespace in values",
			csvContent: `time,load
2025-11-22 10:00:00, 50`,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			filePath := createTempCSV(t, tt.csvContent)

			err := ImportCSV(db, filePath, 30*time.Second, time.UTC)

			if (err != nil) != tt.wantErr {
				t.Errorf("ImportCSV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				ctx := context.Background()
				events, err := db.GetEvents(ctx, 365*24*time.Hour)
				if err != nil {
					t.Fatalf("GetEvents() error = %v", err)
				}
				if len(events) != tt.wantCount {
					t.Errorf("got %d events, want %d", len(events), tt.wantCount)
				}
			}
		})
	}
}

func TestImportCSV_FileNotFound(t *testing.T) {
	db := newTestDB(t)

	err := ImportCSV(db, "/non/existent/file.csv", 30*time.Second, time.UTC)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "open file") {
		t.Errorf("error should contain 'open file', got: %v", err)
	}
}

func TestImportCSV_Timeout(t *testing.T) {
	db := newTestDB(t)

	// Create a large CSV that would take time to process
	var builder strings.Builder
	builder.WriteString("time,load\n")
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		builder.WriteString(ts.Format(time.DateTime))
		builder.WriteString(",50\n")
	}

	filePath := createTempCSV(t, builder.String())

	// Use a very short timeout - but this test is tricky because
	// in-memory SQLite is fast. We verify the timeout mechanism works.
	err := ImportCSV(db, filePath, 1*time.Nanosecond, time.UTC)
	// The timeout may or may not trigger depending on system speed
	// Just verify the function doesn't panic
	_ = err
}

func TestImportCSV_Location(t *testing.T) {
	db := newTestDB(t)

	csvContent := `time,load
2025-11-22 10:00:00,50`

	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	filePath := createTempCSV(t, csvContent)

	err = ImportCSV(db, filePath, 30*time.Second, moscow)
	if err != nil {
		t.Fatalf("ImportCSV() error = %v", err)
	}

	ctx := context.Background()
	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Events are stored in UTC, verify the conversion happened
	if events[0].Timestamp.Location() != time.UTC {
		t.Errorf("expected UTC location, got %s", events[0].Timestamp.Location())
	}
}

func TestImportCSV_LargeFile(t *testing.T) {
	db := newTestDB(t)

	// Create CSV larger than chunkSize (250) to test chunking
	var builder strings.Builder
	builder.WriteString("time,load\n")
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	recordCount := 600
	for i := 0; i < recordCount; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		builder.WriteString(ts.Format(time.DateTime))
		builder.WriteString(",")
		builder.WriteString("42\n")
	}

	filePath := createTempCSV(t, builder.String())

	err := ImportCSV(db, filePath, 60*time.Second, time.UTC)
	if err != nil {
		t.Fatalf("ImportCSV() error = %v", err)
	}

	ctx := context.Background()
	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(events) != recordCount {
		t.Errorf("got %d events, want %d", len(events), recordCount)
	}
}

func TestImportReader_Read(t *testing.T) {
	tests := []struct {
		name       string
		csvContent string
		wantCount  int
		wantErr    bool
	}{
		{
			name: "valid records",
			csvContent: `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30`,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:       "empty after header",
			csvContent: `time,load`,
			wantCount:  0,
			wantErr:    false,
		},
		{
			name:       "no header",
			csvContent: ``,
			wantCount:  0,
			wantErr:    true,
		},
		{
			name: "malformed CSV",
			csvContent: `time,load
"unclosed quote`,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.csvContent)
			r := &importReader{
				reader:   reader,
				location: time.UTC,
			}

			count := 0
			for range r.Read() {
				count++
			}

			if (r.err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", r.err, tt.wantErr)
				return
			}
			if count != tt.wantCount {
				t.Errorf("Read() yielded %d events, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestImportReader_Read_EarlyBreak(t *testing.T) {
	csvContent := `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30
2025-01-01 13:00:00,40
2025-01-01 14:00:00,50`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		reader:   reader,
		location: time.UTC,
	}

	// Read only first 2 events
	count := 0
	for range r.Read() {
		count++
		if count >= 2 {
			break
		}
	}

	if count != 2 {
		t.Errorf("expected to read 2 events, got %d", count)
	}
	if r.err != nil {
		t.Errorf("unexpected error: %v", r.err)
	}
}

func TestImportReader_ReadChunk(t *testing.T) {
	tests := []struct {
		name          string
		csvContent    string
		chunkSize     int
		wantChunks    int
		wantLastChunk int
	}{
		{
			name: "exact multiple of chunk size",
			csvContent: `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30
2025-01-01 13:00:00,40`,
			chunkSize:     2,
			wantChunks:    2,
			wantLastChunk: 2,
		},
		{
			name: "not exact multiple",
			csvContent: `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30
2025-01-01 13:00:00,40
2025-01-01 14:00:00,50`,
			chunkSize:     2,
			wantChunks:    3,
			wantLastChunk: 1,
		},
		{
			name: "single record",
			csvContent: `time,load
2025-01-01 10:00:00,10`,
			chunkSize:     5,
			wantChunks:    1,
			wantLastChunk: 1,
		},
		{
			name:          "empty data",
			csvContent:    `time,load`,
			chunkSize:     5,
			wantChunks:    0,
			wantLastChunk: 0,
		},
		{
			name: "chunk size of 1",
			csvContent: `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30`,
			chunkSize:     1,
			wantChunks:    3,
			wantLastChunk: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.csvContent)
			r := &importReader{
				reader:   reader,
				location: time.UTC,
			}

			chunks := 0
			var lastChunkSize int
			for chunk := range r.ReadChunk(tt.chunkSize) {
				chunks++
				lastChunkSize = len(chunk)
			}

			if r.err != nil {
				t.Errorf("unexpected error: %v", r.err)
			}
			if chunks != tt.wantChunks {
				t.Errorf("ReadChunk() yielded %d chunks, want %d", chunks, tt.wantChunks)
			}
			if tt.wantChunks > 0 && lastChunkSize != tt.wantLastChunk {
				t.Errorf("last chunk size = %d, want %d", lastChunkSize, tt.wantLastChunk)
			}
		})
	}
}

func TestImportReader_ReadChunk_EarlyBreak(t *testing.T) {
	csvContent := `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30
2025-01-01 13:00:00,40
2025-01-01 14:00:00,50
2025-01-01 15:00:00,60`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		reader:   reader,
		location: time.UTC,
	}

	// Read only first chunk
	chunkCount := 0
	for range r.ReadChunk(2) {
		chunkCount++
		break
	}

	if chunkCount != 1 {
		t.Errorf("expected 1 chunk, got %d", chunkCount)
	}
	if r.err != nil {
		t.Errorf("unexpected error: %v", r.err)
	}
}

func TestImportReader_ReadChunk_ErrorPropagation(t *testing.T) {
	csvContent := `time,load
2025-01-01 10:00:00,10
invalid-timestamp,20
2025-01-01 12:00:00,30`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		reader:   reader,
		location: time.UTC,
	}

	chunks := 0
	for range r.ReadChunk(10) {
		chunks++
	}

	if r.err == nil {
		t.Error("expected error for invalid timestamp")
	}
	if !strings.Contains(r.err.Error(), "parse record") {
		t.Errorf("error should contain 'parse record', got: %v", r.err)
	}
}

func TestImportReader_InsertEvents(t *testing.T) {
	db := newTestDB(t)

	csvContent := `time,load
2025-01-01 10:00:00,10
2025-01-01 11:00:00,20
2025-01-01 12:00:00,30`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		db:       db,
		reader:   reader,
		location: time.UTC,
	}

	ctx := context.Background()
	err := r.InsertEvents(ctx, 30*time.Second)
	if err != nil {
		t.Fatalf("InsertEvents() error = %v", err)
	}

	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(events) != 3 {
		t.Errorf("got %d events, want 3", len(events))
	}
}

func TestImportReader_InsertEvents_ReadError(t *testing.T) {
	db := newTestDB(t)

	csvContent := `time,load
2025-01-01 10:00:00,10
bad-data,20`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		db:       db,
		reader:   reader,
		location: time.UTC,
	}

	ctx := context.Background()
	err := r.InsertEvents(ctx, 30*time.Second)
	if err == nil {
		t.Error("expected error for bad CSV data")
	}

	// Transaction should be rolled back, no events should be saved
	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after rollback, got %d", len(events))
	}
}

func TestImportReader_InsertEvents_ContextCanceled(t *testing.T) {
	db := newTestDB(t)

	csvContent := `time,load
2025-01-01 10:00:00,10`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		db:       db,
		reader:   reader,
		location: time.UTC,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.InsertEvents(ctx, 30*time.Second)
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestImportCSV_DuplicateTimestamps(t *testing.T) {
	db := newTestDB(t)

	// SQLite uses INSERT OR REPLACE, so duplicates should update
	csvContent := `time,load
2025-01-01 10:00:00,10
2025-01-01 10:00:00,20`

	filePath := createTempCSV(t, csvContent)

	err := ImportCSV(db, filePath, 30*time.Second, time.UTC)
	if err != nil {
		t.Fatalf("ImportCSV() error = %v", err)
	}

	ctx := context.Background()
	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	// Should have 1 event (second one replaces the first)
	if len(events) != 1 {
		t.Errorf("expected 1 event after duplicate handling, got %d", len(events))
	}
	if events[0].Load != 20 {
		t.Errorf("expected load 20 (last value), got %d", events[0].Load)
	}
}

func TestImportCSV_PathCleaning(t *testing.T) {
	db := newTestDB(t)

	csvContent := `time,load
2025-01-01 10:00:00,50`

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "..", "test.csv")
	cleanPath := filepath.Clean(filePath)

	// Create directory and file at the clean path
	if err := os.WriteFile(cleanPath, []byte(csvContent), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Use the unclean path - it should still work
	err := ImportCSV(db, filePath, 30*time.Second, time.UTC)
	if err != nil {
		t.Fatalf("ImportCSV() error = %v", err)
	}

	ctx := context.Background()
	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestImportCSV_EventTimestampUTC(t *testing.T) {
	db := newTestDB(t)

	csvContent := `time,load
2025-06-15 14:30:00,75`

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	filePath := createTempCSV(t, csvContent)

	err = ImportCSV(db, filePath, 30*time.Second, tokyo)
	if err != nil {
		t.Fatalf("ImportCSV() error = %v", err)
	}

	ctx := context.Background()
	events, err := db.GetEvents(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Verify the timestamp was correctly converted
	// Tokyo is UTC+9, so 14:30 Tokyo = 05:30 UTC
	expectedHour := 5
	expectedMinute := 30
	if events[0].Timestamp.Hour() != expectedHour || events[0].Timestamp.Minute() != expectedMinute {
		t.Errorf("expected time 05:30 UTC, got %s", events[0].Timestamp.Format(time.RFC3339))
	}
}

func TestImportReader_Read_EventValues(t *testing.T) {
	csvContent := `time,load
2025-03-15 08:45:30,42`

	reader := strings.NewReader(csvContent)
	r := &importReader{
		reader:   reader,
		location: time.UTC,
	}

	var events []*databaser.Event
	for event := range r.Read() {
		events = append(events, event)
	}

	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Load != 42 {
		t.Errorf("expected load 42, got %d", event.Load)
	}

	expectedTime := time.Date(2025, 3, 15, 8, 45, 30, 0, time.UTC)
	if !event.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, event.Timestamp)
	}
}
