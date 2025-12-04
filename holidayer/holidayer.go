// Package holidayer provides functionality to periodically fetch holiday data
// from an external API and store it in a database.
//
// API response XML example:
// ```xml
// <calendar year="2025" lang="ru" date="2024.12.01">
//
//	<holidays>
//	  <holiday id="1" title="title-1"/>
//	  <holiday id="2" title="title-2"/>
//	</holidays>
//	<days>
//	  <day d="01.01" t="1" h="1"/>
//	  <day d="01.02" t="2" h="1"/>
//	  <day d="12.31" t="1" f="01.05"/>
//	</days>
//
// </calendar>
// ```
package holidayer

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/z0rr0/ggp/databaser"
)

const (
	holidayTypeHoliday  = 1
	holidayTypeShortDay = 2
	dateFormat          = "01.02" // format used in the XML response for dates (MM.DD)
	yearTemplate        = "<YEAR>"
)

// XMLCalendar is the root structure of the XML response.
type XMLCalendar struct {
	Year     int         `xml:"year,attr"`
	Holidays XMLHolidays `xml:"holidays"`
	Days     XMLDays     `xml:"days"`
}

// XMLHolidays presents the holidays section.
type XMLHolidays struct {
	Items []XMLHoliday `xml:"holiday"`
}

// XMLHoliday is a holiday entry in the calendar.
type XMLHoliday struct {
	ID    int    `xml:"id,attr"`
	Title string `xml:"title,attr"`
}

// XMLDays presents the days section.
type XMLDays struct {
	Items []XMLDay `xml:"day"`
}

// XMLDay is a day entry in the calendar.
type XMLDay struct {
	Date    string `xml:"d,attr"` // format: "01.01"
	Type    int    `xml:"t,attr"` // 1 - holiday, 2 - short day
	Holiday int    `xml:"h,attr"` // holiday ID
	From    string `xml:"f,attr"` // move holiday from date
}

// HolidayParams struct holds the configuration for the fetcher.
type HolidayParams struct {
	Db           *databaser.DB
	Location     *time.Location
	URL          string
	Timeout      time.Duration
	QueryTimeout time.Duration
	Client       *http.Client
}

// Run begins the periodic fetching process.
func (hp *HolidayParams) Run(ctx context.Context) (<-chan struct{}, error) {
	err := hp.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("initial holidays fetch: %v", err)
	}

	doneCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(hp.Timeout)
		defer ticker.Stop()
		slog.Info("holidayer starting", "period", hp.Timeout)

		for {
			select {
			case <-ctx.Done():
				slog.Info("stopping holidayer")
				close(doneCh)
				return
			case <-ticker.C:
				slog.Info("wake up holidayer")
				if fetchErr := hp.Fetch(ctx); fetchErr != nil {
					slog.Error("holidayer error", "error", fetchErr)
				}
			}
		}
	}()

	return doneCh, nil
}

// Fetch retrieves the current load and saves it to the database.
func (hp *HolidayParams) Fetch(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, hp.QueryTimeout)
	defer cancel()

	year := time.Now().In(hp.Location).Year()
	url := strings.Replace(hp.URL, yearTemplate, strconv.Itoa(year), 1)

	slog.DebugContext(ctx, "fetching holidays", "url", url, "year", year)
	holidays, err := hp.getHolidays(ctx, url)
	if err != nil {
		return fmt.Errorf("get holidays: %v", err)
	}

	// add next year holidays
	year++
	url = strings.Replace(hp.URL, yearTemplate, strconv.Itoa(year), 1)

	slog.DebugContext(ctx, "fetching holidays", "url", url, "year", year)
	holidaysNext, err := hp.getHolidays(ctx, url)
	if err != nil {
		return fmt.Errorf("get holidays for next year: %v", err)
	}

	holidays = append(holidays, holidaysNext...)
	err = databaser.InTransaction(ctx, hp.Db, func(tx *sqlx.Tx) error {
		return databaser.SaveManyHolidaysTx(ctx, tx, holidays)
	})

	if err != nil {
		return fmt.Errorf("save holidays: %v", err)
	}

	slog.InfoContext(ctx, "holidayer fetched", "count", len(holidays))
	return nil
}

// getHolidays makes an HTTP request to fetch holidays for the specified year.
func (hp *HolidayParams) getHolidays(ctx context.Context, url string) ([]databaser.Holiday, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %v", err)
	}

	resp, err := hp.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("close body error", "error", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var calendar XMLCalendar
	err = xml.NewDecoder(resp.Body).Decode(&calendar)
	if err != nil {
		return nil, fmt.Errorf("decode response: %v", err)
	}

	holidayTitles := make(map[int]string, len(calendar.Holidays.Items))
	for _, h := range calendar.Holidays.Items {
		holidayTitles[h.ID] = h.Title
	}

	n := len(calendar.Days.Items)
	if n == 0 {
		slog.WarnContext(ctx, "no holidays found in the response", "year", calendar.Year)
		return nil, nil
	}

	holidays := make([]databaser.Holiday, 0, n)
	for _, day := range calendar.Days.Items {
		if day.Type == holidayTypeHoliday || day.Type == holidayTypeShortDay {
			dateParsed, dateErr := time.Parse(dateFormat, day.Date)
			if dateErr != nil {
				return nil, fmt.Errorf("parse date %q: %w", day.Date, dateErr)
			}

			dt := databaser.DateOnly(time.Date(calendar.Year, dateParsed.Month(), dateParsed.Day(), 0, 0, 0, 0, hp.Location))
			holidays = append(
				holidays,
				databaser.Holiday{
					Day:   &dt,
					Title: holidayTitles[day.Holiday], // not required
				},
			)
		}
	}

	slog.InfoContext(ctx, "collected holidays", "holidays", len(holidays), "year", calendar.Year)
	return holidays, nil
}
