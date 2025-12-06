package predictor

import (
	"math"
	"testing"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

// mockHolidayChecker is a simple holiday checker for testing
type mockHolidayChecker struct {
	holidays map[string]bool
}

func newMockHolidayChecker(dates ...string) *mockHolidayChecker {
	m := &mockHolidayChecker{holidays: make(map[string]bool)}
	for _, d := range dates {
		m.holidays[d] = true
	}
	return m
}

func (m *mockHolidayChecker) IsHoliday(t time.Time) bool {
	key := t.Format("2006-01-02")
	return m.holidays[key]
}

func (m *mockHolidayChecker) HolidayTitle(t time.Time) string {
	if m.IsHoliday(t) {
		return "Test Holiday"
	}
	return ""
}

func TestNew(t *testing.T) {
	checker := newMockHolidayChecker()
	p := New(checker)

	if p == nil {
		t.Fatal("expected non-nil predictor")
	}

	if p.holidayChecker != checker {
		t.Error("holiday checker not set")
	}

	if p.decayLambda != 0.1 {
		t.Errorf("decayLambda = %v, want 0.1", p.decayLambda)
	}

	if p.minWeight != 0.5 {
		t.Errorf("minWeight = %v, want 0.5", p.minWeight)
	}

	if p.maxRecentCount != 40 {
		t.Errorf("maxRecentCount = %d, want 40", p.maxRecentCount)
	}

	if p.confidenceThreshold != 20.0 {
		t.Errorf("confidenceThreshold = %v, want 20.0", p.confidenceThreshold)
	}

	for d := range dayTypesCount {
		for h := range hoursInDay {
			if p.stats[d][h] == nil {
				t.Errorf("stats[%d][%d] is nil", d, h)
			}
		}
	}
}

func TestAddEvent(t *testing.T) {
	tests := []struct {
		name   string
		events []databaser.Event
		want   struct {
			dayType     DayType
			hour        int
			count       uint64
			totalWeight float64
		}
	}{
		{
			name: "single event",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 50}, // Monday
			},
			want: struct {
				dayType     DayType
				hour        int
				count       uint64
				totalWeight float64
			}{DayType(time.Monday), 10, 1, 1.0},
		},
		{
			name: "multiple events same hour",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 40}, // Monday
				{Timestamp: time.Date(2025, 1, 6, 10, 30, 0, 0, time.UTC), Load: 60},
			},
			want: struct {
				dayType     DayType
				hour        int
				count       uint64
				totalWeight float64
			}{DayType(time.Monday), 10, 2, 1.9}, // decay applied due to 30min difference
		},
		{
			name: "events with decay",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 50},  // Monday
				{Timestamp: time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC), Load: 60}, // Monday
			},
			want: struct {
				dayType     DayType
				hour        int
				count       uint64
				totalWeight float64
			}{DayType(time.Monday), 10, 2, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker())

			for _, event := range tt.events {
				p.AddEvent(event)
			}

			stats := p.stats[tt.want.dayType][tt.want.hour]
			if stats.Count != tt.want.count {
				t.Errorf("Count = %d, want %d", stats.Count, tt.want.count)
			}

			if tt.want.totalWeight > 0 && stats.TotalWeight < tt.want.totalWeight {
				t.Errorf("TotalWeight = %v, want >= %v", stats.TotalWeight, tt.want.totalWeight)
			}

			if len(p.recentEvents) != len(tt.events) {
				t.Errorf("recentEvents length = %d, want %d", len(p.recentEvents), len(tt.events))
			}
		})
	}
}

func TestAddEvent_RecentEventsLimit(t *testing.T) {
	p := New(newMockHolidayChecker())
	p.maxRecentCount = 10

	baseTime := time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC)
	for i := range 15 {
		event := databaser.Event{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Load:      uint8(i),
		}
		p.AddEvent(event)
	}

	if len(p.recentEvents) != p.maxRecentCount {
		t.Errorf("recentEvents length = %d, want %d", len(p.recentEvents), p.maxRecentCount)
	}

	if p.recentEvents[0].Load != 5 {
		t.Errorf("first event load = %d, want 5", p.recentEvents[0].Load)
	}

	if p.recentEvents[9].Load != 14 {
		t.Errorf("last event load = %d, want 14", p.recentEvents[9].Load)
	}
}

func TestPredict(t *testing.T) {
	tests := []struct {
		name       string
		hoursAhead uint8
		events     []databaser.Event
		holidays   []string
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "no data - fallback",
			hoursAhead: 1,
			events:     nil,
			wantMin:    0.0,
			wantMax:    100.0,
		},
		{
			name:       "with historical data",
			hoursAhead: 1,
			events: []databaser.Event{
				{Timestamp: time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Hour), Load: 50},
				{Timestamp: time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Hour).Add(30 * time.Minute), Load: 55},
			},
			wantMin: 40.0,
			wantMax: 65.0,
		},
		{
			name:       "holiday prediction",
			hoursAhead: 24,
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), Load: 30},
			},
			holidays: []string{time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")},
			wantMin:  0.0,
			wantMax:  100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker(tt.holidays...))

			for _, event := range tt.events {
				p.AddEvent(event)
			}

			prediction := p.Predict(tt.hoursAhead)

			if prediction.Load < tt.wantMin || prediction.Load > tt.wantMax {
				t.Errorf("Load = %v, want between %v and %v", prediction.Load, tt.wantMin, tt.wantMax)
			}

			if prediction.Confidence < 0.0 || prediction.Confidence > 1.0 {
				t.Errorf("Confidence = %v, want between 0.0 and 1.0", prediction.Confidence)
			}

			expectedTime := time.Now().UTC().Add(time.Duration(tt.hoursAhead) * time.Hour)
			if math.Abs(prediction.TargetTime.Sub(expectedTime).Minutes()) > 1 {
				t.Errorf("TargetTime diff too large: %v", prediction.TargetTime.Sub(expectedTime))
			}
		})
	}
}

func TestPredictRange(t *testing.T) {
	p := New(newMockHolidayChecker())
	baseTime := time.Now().UTC().Truncate(time.Hour)

	for i := range 10 {
		event := databaser.Event{
			Timestamp: baseTime.Add(-time.Duration(i) * time.Hour),
			Load:      50,
		}
		p.AddEvent(event)
	}

	tests := []struct {
		name     string
		maxHours uint8
		wantLen  int
	}{
		{"1 hour", 1, 1},
		{"12 hours", 12, 12},
		{"24 hours", 24, 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predictions := p.PredictRange(tt.maxHours)

			if len(predictions) != tt.wantLen {
				t.Errorf("len(predictions) = %d, want %d", len(predictions), tt.wantLen)
			}

			for i, pred := range predictions {
				if pred.Load < 0.0 || pred.Load > 100.0 {
					t.Errorf("predictions[%d].Load = %v, want 0-100", i, pred.Load)
				}
			}
		})
	}
}

func TestGetTypicalLoad(t *testing.T) {
	tests := []struct {
		name     string
		events   []databaser.Event
		testTime time.Time
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "no data - returns fallback",
			events:   nil,
			testTime: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC),
			wantMin:  averageLoad - 1,
			wantMax:  averageLoad + 1,
		},
		{
			name: "with data - returns weighted average",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 50},
				{Timestamp: time.Date(2025, 1, 6, 10, 30, 0, 0, time.UTC), Load: 60},
			},
			testTime: time.Date(2025, 1, 6, 10, 15, 0, 0, time.UTC),
			wantMin:  50.0,
			wantMax:  60.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker())

			for _, event := range tt.events {
				p.AddEvent(event)
			}

			load := p.GetTypicalLoad(tt.testTime)

			if load < tt.wantMin || load > tt.wantMax {
				t.Errorf("GetTypicalLoad() = %v, want between %v and %v", load, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestGetDayType(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		holidays []string
		want     DayType
	}{
		{
			name: "Monday",
			time: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), // Monday
			want: DayType(time.Monday),
		},
		{
			name: "Sunday",
			time: time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC), // Sunday
			want: DayType(time.Sunday),
		},
		{
			name:     "Holiday",
			time:     time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), // Wednesday but a holiday
			holidays: []string{"2025-01-01"},
			want:     Holiday,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker(tt.holidays...))
			got := p.getDayType(tt.time)

			if got != tt.want {
				t.Errorf("getDayType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateTrend(t *testing.T) {
	tests := []struct {
		name   string
		events []databaser.Event
		want   float64
	}{
		{
			name:   "no events",
			events: nil,
			want:   0,
		},
		{
			name: "insufficient events",
			events: []databaser.Event{
				{Timestamp: time.Now().UTC(), Load: 50},
			},
			want: 0,
		},
		{
			name: "increasing trend",
			events: []databaser.Event{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Load: 40},
				{Timestamp: time.Now().UTC().Add(-1 * time.Hour), Load: 50},
				{Timestamp: time.Now().UTC(), Load: 60},
			},
			want: 10.0,
		},
		{
			name: "decreasing trend",
			events: []databaser.Event{
				{Timestamp: time.Now().UTC().Add(-2 * time.Hour), Load: 60},
				{Timestamp: time.Now().UTC().Add(-1 * time.Hour), Load: 50},
				{Timestamp: time.Now().UTC(), Load: 40},
			},
			want: -10.0,
		},
		{
			name: "same interval too small",
			events: []databaser.Event{
				{Timestamp: time.Now().UTC(), Load: 40},
				{Timestamp: time.Now().UTC().Add(1 * time.Minute), Load: 50},
				{Timestamp: time.Now().UTC().Add(2 * time.Minute), Load: 60},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker())
			p.recentEvents = tt.events

			got := p.calculateTrend()

			if tt.name == "increasing trend" || tt.name == "decreasing trend" {
				if math.Abs(got-tt.want) > 1.0 {
					t.Errorf("calculateTrend() = %v, want ~%v", got, tt.want)
				}
			} else {
				if got != tt.want {
					t.Errorf("calculateTrend() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestFallbackPrediction(t *testing.T) {
	tests := []struct {
		name      string
		events    []databaser.Event
		dayOfWeek int
		wantMin   float64
		wantMax   float64
	}{
		{
			name:      "no data - returns average",
			events:    nil,
			dayOfWeek: int(time.Monday),
			wantMin:   averageLoad,
			wantMax:   averageLoad,
		},
		{
			name: "with data - returns day average",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 50}, // Monday
				{Timestamp: time.Date(2025, 1, 6, 14, 0, 0, 0, time.UTC), Load: 60}, // Monday
			},
			dayOfWeek: int(time.Monday),
			wantMin:   50.0,
			wantMax:   60.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker())

			for _, event := range tt.events {
				p.AddEvent(event)
			}

			got := p.fallbackPrediction(tt.dayOfWeek)

			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("fallbackPrediction() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name        string
		stats       *HourlyStats
		dayType     DayType
		wantMin     float64
		wantMax     float64
		wantPenalty bool
	}{
		{
			name: "high weight - high confidence",
			stats: &HourlyStats{
				TotalWeight: 20.0,
				LastUpdate:  time.Now().UTC(),
			},
			dayType: DayType(time.Monday),
			wantMin: 0.8,
			wantMax: 1.0,
		},
		{
			name: "low weight - low confidence",
			stats: &HourlyStats{
				TotalWeight: 5.0,
				LastUpdate:  time.Now().UTC(),
			},
			dayType: DayType(time.Monday),
			wantMin: 0.2,
			wantMax: 0.3,
		},
		{
			name: "holiday penalty",
			stats: &HourlyStats{
				TotalWeight: 20.0,
				LastUpdate:  time.Now().UTC(),
			},
			dayType:     Holiday,
			wantMin:     0.5,
			wantMax:     0.8,
			wantPenalty: true,
		},
		{
			name: "stale data penalty",
			stats: &HourlyStats{
				TotalWeight: 20.0,
				LastUpdate:  time.Now().UTC().Add(-30 * 24 * time.Hour),
			},
			dayType: DayType(time.Monday),
			wantMin: 0.1,
			wantMax: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker())
			got := p.calculateConfidence(tt.stats, tt.dayType)

			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("calculateConfidence() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}

			if got < 0.0 || got > 1.0 {
				t.Errorf("calculateConfidence() = %v, want between 0.0 and 1.0", got)
			}
		})
	}
}

func TestGetWeightedAverage(t *testing.T) {
	tests := []struct {
		name    string
		events  []databaser.Event
		dayType DayType
		hour    int
		want    float64
	}{
		{
			name:    "no data - returns average",
			events:  nil,
			dayType: DayType(time.Monday),
			hour:    10,
			want:    averageLoad,
		},
		{
			name: "with data - returns weighted average",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 50},  // Monday
				{Timestamp: time.Date(2025, 1, 6, 10, 30, 0, 0, time.UTC), Load: 60}, // Monday
			},
			dayType: DayType(time.Monday),
			hour:    10,
			want:    55.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker())

			for _, event := range tt.events {
				p.AddEvent(event)
			}

			got := p.getWeightedAverage(tt.dayType, tt.hour)

			if math.Abs(got-tt.want) > 1.0 {
				t.Errorf("getWeightedAverage() = %v, want ~%v", got, tt.want)
			}
		})
	}
}

func TestPredictWithBlending(t *testing.T) {
	tests := []struct {
		name       string
		events     []databaser.Event
		targetTime time.Time
		holidays   []string
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "weekday - no blending",
			events:     []databaser.Event{{Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Load: 50}},
			targetTime: time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC),
			wantMin:    40.0,
			wantMax:    60.0,
		},
		{
			name: "holiday - blends with Sunday",
			events: []databaser.Event{
				{Timestamp: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), Load: 30},
				{Timestamp: time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC), Load: 40},
			},
			targetTime: time.Date(2025, 1, 9, 10, 0, 0, 0, time.UTC),
			holidays:   []string{"2025-01-09"},
			wantMin:    25.0,
			wantMax:    45.0,
		},
		{
			name:       "no data - returns average",
			events:     nil,
			targetTime: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC),
			wantMin:    averageLoad,
			wantMax:    averageLoad,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(newMockHolidayChecker(tt.holidays...))

			for _, event := range tt.events {
				p.AddEvent(event)
			}

			got := p.predictWithBlending(tt.targetTime, tt.targetTime.Hour())

			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("predictWithBlending() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	p := New(newMockHolidayChecker())
	done := make(chan bool)
	baseTime := time.Now().UTC()

	// Writer goroutine
	go func() {
		for i := range 100 {
			event := databaser.Event{
				Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
				Load:      uint8(i % 100),
			}
			p.AddEvent(event)
		}
		done <- true
	}()

	// Reader goroutines
	for range 3 {
		go func() {
			for range 50 {
				_ = p.Predict(1)
				_ = p.GetTypicalLoad(baseTime)
			}
			done <- true
		}()
	}

	for range 4 {
		<-done
	}
}

func TestString(t *testing.T) {
	p := New(newMockHolidayChecker())
	event := databaser.Event{
		Timestamp: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC),
		Load:      50,
	}
	p.AddEvent(event)

	str := p.String()
	if str == "" {
		t.Error("String() returned empty string")
	}

	if len(str) < 100 {
		t.Error("String() output seems too short")
	}
}

func BenchmarkAddEvent(b *testing.B) {
	p := New(newMockHolidayChecker())
	baseTime := time.Now().UTC()

	b.ResetTimer()
	for i := range b.N {
		event := databaser.Event{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Load:      uint8(i % 100),
		}
		p.AddEvent(event)
	}
}

func BenchmarkPredict(b *testing.B) {
	p := New(newMockHolidayChecker())
	baseTime := time.Now().UTC()

	for i := range 1000 {
		event := databaser.Event{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Load:      uint8(i % 100),
		}
		p.AddEvent(event)
	}

	b.ResetTimer()
	for range b.N {
		_ = p.Predict(1)
	}
}

func BenchmarkPredictRange(b *testing.B) {
	p := New(newMockHolidayChecker())
	baseTime := time.Now().UTC()

	for i := range 1000 {
		event := databaser.Event{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Load:      uint8(i % 100),
		}
		p.AddEvent(event)
	}

	b.ResetTimer()
	for range b.N {
		_ = p.PredictRange(24)
	}
}
