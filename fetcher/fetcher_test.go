package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

func newTestDB(t *testing.T) *databaser.DB {
	t.Helper()
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:", 1)
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

// writeJSON writes JSON response with proper Content-Type header.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("failed to encode JSON: %v", err)
	}
}

func TestGetLoad(t *testing.T) {
	tests := []struct {
		name         string
		responseBody any
		contentType  string
		statusCode   int
		wantLoad     uint8
		wantErr      bool
	}{
		{
			name:         "valid load 34%",
			responseBody: Club{ID: 1309, Title: "Test Club", CurrentLoad: "34%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     34,
			wantErr:      false,
		},
		{
			name:         "valid load 0%",
			responseBody: Club{ID: 1, Title: "Empty Club", CurrentLoad: "0%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      false,
		},
		{
			name:         "valid load 100%",
			responseBody: Club{ID: 1, Title: "Full Club", CurrentLoad: "100%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     100,
			wantErr:      false,
		},
		{
			name:         "load without percent sign",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: "50"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     50,
			wantErr:      false,
		},
		{
			name:         "content-type with charset",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: "75%"},
			contentType:  "application/json; charset=utf-8",
			statusCode:   http.StatusOK,
			wantLoad:     75,
			wantErr:      false,
		},
		{
			name:         "empty currentLoad",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: ""},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "invalid currentLoad format",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: "abc%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "negative load value",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: "-5%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "load exceeds 100%",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: "101%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "load exceeds uint8 max",
			responseBody: Club{ID: 1, Title: "Test", CurrentLoad: "300%"},
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "server error 500",
			responseBody: nil,
			contentType:  "",
			statusCode:   http.StatusInternalServerError,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "not found 404",
			responseBody: nil,
			contentType:  "",
			statusCode:   http.StatusNotFound,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "unauthorized 401",
			responseBody: nil,
			contentType:  "",
			statusCode:   http.StatusUnauthorized,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "invalid JSON response",
			responseBody: "not a json",
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "wrong content-type text/html",
			responseBody: Club{ID: 1, CurrentLoad: "50%"},
			contentType:  "text/html",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "wrong content-type text/plain",
			responseBody: Club{ID: 1, CurrentLoad: "50%"},
			contentType:  "text/plain",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
		{
			name:         "empty content-type",
			responseBody: Club{ID: 1, CurrentLoad: "50%"},
			contentType:  "",
			statusCode:   http.StatusOK,
			wantLoad:     0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "" {
					t.Error("expected Authorization header")
				}
				if r.Header.Get("Accept") == "" {
					t.Error("expected Accept header")
				}
				if r.Method != http.MethodGet {
					t.Errorf("expected GET method, got %s", r.Method)
				}

				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != nil {
					if str, ok := tt.responseBody.(string); ok {
						if _, err := w.Write([]byte(str)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					} else {
						if err := json.NewEncoder(w).Encode(tt.responseBody); err != nil {
							t.Errorf("failed to encode JSON: %v", err)
						}
					}
				}
			}))
			defer server.Close()

			f := &Fetcher{
				Client:       server.Client(),
				URL:          server.URL,
				Token:        "test-token",
				QueryTimeout: 5 * time.Second,
			}

			ctx := context.Background()
			load, err := f.getLoad(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("getLoad() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if load != tt.wantLoad {
				t.Errorf("getLoad() = %d, want %d", load, tt.wantLoad)
			}
		})
	}
}

func TestGetLoad_ContextTimeout(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		// Block long enough for client timeout to trigger
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Create client with short timeout to test timeout behavior
	client := &http.Client{Timeout: 50 * time.Millisecond}

	f := &Fetcher{
		Client:       client,
		URL:          server.URL,
		Token:        "test-token",
		QueryTimeout: 5 * time.Second,
	}

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		_, err := f.getLoad(ctx)
		errCh <- err
	}()

	<-started

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected timeout error, got nil")
		}
	case <-time.After(time.Second):
		t.Error("getLoad did not return within expected time")
	}
}

func TestGetLoad_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeJSON(t, w, Club{ID: 1, CurrentLoad: "50%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		QueryTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := f.getLoad(ctx)
	if err == nil {
		t.Error("expected context canceled error, got nil")
	}
}

func TestFetch(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, Club{ID: 1, Title: "Test", CurrentLoad: "42%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		QueryTimeout: 5 * time.Second,
	}

	eventCh := make(chan databaser.Event, 1)
	ctx := context.Background()

	err := f.Fetch(ctx, eventCh)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	select {
	case event := <-eventCh:
		if event.Load != 42 {
			t.Errorf("expected load 42, got %d", event.Load)
		}
		if event.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	default:
		t.Error("expected event on channel")
	}
}

func TestFetch_HTTPError(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		QueryTimeout: 5 * time.Second,
	}

	eventCh := make(chan databaser.Event, 1)
	ctx := context.Background()

	err := f.Fetch(ctx, eventCh)
	if err == nil {
		t.Error("expected error, got nil")
	}

	select {
	case <-eventCh:
		t.Error("expected no event on channel after error")
	default:
		// Expected: no event sent
	}
}

func TestRun(t *testing.T) {
	db := newTestDB(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		writeJSON(t, w, Club{ID: 1, Title: "Test", CurrentLoad: "50%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		Timeout:      50 * time.Millisecond,
		QueryTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, eventCh, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Wait for initial fetch + at least one tick
	time.Sleep(80 * time.Millisecond)
	cancel()

	// Drain events
	drainEvents(eventCh)

	// Wait for goroutine to finish
	<-doneCh

	if requestCount < 2 {
		t.Errorf("expected at least 2 requests, got %d", requestCount)
	}
}

func TestRun_InitialFetchError(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		Timeout:      time.Second,
		QueryTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh, eventCh, err := f.Run(ctx)
	if err == nil {
		t.Fatal("expected error on initial fetch failure")
	}
	if doneCh != nil {
		t.Error("expected nil doneCh on error")
	}
	if eventCh != nil {
		t.Error("expected nil eventCh on error")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, Club{ID: 1, CurrentLoad: "25%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		Timeout:      time.Hour, // Long timeout to ensure cancellation works
		QueryTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, eventCh, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Read initial event
	<-eventCh

	// Cancel context
	cancel()

	// Should complete quickly
	select {
	case <-doneCh:
		// Success
	case <-time.After(time.Second):
		t.Error("Run() did not stop after context cancellation")
	}
}

func TestRun_ChannelsClosed(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, Club{ID: 1, CurrentLoad: "30%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		Timeout:      50 * time.Millisecond,
		QueryTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, eventCh, err := f.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Drain initial event
	<-eventCh

	cancel()
	<-doneCh

	// Verify channels are closed
	_, ok := <-eventCh
	if ok {
		t.Error("eventCh should be closed")
	}

	_, ok = <-doneCh
	if ok {
		t.Error("doneCh should be closed")
	}
}

func TestFetch_EventTimestamp(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, Club{ID: 1, CurrentLoad: "55%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Db:           db,
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "test-token",
		QueryTimeout: 5 * time.Second,
	}

	eventCh := make(chan databaser.Event, 1)
	ctx := context.Background()

	before := time.Now().UTC().Truncate(time.Second)
	err := f.Fetch(ctx, eventCh)
	after := time.Now().UTC().Truncate(time.Second).Add(time.Second)

	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	event := <-eventCh

	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("timestamp %v not in expected range [%v, %v]", event.Timestamp, before, after)
	}

	// Verify timestamp is truncated to seconds (no nanoseconds)
	if event.Timestamp.Nanosecond() != 0 {
		t.Errorf("timestamp should be truncated to seconds, got nanosecond=%d", event.Timestamp.Nanosecond())
	}
}

func TestClubJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantClub Club
		wantErr  bool
	}{
		{
			name:     "full response",
			json:     `{"id":1309,"title":"st. Street","currentLoad":"34%"}`,
			wantClub: Club{ID: 1309, Title: "st. Street", CurrentLoad: "34%"},
			wantErr:  false,
		},
		{
			name:     "minimal response",
			json:     `{"currentLoad":"50%"}`,
			wantClub: Club{CurrentLoad: "50%"},
			wantErr:  false,
		},
		{
			name:     "extra fields ignored",
			json:     `{"id":1,"currentLoad":"10%","city":"City","unknownField":"value"}`,
			wantClub: Club{ID: 1, CurrentLoad: "10%"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var club Club
			err := json.Unmarshal([]byte(tt.json), &club)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if club != tt.wantClub {
				t.Errorf("json.Unmarshal() = %+v, want %+v", club, tt.wantClub)
			}
		})
	}
}

func TestGetLoad_RequestHeaders(t *testing.T) {
	var capturedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		writeJSON(t, w, Club{ID: 1, CurrentLoad: "50%"})
	}))
	defer server.Close()

	f := &Fetcher{
		Client:       server.Client(),
		URL:          server.URL,
		Token:        "Bearer secret-token",
		QueryTimeout: 5 * time.Second,
	}

	ctx := context.Background()
	_, err := f.getLoad(ctx)
	if err != nil {
		t.Fatalf("getLoad() error = %v", err)
	}

	expectedHeaders := map[string]string{
		"Authorization":    "Bearer secret-token",
		"Accept":           "application/json, text/javascript, */*; q=0.01",
		"Cache-Control":    "no-cache",
		"X-Requested-With": "XMLHttpRequest",
	}

	for header, want := range expectedHeaders {
		got := capturedHeaders.Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
}

func drainEvents(ch <-chan databaser.Event) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}
