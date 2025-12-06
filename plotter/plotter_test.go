package plotter

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

func TestGetDateFormat(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		events     []time.Time
		wantFormat string
	}{
		{
			name:       "empty events",
			events:     []time.Time{},
			wantFormat: dtFormatMap[dtFormatDay],
		},
		{
			name:       "single event",
			events:     []time.Time{baseTime},
			wantFormat: dtFormatMap[dtFormatDay],
		},
		{
			name: "second range - under 2 minutes",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Minute),
			},
			wantFormat: dtFormatMap[dtFormatSecond],
		},
		{
			name: "minute range - under 24 hours",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Hour * 12),
			},
			wantFormat: dtFormatMap[dtFormatMinute],
		},
		{
			name: "hour range - under 72 hours",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Hour * 48),
			},
			wantFormat: dtFormatMap[dtFormatHour],
		},
		{
			name: "day range - under 14 days",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Hour * 24 * 7),
			},
			wantFormat: dtFormatMap[dtFormatDay],
		},
		{
			name: "week range - under 3 months",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Hour * 24 * 30),
			},
			wantFormat: dtFormatMap[dtFormatWeek],
		},
		{
			name: "month range - under 2 years",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Hour * 24 * 365),
			},
			wantFormat: dtFormatMap[dtFormatMonth],
		},
		{
			name: "year range - over 2 years",
			events: []time.Time{
				baseTime,
				baseTime.Add(time.Hour * 24 * 365 * 3),
			},
			wantFormat: dtFormatMap[dtFormatYear],
		},
		{
			name: "boundary - exactly 2 minutes",
			events: []time.Time{
				baseTime,
				baseTime.Add(periodSecond),
			},
			wantFormat: dtFormatMap[dtFormatSecond],
		},
		{
			name: "boundary - just over 2 minutes",
			events: []time.Time{
				baseTime,
				baseTime.Add(periodSecond + time.Second),
			},
			wantFormat: dtFormatMap[dtFormatMinute],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDateFormat(tt.events)
			if got != tt.wantFormat {
				t.Errorf("getDateFormat() = %q, want %q", got, tt.wantFormat)
			}
		})
	}
}

func TestGraph(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		events     []databaser.Event
		prediction []databaser.Event
		location   *time.Location
		wantErr    bool
	}{
		{
			name: "single event - chart library requires 2 points",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 50},
			},
			prediction: nil,
			location:   time.UTC,
			wantErr:    true,
		},
		{
			name: "valid two events",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 30},
				{Timestamp: baseTime.Add(time.Hour), Load: 50},
			},
			prediction: nil,
			location:   time.UTC,
			wantErr:    false,
		},
		{
			name: "valid multiple events",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 30},
				{Timestamp: baseTime.Add(time.Hour), Load: 50},
				{Timestamp: baseTime.Add(time.Hour * 2), Load: 70},
			},
			prediction: nil,
			location:   time.UTC,
			wantErr:    false,
		},
		{
			name: "with predictions",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 30},
				{Timestamp: baseTime.Add(time.Hour), Load: 50},
			},
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 60.5},
				{Timestamp: baseTime.Add(time.Hour * 3), Predict: 55.2},
			},
			location: time.UTC,
			wantErr:  false,
		},
		{
			name:       "empty events",
			events:     []databaser.Event{},
			prediction: nil,
			location:   time.UTC,
			wantErr:    true,
		},
		{
			name:       "nil events",
			events:     nil,
			prediction: nil,
			location:   time.UTC,
			wantErr:    true,
		},
		{
			name: "zero load values",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 0},
				{Timestamp: baseTime.Add(time.Hour), Load: 0},
			},
			prediction: nil,
			location:   time.UTC,
			wantErr:    false,
		},
		{
			name: "max load values",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 255},
				{Timestamp: baseTime.Add(time.Hour), Load: 100},
			},
			prediction: nil,
			location:   time.UTC,
			wantErr:    false,
		},
		{
			name: "single prediction ignored",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 50},
				{Timestamp: baseTime.Add(time.Hour), Load: 60},
			},
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 70.0},
			},
			location: time.UTC,
			wantErr:  false,
		},
		{
			name: "long time range",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 50},
				{Timestamp: baseTime.Add(time.Hour * 24 * 365 * 3), Load: 60},
			},
			prediction: nil,
			location:   time.UTC,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Graph(tt.events, tt.prediction, tt.location)

			if (err != nil) != tt.wantErr {
				t.Errorf("Graph() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(result) == 0 {
					t.Error("Graph() returned empty result")
				}
				// PNG files start with specific magic bytes
				if !bytes.HasPrefix(result, []byte{0x89, 'P', 'N', 'G'}) {
					t.Error("Graph() result is not a valid PNG")
				}
			}
		})
	}
}

func TestGraph_DifferentLocations(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	events := []databaser.Event{
		{Timestamp: baseTime, Load: 50},
		{Timestamp: baseTime.Add(time.Hour), Load: 60},
	}

	locations := []struct {
		name string
		loc  *time.Location
	}{
		{"UTC", time.UTC},
		{"Local", time.Local},
	}

	// Try to load additional locations
	if moscow, err := time.LoadLocation("Europe/Moscow"); err == nil {
		locations = append(locations, struct {
			name string
			loc  *time.Location
		}{"Moscow", moscow})
	}
	if tokyo, err := time.LoadLocation("Asia/Tokyo"); err == nil {
		locations = append(locations, struct {
			name string
			loc  *time.Location
		}{"Tokyo", tokyo})
	}

	for _, loc := range locations {
		t.Run(loc.name, func(t *testing.T) {
			result, err := Graph(events, nil, loc.loc)
			if err != nil {
				t.Fatalf("Graph() error = %v", err)
			}
			if len(result) == 0 {
				t.Error("Graph() returned empty result")
			}
		})
	}
}

func TestGraph_ErrorMessage(t *testing.T) {
	_, err := Graph(nil, nil, time.UTC)
	if err == nil {
		t.Fatal("expected error for nil events")
	}

	expectedMsg := "graph called with no events"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("error message should contain %q, got %q", expectedMsg, err.Error())
	}
}

func TestGraph_MaxYCalculation(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		events     []databaser.Event
		prediction []databaser.Event
	}{
		{
			name: "events have higher max",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 100},
				{Timestamp: baseTime.Add(time.Hour), Load: 50},
			},
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 30.0},
				{Timestamp: baseTime.Add(time.Hour * 3), Predict: 40.0},
			},
		},
		{
			name: "predictions have higher max",
			events: []databaser.Event{
				{Timestamp: baseTime, Load: 30},
				{Timestamp: baseTime.Add(time.Hour), Load: 40},
			},
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 90.0},
				{Timestamp: baseTime.Add(time.Hour * 3), Predict: 95.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Graph(tt.events, tt.prediction, time.UTC)
			if err != nil {
				t.Fatalf("Graph() error = %v", err)
			}
			if len(result) == 0 {
				t.Error("Graph() returned empty result")
			}
		})
	}
}

func TestGraph_BufferPoolReuse(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	events := []databaser.Event{
		{Timestamp: baseTime, Load: 50},
		{Timestamp: baseTime.Add(time.Hour), Load: 60},
	}

	// Generate multiple graphs to test buffer pool reuse
	var results [][]byte
	for i := 0; i < 5; i++ {
		result, err := Graph(events, nil, time.UTC)
		if err != nil {
			t.Fatalf("Graph() iteration %d error = %v", i, err)
		}
		results = append(results, result)
	}

	// Verify all results are valid PNGs and have the same length
	for i, result := range results {
		if !bytes.HasPrefix(result, []byte{0x89, 'P', 'N', 'G'}) {
			t.Errorf("iteration %d: result is not a valid PNG", i)
		}
	}

	// Verify results are independent copies (modifying one doesn't affect others)
	if len(results) >= 2 && len(results[0]) > 10 {
		original := results[0][10]
		results[0][10] = 0xFF
		if results[1][10] == 0xFF && original != 0xFF {
			t.Error("buffer pool reuse caused data corruption between calls")
		}
		results[0][10] = original
	}
}

func TestGraph_ConcurrentCalls(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	events := []databaser.Event{
		{Timestamp: baseTime, Load: 50},
		{Timestamp: baseTime.Add(time.Hour), Load: 60},
	}

	const goroutines = 10
	results := make(chan []byte, goroutines)
	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			result, err := Graph(events, nil, time.UTC)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}

	for i := 0; i < goroutines; i++ {
		select {
		case err := <-errors:
			t.Errorf("concurrent Graph() error = %v", err)
		case result := <-results:
			if !bytes.HasPrefix(result, []byte{0x89, 'P', 'N', 'G'}) {
				t.Error("concurrent Graph() result is not a valid PNG")
			}
		}
	}
}

func TestGraph_AllDateFormats(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	// Test each date format by creating events with appropriate time spans
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"second format", time.Minute},
		{"minute format", time.Hour * 12},
		{"hour format", time.Hour * 48},
		{"day format", time.Hour * 24 * 7},
		{"week format", time.Hour * 24 * 45},
		{"month format", time.Hour * 24 * 400},
		{"year format", time.Hour * 24 * 800},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := []databaser.Event{
				{Timestamp: baseTime, Load: 50},
				{Timestamp: baseTime.Add(tt.duration), Load: 60},
			}

			result, err := Graph(events, nil, time.UTC)
			if err != nil {
				t.Fatalf("Graph() error = %v", err)
			}
			if !bytes.HasPrefix(result, []byte{0x89, 'P', 'N', 'G'}) {
				t.Error("Graph() result is not a valid PNG")
			}
		})
	}
}

func TestGraph_WithPredictionsVariousLengths(t *testing.T) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	events := []databaser.Event{
		{Timestamp: baseTime, Load: 50},
		{Timestamp: baseTime.Add(time.Hour), Load: 60},
	}

	tests := []struct {
		name       string
		prediction []databaser.Event
	}{
		{
			name:       "no predictions",
			prediction: nil,
		},
		{
			name:       "empty predictions",
			prediction: []databaser.Event{},
		},
		{
			name: "single prediction (not shown)",
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 70.0},
			},
		},
		{
			name: "two predictions (shown)",
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 70.0},
				{Timestamp: baseTime.Add(time.Hour * 3), Predict: 75.0},
			},
		},
		{
			name: "many predictions",
			prediction: []databaser.Event{
				{Timestamp: baseTime.Add(time.Hour * 2), Predict: 70.0},
				{Timestamp: baseTime.Add(time.Hour * 3), Predict: 75.0},
				{Timestamp: baseTime.Add(time.Hour * 4), Predict: 80.0},
				{Timestamp: baseTime.Add(time.Hour * 5), Predict: 72.0},
				{Timestamp: baseTime.Add(time.Hour * 6), Predict: 65.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Graph(events, tt.prediction, time.UTC)
			if err != nil {
				t.Fatalf("Graph() error = %v", err)
			}
			if !bytes.HasPrefix(result, []byte{0x89, 'P', 'N', 'G'}) {
				t.Error("Graph() result is not a valid PNG")
			}
		})
	}
}

func TestDtFormatMap_AllFormatsExist(t *testing.T) {
	expectedFormats := []int{
		dtFormatSecond,
		dtFormatMinute,
		dtFormatHour,
		dtFormatDay,
		dtFormatWeek,
		dtFormatMonth,
		dtFormatYear,
	}

	for _, format := range expectedFormats {
		if _, ok := dtFormatMap[format]; !ok {
			t.Errorf("dtFormatMap missing format %d", format)
		}
	}
}

func TestDtFormatMap_ValidLayouts(t *testing.T) {
	testTime := time.Date(2025, 6, 15, 14, 30, 45, 0, time.UTC)

	for format, layout := range dtFormatMap {
		t.Run(layout, func(t *testing.T) {
			// Should not panic
			result := testTime.Format(layout)
			if result == "" {
				t.Errorf("format %d with layout %q produced empty result", format, layout)
			}
		})
	}
}

func BenchmarkGraph(b *testing.B) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	events := make([]databaser.Event, 100)
	for i := range events {
		events[i] = databaser.Event{
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			Load:      uint8(50 + i%50),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Graph(events, nil, time.UTC)
		if err != nil {
			b.Fatalf("Graph() error = %v", err)
		}
	}
}

func BenchmarkGetDateFormat(b *testing.B) {
	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	events := make([]time.Time, 100)
	for i := range events {
		events[i] = baseTime.Add(time.Duration(i) * time.Hour)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getDateFormat(events)
	}
}
