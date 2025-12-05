// Package fetcher implements a service that periodically fetches club load data.
package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

// Club represents the JSON structure of the club data returned by the API.
type Club struct {
	Title       string `json:"title"`
	CurrentLoad string `json:"currentLoad"`  //nolint:tagliatelle
	ID          int    `json:"id"`
}

// Fetcher struct holds the configuration for the fetcher.
type Fetcher struct {
	Db           *databaser.DB
	Client       *http.Client
	URL          string
	Token        string
	Timeout      time.Duration
	QueryTimeout time.Duration
}

// Run begins the periodic fetching process.
func (f *Fetcher) Run(ctx context.Context) (<-chan struct{}, <-chan databaser.Event, error) {
	eventCh := make(chan databaser.Event, 1)
	err := f.Fetch(ctx, eventCh)
	if err != nil {
		close(eventCh)
		return nil, nil, fmt.Errorf("initial fetch: %w", err)
	}

	doneCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(f.Timeout)
		defer func() {
			ticker.Stop()
			close(eventCh)
			close(doneCh)
		}()
		slog.Info("fetcher starting", "period", f.Timeout)

		for {
			select {
			case <-ctx.Done():
				slog.Info("stopping fetcher")
				return
			case <-ticker.C:
				slog.Info("wake up fetcher")
				if fetchErr := f.Fetch(ctx, eventCh); fetchErr != nil {
					slog.Error("fetch error", "error", fetchErr)
				}
			}
		}
	}()

	return doneCh, eventCh, nil
}

// Fetch retrieves the current load and saves it to the database.
func (f *Fetcher) Fetch(ctx context.Context, eventCh chan<- databaser.Event) error {
	ctx, cancel := context.WithTimeout(ctx, f.QueryTimeout)
	defer cancel()

	load, err := f.getLoad(ctx)
	if err != nil {
		return fmt.Errorf("get load: %w", err)
	}

	event := databaser.Event{Load: load, Timestamp: time.Now().UTC().Truncate(time.Second)}
	if err = f.Db.SaveEvent(ctx, event); err != nil {
		return fmt.Errorf("save event: %w", err)
	}

	eventCh <- event
	slog.Info("fetched", "event", &event)
	return nil
}

// getLoad makes an HTTP request to fetch the current load.
func (f *Fetcher) getLoad(ctx context.Context) (uint8, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("DNT", "1")
	req.Header.Set("Authorization", f.Token)
	req.Header.Set("Referer", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0")
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("X-Angular-Widget", "true")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("TE", "trailers")

	resp, err := f.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("close body error", "error", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	club := new(Club)
	dec := json.NewDecoder(resp.Body)

	if err = dec.Decode(&club); err != nil {
		return 0, fmt.Errorf("decode JSON: %w", err)
	}

	if club.CurrentLoad == "" {
		return 0, errors.New("currentLoad is not set")
	}

	p, err := strconv.ParseUint(strings.TrimRight(club.CurrentLoad, "%"), 10, 8)
	if err != nil {
		return 0, fmt.Errorf("parse currentLoad=%q: %w", club.CurrentLoad, err)
	}

	return uint8(p), nil
}
