// Package plotter provides utilities for creating and managing plots.
package plotter

import (
	"bytes"
	"errors"
	"log/slog"
	"sync"
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
	periodSecond = time.Minute * 2
	periodMinute = time.Hour * 24
	periodHour   = time.Hour * 72
	periodDay    = time.Hour * 24 * 14
	periodWeek   = time.Hour * 24 * 30 * 3
	periodMonth  = time.Hour * 24 * 365 * 2
)

var (
	// dtFormatMap maps time format constants to their corresponding layout strings.
	// Full format "2006-01-02T15:04:05Z07:00"
	dtFormatMap = map[int]string{
		dtFormatSecond: "5s",
		dtFormatMinute: "15:04",
		dtFormatHour:   "Mon 15:00",
		dtFormatDay:    "Mon _2 Jan",
		dtFormatWeek:   "Monday _2",
		dtFormatMonth:  "01.2006",
		dtFormatYear:   "2006",
	}

	bufferPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
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
func Graph(events, prediction []databaser.Event, location *time.Location) ([]byte, error) {
	var (
		n  = len(events)
		np = len(prediction)
		xs = make([]time.Time, 0, n)
		ys = make([]float64, 0, n)
		// prediction
		pxs = make([]time.Time, 0, np)
		pys = make([]float64, 0, np)
	)

	if n < 1 {
		return nil, errors.New("graph called with no events")
	}

	maxY := 0.0
	for _, event := range events {
		load := event.FloatLoad()
		xs = append(xs, event.Timestamp)
		ys = append(ys, load)
		maxY = max(maxY, load)
	}

	for _, event := range prediction {
		load := event.Predict
		pxs = append(pxs, event.Timestamp)
		pys = append(pys, load)
		maxY = max(maxY, load)
	}

	mainSeries := chart.TimeSeries{
		Name:    "Load",
		XValues: xs,
		YValues: ys,
		Style: chart.Style{
			StrokeColor: chart.ColorBlue,
			StrokeWidth: 4.0,
		},
	}
	series := []chart.Series{mainSeries}

	if np > 1 {
		for _, event := range prediction {
			load := event.FloatLoad()
			maxY = max(maxY, load)

			pxs = append(pxs, event.Timestamp.Add(5*time.Minute))
			pys = append(pys, load+5.0)
		}

		predictionSeries := chart.TimeSeries{
			Name:    "Prediction",
			XValues: pxs,
			YValues: pys,
			Style: chart.Style{
				StrokeColor:     chart.ColorRed,
				StrokeWidth:     3.0,
				StrokeDashArray: []float64{5.0, 5.0},
			},
		}
		series = append(series, predictionSeries)
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
			GridMinorStyle: chart.Style{
				StrokeColor: chart.ColorLightGray,
				StrokeWidth: 1.0,
			},
		},
		YAxis: chart.YAxis{
			Name: "Load (%)",
			Range: &chart.ContinuousRange{
				Min: 0.0,
				Max: maxY + 10.0,
			},
			GridMajorStyle: chart.Style{
				StrokeColor: chart.ColorAlternateGray,
				StrokeWidth: 1.0,
			},
			GridMinorStyle: chart.Style{
				StrokeColor: chart.ColorLightGray,
				StrokeWidth: 1.0,
			},
		},
		Series: series,
	}

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	err := graph.Render(chart.PNG, buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
