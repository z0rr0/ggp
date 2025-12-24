package predictor

import (
	"context"
	"testing"
	"time"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
)

func setupTestDB(t *testing.T, ctx context.Context) *databaser.DB {
	t.Helper()

	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	return db
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t, ctx)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	eventCh := make(chan databaser.Event, 1)
	cfg := &config.Config{
		Base: config.Base{
			TimeLocation: time.UTC,
		},
		Predictor: config.Predictor{
			Hours:    24,
			LoadSize: 100,
			Timeout:  3 * time.Second,
		},
	}

	controller, err := Run(ctx, db, eventCh, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if controller == nil {
		t.Fatal("expected non-nil controller")
	}

	if controller.predictor == nil {
		t.Error("predictor not initialized")
	}

	if controller.Hours != 24 {
		t.Errorf("Hours = %d, want 24", controller.Hours)
	}
}

func TestController_Run(t *testing.T) {
	tests := []struct {
		name       string
		sendEvents bool
	}{
		{
			name:       "no event channel",
			sendEvents: false,
		},
		{
			name:       "with event channel",
			sendEvents: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			var eventCh <-chan databaser.Event
			if tt.sendEvents {
				ch := make(chan databaser.Event, 1)
				eventCh = ch
				go func() {
					ch <- databaser.Event{
						Timestamp: time.Now().UTC(),
						Load:      50,
					}
					close(ch)
				}()
			}

			controller := &Controller{
				predictor: New(newMockHolidayChecker()),
				eventCh:   eventCh,
				Hours:     24,
				loadSize:  100,
				timeout:   3 * time.Second,
			}

			doneCh := controller.Run(ctx)

			select {
			case <-doneCh:
			case <-time.After(200 * time.Millisecond):
				t.Error("controller did not stop in time")
			}
		})
	}
}

func TestController_Run_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	eventCh := make(chan databaser.Event)

	controller := &Controller{
		predictor: New(newMockHolidayChecker()),
		eventCh:   eventCh,
		Hours:     24,
		loadSize:  100,
		timeout:   3 * time.Second,
	}

	doneCh := controller.Run(ctx)

	cancel()

	select {
	case <-doneCh:
	case <-time.After(100 * time.Millisecond):
		t.Error("controller did not stop after context cancellation")
	}
}

func TestController_LoadEvents(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t, ctx)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	baseTime := time.Now().UTC()
	events := []databaser.Event{
		{Timestamp: baseTime.Add(-1 * time.Hour), Load: 40},
		{Timestamp: baseTime.Add(-2 * time.Hour), Load: 50},
		{Timestamp: baseTime.Add(-3 * time.Hour), Load: 60},
	}

	for _, event := range events {
		if err := db.SaveEvent(ctx, event); err != nil {
			t.Fatalf("failed to save event: %v", err)
		}
	}

	controller := &Controller{
		predictor: New(newMockHolidayChecker()),
		Hours:     24,
		loadSize:  100,
		timeout:   3 * time.Second,
	}

	if err := controller.LoadEvents(ctx, db); err != nil {
		t.Fatalf("LoadEvents() error = %v", err)
	}

	if len(controller.predictor.recentEvents) != len(events) {
		t.Errorf("loaded events count = %d, want %d", len(controller.predictor.recentEvents), len(events))
	}
}

func TestController_LoadEvents_Empty(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t, ctx)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	controller := &Controller{
		predictor: New(newMockHolidayChecker()),
		Hours:     24,
		loadSize:  100,
		timeout:   3 * time.Second,
	}

	if err := controller.LoadEvents(ctx, db); err != nil {
		t.Fatalf("LoadEvents() error = %v", err)
	}

	if len(controller.predictor.recentEvents) != 0 {
		t.Errorf("loaded events count = %d, want 0", len(controller.predictor.recentEvents))
	}
}

func TestController_PredictLoad(t *testing.T) {
	controller := &Controller{
		predictor: New(newMockHolidayChecker()),
		Hours:     12,
		loadSize:  100,
		timeout:   3 * time.Second,
	}

	baseTime := time.Now().UTC().Truncate(time.Hour)
	for i := range 10 {
		event := databaser.Event{
			Timestamp: baseTime.Add(-time.Duration(i) * time.Hour),
			Load:      50,
		}
		controller.predictor.AddEvent(event)
	}

	tests := []struct {
		name    string
		hours   uint8
		wantLen int
	}{
		{"predict 1 hour", 1, 2},
		{"predict 6 hours", 6, 7},
		{"predict 12 hours", 12, 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := controller.PredictLoad(tt.hours)

			if len(events) != tt.wantLen {
				t.Errorf("PredictLoad() returned %d events, want %d", len(events), tt.wantLen)
			}

			if events[0].Predict < 0 || events[0].Predict > 100 {
				t.Errorf("current load prediction = %v, want 0-100", events[0].Predict)
			}

			for i := 1; i < len(events); i++ {
				if events[i].Predict < 0 || events[i].Predict > 100 {
					t.Errorf("prediction[%d] = %v, want 0-100", i, events[i].Predict)
				}
			}
		})
	}
}

func TestController_PredictLoad_CurrentTime(t *testing.T) {
	controller := &Controller{
		predictor: New(newMockHolidayChecker()),
		Hours:     1,
		loadSize:  100,
		timeout:   3 * time.Second,
	}

	events := controller.PredictLoad(1)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	now := time.Now().UTC()
	if events[0].Timestamp.After(now.Add(time.Minute)) {
		t.Errorf("first event timestamp %v is too far in the future", events[0].Timestamp)
	}

	if events[1].Timestamp.Before(now) {
		t.Errorf("second event timestamp %v is in the past", events[1].Timestamp)
	}
}
