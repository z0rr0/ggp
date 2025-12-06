package holidayer

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// writeXML writes XML response with proper Content-Type header.
func writeXML(t *testing.T, w http.ResponseWriter, contentType, body string) {
	t.Helper()
	w.Header().Set("Content-Type", contentType)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("failed to write response: %v", err)
	}
}

const validXMLResponse = `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026" lang="ru" date="2025.09.30" country="ru">
    <holidays>
        <holiday id="1" title="Новогодние каникулы"/>
        <holiday id="2" title="Рождество Христово"/>
        <holiday id="3" title="День защитника Отечества"/>
    </holidays>
    <days>
        <day d="01.01" t="1" h="1"/>
        <day d="01.02" t="1" h="1"/>
        <day d="01.07" t="1" h="2"/>
        <day d="02.23" t="1" h="3"/>
        <day d="04.30" t="2"/>
    </days>
</calendar>`

const emptyDaysXMLResponse = `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026" lang="ru" date="2025.09.30" country="ru">
    <holidays>
        <holiday id="1" title="Test"/>
    </holidays>
    <days>
    </days>
</calendar>`

func TestGetHolidays(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		contentType  string
		statusCode   int
		wantCount    int
		wantErr      bool
	}{
		{
			name:         "valid XML with text/xml",
			responseBody: validXMLResponse,
			contentType:  "text/xml",
			statusCode:   http.StatusOK,
			wantCount:    5,
			wantErr:      false,
		},
		{
			name:         "valid XML with application/xml",
			responseBody: validXMLResponse,
			contentType:  "application/xml",
			statusCode:   http.StatusOK,
			wantCount:    5,
			wantErr:      false,
		},
		{
			name:         "content-type with charset",
			responseBody: validXMLResponse,
			contentType:  "text/xml; charset=utf-8",
			statusCode:   http.StatusOK,
			wantCount:    5,
			wantErr:      false,
		},
		{
			name:         "application/xml with charset",
			responseBody: validXMLResponse,
			contentType:  "application/xml; charset=utf-8",
			statusCode:   http.StatusOK,
			wantCount:    5,
			wantErr:      false,
		},
		{
			name:         "empty days section",
			responseBody: emptyDaysXMLResponse,
			contentType:  "text/xml",
			statusCode:   http.StatusOK,
			wantCount:    0,
			wantErr:      false,
		},
		{
			name:         "server error 500",
			responseBody: "",
			contentType:  "",
			statusCode:   http.StatusInternalServerError,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "not found 404",
			responseBody: "",
			contentType:  "",
			statusCode:   http.StatusNotFound,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "bad request 400",
			responseBody: "",
			contentType:  "",
			statusCode:   http.StatusBadRequest,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "invalid XML response",
			responseBody: "not valid xml <broken",
			contentType:  "text/xml",
			statusCode:   http.StatusOK,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "wrong content-type text/html",
			responseBody: validXMLResponse,
			contentType:  "text/html",
			statusCode:   http.StatusOK,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "wrong content-type application/json",
			responseBody: validXMLResponse,
			contentType:  "application/json",
			statusCode:   http.StatusOK,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "wrong content-type text/plain",
			responseBody: validXMLResponse,
			contentType:  "text/plain",
			statusCode:   http.StatusOK,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name: "invalid date format in day",
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Test"/></holidays>
    <days><day d="2026-01-01" t="1" h="1"/></days>
</calendar>`,
			contentType: "text/xml",
			statusCode:  http.StatusOK,
			wantCount:   0,
			wantErr:     true,
		},
		{
			name: "only type 1 holidays",
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Holiday"/></holidays>
    <days>
        <day d="01.01" t="1" h="1"/>
        <day d="01.02" t="1" h="1"/>
    </days>
</calendar>`,
			contentType: "text/xml",
			statusCode:  http.StatusOK,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name: "only type 2 short days",
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays></holidays>
    <days>
        <day d="04.30" t="2"/>
        <day d="05.08" t="2"/>
    </days>
</calendar>`,
			contentType: "text/xml",
			statusCode:  http.StatusOK,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name: "mixed type 1 and type 2",
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="New Year"/></holidays>
    <days>
        <day d="01.01" t="1" h="1"/>
        <day d="04.30" t="2"/>
    </days>
</calendar>`,
			contentType: "text/xml",
			statusCode:  http.StatusOK,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name: "unknown day type ignored",
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Test"/></holidays>
    <days>
        <day d="01.01" t="1" h="1"/>
        <day d="06.15" t="3"/>
        <day d="07.20" t="99"/>
    </days>
</calendar>`,
			contentType: "text/xml",
			statusCode:  http.StatusOK,
			wantCount:   1,
			wantErr:     false,
		},
		{
			name: "day without holiday reference",
			responseBody: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Holiday One"/></holidays>
    <days>
        <day d="01.01" t="1"/>
    </days>
</calendar>`,
			contentType: "text/xml",
			statusCode:  http.StatusOK,
			wantCount:   1,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET method, got %s", r.Method)
				}

				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					if _, err := w.Write([]byte(tt.responseBody)); err != nil {
						t.Errorf("failed to write response: %v", err)
					}
				}
			}))
			defer server.Close()

			hp := &HolidayParams{
				Location: time.UTC,
				Client:   server.Client(),
			}

			ctx := context.Background()
			holidays, err := hp.getHolidays(ctx, server.URL)

			if (err != nil) != tt.wantErr {
				t.Errorf("getHolidays() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(holidays) != tt.wantCount {
				t.Errorf("getHolidays() returned %d holidays, want %d", len(holidays), tt.wantCount)
			}
		})
	}
}

func TestGetHolidays_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeXML(t, w, "text/xml", validXMLResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Location: time.UTC,
		Client:   server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := hp.getHolidays(ctx, server.URL)
	if err == nil {
		t.Error("expected context canceled error, got nil")
	}
}

func TestGetHolidays_ContextTimeout(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}

	hp := &HolidayParams{
		Location: time.UTC,
		Client:   client,
	}

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		_, err := hp.getHolidays(ctx, server.URL)
		errCh <- err
	}()

	<-started

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected timeout error, got nil")
		}
	case <-time.After(time.Second):
		t.Error("getHolidays did not return within expected time")
	}
}

func TestGetHolidays_HolidayTitles(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays>
        <holiday id="1" title="New Year"/>
        <holiday id="2" title="Christmas"/>
    </holidays>
    <days>
        <day d="01.01" t="1" h="1"/>
        <day d="01.07" t="1" h="2"/>
        <day d="04.30" t="2"/>
    </days>
</calendar>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeXML(t, w, "text/xml", xmlResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Location: time.UTC,
		Client:   server.Client(),
	}

	ctx := context.Background()
	holidays, err := hp.getHolidays(ctx, server.URL)
	if err != nil {
		t.Fatalf("getHolidays() error = %v", err)
	}

	if len(holidays) != 3 {
		t.Fatalf("expected 3 holidays, got %d", len(holidays))
	}

	expectedTitles := map[string]string{
		"2026-01-01": "New Year",
		"2026-01-07": "Christmas",
		"2026-04-30": "", // short day without holiday reference
	}

	for _, h := range holidays {
		dateStr := h.Day.Format(time.DateOnly)
		expectedTitle, ok := expectedTitles[dateStr]
		if !ok {
			t.Errorf("unexpected date %s", dateStr)
			continue
		}
		if h.Title != expectedTitle {
			t.Errorf("holiday %s: title = %q, want %q", dateStr, h.Title, expectedTitle)
		}
	}
}

func TestGetHolidays_Location(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Test"/></holidays>
    <days><day d="01.01" t="1" h="1"/></days>
</calendar>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeXML(t, w, "text/xml", xmlResponse)
	}))
	defer server.Close()

	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	hp := &HolidayParams{
		Location: moscow,
		Client:   server.Client(),
	}

	ctx := context.Background()
	holidays, err := hp.getHolidays(ctx, server.URL)
	if err != nil {
		t.Fatalf("getHolidays() error = %v", err)
	}

	if len(holidays) != 1 {
		t.Fatalf("expected 1 holiday, got %d", len(holidays))
	}

	// The date should use the specified location
	loc := holidays[0].Day.Time().Location()
	if loc.String() != moscow.String() {
		t.Errorf("holiday location = %s, want %s", loc.String(), moscow.String())
	}
}

func TestFetch(t *testing.T) {
	db := newTestDB(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Return different years based on URL
		if strings.Contains(r.URL.Path, "2025") || requestCount == 1 {
			writeXML(t, w, "text/xml", `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2025">
    <holidays><holiday id="1" title="Test 2025"/></holidays>
    <days><day d="01.01" t="1" h="1"/></days>
</calendar>`)
		} else {
			writeXML(t, w, "text/xml", `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Test 2026"/></holidays>
    <days><day d="01.01" t="1" h="1"/></days>
</calendar>`)
		}
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx := context.Background()
	err := hp.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	// Should make 2 requests (current year and next year)
	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}
}

func TestFetch_FirstYearError(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx := context.Background()
	err := hp.Fetch(ctx)
	if err == nil {
		t.Error("expected error, got nil")
		return
	}
	if !strings.Contains(err.Error(), "get holidays") {
		t.Errorf("error should contain 'get holidays', got: %v", err)
	}
}

func TestFetch_SecondYearError(t *testing.T) {
	db := newTestDB(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			writeXML(t, w, "text/xml", validXMLResponse)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx := context.Background()
	err := hp.Fetch(ctx)
	if err == nil {
		t.Error("expected error, got nil")
		return
	}
	if !strings.Contains(err.Error(), "next year") {
		t.Errorf("error should contain 'next year', got: %v", err)
	}
}

func TestFetch_SavesHolidaysToDatabase(t *testing.T) {
	db := newTestDB(t)

	currentYear := time.Now().Year()
	nextYear := currentYear + 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		year := currentYear
		if strings.Contains(r.URL.String(), "year=next") {
			year = nextYear
		}
		xml := strings.Replace(`<?xml version="1.0" encoding="UTF-8"?>
<calendar year="YEAR">
    <holidays><holiday id="1" title="Holiday"/></holidays>
    <days>
        <day d="01.01" t="1" h="1"/>
        <day d="06.12" t="1" h="1"/>
    </days>
</calendar>`, "YEAR", time.Now().Format("2006"), 1)
		writeXML(t, w, "text/xml", xml)
		_ = year // suppress unused
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx := context.Background()
	err := hp.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	// Verify holidays are in database
	holidays, err := db.GetHolidays(ctx, currentYear, time.UTC)
	if err != nil {
		t.Fatalf("GetHolidays() error = %v", err)
	}

	if len(holidays) == 0 {
		t.Error("expected holidays in database, got none")
	}
}

func TestFetch_ContextTimeout(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeXML(t, w, "text/xml", validXMLResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		QueryTimeout: 50 * time.Millisecond,
		Client:       server.Client(),
	}

	ctx := context.Background()
	err := hp.Fetch(ctx)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestRun(t *testing.T) {
	db := newTestDB(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		writeXML(t, w, "text/xml", validXMLResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		Timeout:      50 * time.Millisecond,
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, err := hp.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Wait for initial fetch + at least one tick
	time.Sleep(80 * time.Millisecond)
	cancel()

	// Wait for goroutine to finish
	<-doneCh

	// Initial fetch makes 2 requests (current + next year)
	// Each tick also makes 2 requests
	if requestCount < 4 {
		t.Errorf("expected at least 4 requests, got %d", requestCount)
	}
}

func TestRun_InitialFetchError(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		Timeout:      time.Second,
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneCh, err := hp.Run(ctx)
	if err == nil {
		t.Fatal("expected error on initial fetch failure")
	}
	if doneCh != nil {
		t.Error("expected nil doneCh on error")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeXML(t, w, "text/xml", validXMLResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		Timeout:      time.Hour, // Long timeout to ensure cancellation works
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, err := hp.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	cancel()

	// Should complete quickly
	select {
	case <-doneCh:
		// Success
	case <-time.After(time.Second):
		t.Error("Run() did not stop after context cancellation")
	}
}

func TestRun_ChannelClosed(t *testing.T) {
	db := newTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeXML(t, w, "text/xml", validXMLResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		Timeout:      50 * time.Millisecond,
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, err := hp.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	cancel()
	<-doneCh

	// Verify channel is closed
	_, ok := <-doneCh
	if ok {
		t.Error("doneCh should be closed")
	}
}

func TestRun_ContinuesAfterFetchError(t *testing.T) {
	db := newTestDB(t)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// First 2 requests succeed (initial fetch)
		// Next requests fail then succeed again
		if requestCount <= 2 || requestCount > 4 {
			writeXML(t, w, "text/xml", validXMLResponse)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/<YEAR>",
		Timeout:      30 * time.Millisecond,
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	doneCh, err := hp.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Wait for multiple ticks
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-doneCh

	// Should have continued after error
	if requestCount < 6 {
		t.Errorf("expected at least 6 requests (continues after error), got %d", requestCount)
	}
}

func TestYearTemplate(t *testing.T) {
	db := newTestDB(t)

	capturedURLs := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURLs = append(capturedURLs, r.URL.String())
		writeXML(t, w, "text/xml", validXMLResponse)
	}))
	defer server.Close()

	hp := &HolidayParams{
		Db:           db,
		Location:     time.UTC,
		URL:          server.URL + "/calendar/<YEAR>/data",
		QueryTimeout: 5 * time.Second,
		Client:       server.Client(),
	}

	ctx := context.Background()
	err := hp.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if len(capturedURLs) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(capturedURLs))
	}

	currentYear := time.Now().Year()
	nextYear := currentYear + 1

	// Check year replacement
	for _, url := range capturedURLs {
		if strings.Contains(url, "<YEAR>") {
			t.Errorf("URL should not contain <YEAR> placeholder: %s", url)
		}
	}

	foundCurrent, foundNext := false, false
	for _, url := range capturedURLs {
		if strings.Contains(url, time.Now().Format("2006")) {
			foundCurrent = true
		}
		if strings.Contains(url, time.Date(nextYear, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")) {
			foundNext = true
		}
	}

	if !foundCurrent {
		t.Error("expected URL with current year")
	}
	if !foundNext {
		t.Error("expected URL with next year")
	}
}

func TestXMLCalendar_Unmarshal(t *testing.T) {
	tests := []struct {
		name          string
		xml           string
		wantFirstDay  string
		wantYear      int
		wantHolidays  int
		wantDays      int
		wantFirstType int
	}{
		{
			name:          "full calendar",
			xml:           validXMLResponse,
			wantYear:      2026,
			wantHolidays:  3,
			wantDays:      5,
			wantFirstDay:  "01.01",
			wantFirstType: 1,
		},
		{
			name: "minimal calendar",
			xml: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2025">
    <holidays></holidays>
    <days></days>
</calendar>`,
			wantYear:     2025,
			wantHolidays: 0,
			wantDays:     0,
		},
		{
			name: "calendar with from attribute",
			xml: `<?xml version="1.0" encoding="UTF-8"?>
<calendar year="2026">
    <holidays><holiday id="1" title="Test"/></holidays>
    <days><day d="01.09" t="1" f="01.03"/></days>
</calendar>`,
			wantYear:      2026,
			wantHolidays:  1,
			wantDays:      1,
			wantFirstDay:  "01.09",
			wantFirstType: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeXML(t, w, "text/xml", tt.xml)
			}))
			defer server.Close()

			hp := &HolidayParams{
				Location: time.UTC,
				Client:   server.Client(),
			}

			ctx := context.Background()
			holidays, err := hp.getHolidays(ctx, server.URL)
			if err != nil {
				if tt.wantDays > 0 {
					t.Fatalf("getHolidays() error = %v", err)
				}
				return
			}

			if len(holidays) != tt.wantDays {
				t.Errorf("got %d holidays, want %d", len(holidays), tt.wantDays)
			}
		})
	}
}

func TestGetHolidays_EmptyURL(t *testing.T) {
	hp := &HolidayParams{
		Location: time.UTC,
		Client:   http.DefaultClient,
	}

	ctx := context.Background()
	_, err := hp.getHolidays(ctx, "")
	if err == nil {
		t.Error("expected error with empty URL")
	}
}

func TestGetHolidays_InvalidURL(t *testing.T) {
	hp := &HolidayParams{
		Location: time.UTC,
		Client:   http.DefaultClient,
	}

	ctx := context.Background()
	_, err := hp.getHolidays(ctx, "://invalid-url")
	if err == nil {
		t.Error("expected error with invalid URL")
	}
}
