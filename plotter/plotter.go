// Package plotter provides utilities for creating and managing plots.
package plotter

import (
	"bytes"
	"log/slog"
	"time"

	"github.com/wcharczuk/go-chart/v2"

	"github.com/z0rr0/ggp/databaser"
)

const dateFormat = "Mon 02.01 15:04"

// Graph generates a graph from the provided events and returns a new image like byte slice.
func Graph(events []databaser.Event) ([]byte, error) {
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

	slog.Debug("created time series", "points", n)

	graph := chart.Chart{
		Title: "Golden Gym",
		//Width:  int(float64(width) * scale),
		//Height: int(float64(height) * scale),
		XAxis: chart.XAxis{
			Name:           "Time",
			ValueFormatter: chart.TimeValueFormatterWithFormat(dateFormat),
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
