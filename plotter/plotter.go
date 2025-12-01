// Package plotter provides utilities for creating and managing plots.
package plotter

import (
	"bytes"
	"log/slog"
	"time"

	"github.com/wcharczuk/go-chart/v2"

	"github.com/z0rr0/ggp/databaser"
)

const (
	// Time format constants
	dtFormatSecond = iota + 1
	dtFormatMinute
	dtFormatHour
	dtFormatDay
	dtFormatWeek
	dtFormatMonth
	dtFormatYear
)

const (
	// periods defines time durations for different ranges
	periodSecond = time.Minute * 5
	periodMinute = time.Hour * 2
	periodHour   = time.Hour * 36
	periodDay    = time.Hour * 24 * 10
	periodWeek   = time.Hour * 24 * 30 * 2
	periodMonth  = time.Hour * 24 * 365 * 2
)

var (
	// dtFormatMap maps time format constants to their corresponding layout strings.
	// Full format "2006-01-02T15:04:05Z07:00"
	dtFormatMap = map[int]string{
		dtFormatSecond: "04:05",
		dtFormatMinute: "15:04",
		dtFormatHour:   "Mon 02.01 15",
		dtFormatDay:    "Mon 02.01",
		dtFormatWeek:   "Mon 02",
		dtFormatMonth:  "01.2006",
		dtFormatYear:   "2006",
	}
)

func getDateFormat(events []time.Time) string {
	n := len(events)
	if n < 2 {
		return dtFormatMap[dtFormatDay]
	}

	slog.Debug("determining date format", "first", events[0], "last", events[n-1])
	switch diff := events[n-1].Sub(events[0]); {
	case diff <= periodSecond:
		return dtFormatMap[dtFormatSecond]
	case diff <= periodMinute:
		return dtFormatMap[dtFormatMinute]
	case diff <= periodHour:
		return dtFormatMap[dtFormatHour]
	case diff <= periodDay:
		return dtFormatMap[dtFormatDay]
	case diff <= periodWeek:
		return dtFormatMap[dtFormatWeek]
	case diff <= periodMonth:
		return dtFormatMap[dtFormatMonth]
	default:
		return dtFormatMap[dtFormatYear]
	}
}

// Graph generates a graph from the provided events and returns a new image like byte slice.
func Graph(events []databaser.Event, location *time.Location) ([]byte, error) {
	var (
		n   = len(events)
		buf = new(bytes.Buffer)
		xs  = make([]time.Time, 0, n)
		ys  = make([]float64, 0, n)
	)

	for _, event := range events {
		xs = append(xs, event.Timestamp)
		ys = append(ys, float64(event.Load))
	}

	series := chart.TimeSeries{
		Name:    "Load",
		XValues: xs,
		YValues: ys,
		Style: chart.Style{
			StrokeColor: chart.ColorBlue,
			StrokeWidth: 2.0,
		},
	}

	layout := getDateFormat(xs)
	slog.Debug("created time series", "points", n, "dateFormat", layout)

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name: "Time",
			ValueFormatter: func(v interface{}) string {
				if vt, ok := v.(time.Time); ok {
					return vt.In(location).Format(layout)
				}
				if vt, ok := v.(int64); ok {
					return time.Unix(0, vt).In(location).Format(layout)
				}
				if vt, ok := v.(float64); ok {
					return time.Unix(0, int64(vt)).In(location).Format(layout)
				}
				return ""
			},
			GridMajorStyle: chart.Style{
				StrokeColor: chart.ColorAlternateGray,
				StrokeWidth: 1.0,
			},
		},
		YAxis: chart.YAxis{
			Name: "Load (%)",
			GridMajorStyle: chart.Style{
				StrokeColor: chart.ColorAlternateGray,
				StrokeWidth: 1.0,
			},
		},
		Series: []chart.Series{series},
	}

	err := graph.Render(chart.PNG, buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
