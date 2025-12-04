package predictor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
)

type Controller struct {
	predictor *Predictor
	eventCh   <-chan databaser.Event
	Hours     uint8
}

func Run(ctx context.Context, db *databaser.DB, eventCh <-chan databaser.Event, cfg *config.Config) (*Controller, error) {
	holidayChecker, err := NewRussianHolidayChecker(ctx, db, cfg.Base.TimeLocation)
	if err != nil {
		return nil, fmt.Errorf("NewRussianHolidayChecker: %w", err)
	}

	controller := &Controller{
		predictor: New(holidayChecker),
		eventCh:   eventCh,
		Hours:     cfg.Predictor.Hours,
	}

	// load events from the database
	if err = controller.LoadEvents(ctx, db); err != nil {
		return nil, fmt.Errorf("LoadEvents: %w", err)
	}

	return controller, nil
}

// Run starts the controller to listen for events and process them.
func (c *Controller) Run(ctx context.Context) <-chan struct{} {
	doneCh := make(chan struct{})
	if c.eventCh == nil {
		slog.InfoContext(ctx, "no event channel provided, predictor controller will not run")
		close(doneCh)
		return doneCh
	}

	go func() {
		defer close(doneCh)
		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "stopping predictor controller")
				return
			case event, ok := <-c.eventCh:
				if !ok {
					slog.InfoContext(ctx, "event channel closed, stopping predictor controller")
					return
				}
				slog.DebugContext(ctx, "predictor received event", "event", event)
				c.predictor.AddEvent(event)
			}
		}
	}()
	return doneCh
}

// LoadEvents loads historical events from the database into the predictor.
func (c *Controller) LoadEvents(ctx context.Context, db *databaser.DB) error {
	const limit = 1000
	var n, offset int

	for {
		events, err := db.GetAllEvents(ctx, limit, offset)
		if err != nil {
			return fmt.Errorf("GetAllEvents: %w", err)
		}

		if n = len(events); n == 0 {
			break
		}
		slog.DebugContext(ctx, "got events", "events", n)

		for _, event := range events {
			c.predictor.AddEvent(event)
		}

		offset += n
		slog.DebugContext(ctx, "add events", "offset", offset)
	}

	slog.InfoContext(ctx, "predictor loaded events")
	return nil
}

// PredictLoad generates load predictions for the configured number of hours.
func (c *Controller) PredictLoad(hours uint8) []databaser.Event {
	now := time.Now().UTC()
	predictions := c.predictor.PredictRange(hours)
	events := make([]databaser.Event, 0, len(predictions)+1)

	events = append(events, databaser.Event{Timestamp: now, Predict: c.predictor.GetTypicalLoad(now)})
	for _, p := range predictions {
		events = append(events, databaser.Event{Timestamp: p.TargetTime, Predict: p.Load})
	}

	return events
}
