// Package predictor provides functionalities for loading predictions based on saved data.
package predictor

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

const (
	dayTypesCount = 8  // 7 days + holiday
	hoursInDay    = 24 // 0..23

	averageLoad = 25.0 // not 50, 25 is more realistic for an average load
)

// HourlyStats is a storage for hourly statistics.
type HourlyStats struct {
	LastUpdate  time.Time // last update time
	WeightedSum float64   // Sum(load × weight)
	TotalWeight float64   // Sum(weight)
	Count       uint64    // total events counted
}

// Prediction represents a load prediction for a specific hour.
type Prediction struct {
	TargetTime time.Time
	Hour       int
	Load       float64
	Confidence float64 // prediction confidence [0.0..1.0]
	IsHoliday  bool
}

// Predictor holds the statistics and provides methods to update and retrieve predictions.
type Predictor struct {
	stats               [dayTypesCount][hoursInDay]*HourlyStats
	holidayChecker      HolidayChecker
	recentEvents        []databaser.Event
	decayLambda         float64
	minWeight           float64
	confidenceThreshold float64
	maxRecentCount      int
	mu                  sync.RWMutex
}

// New creates a new Predictor instance with the provided HolidayChecker.
func New(holidayChecker HolidayChecker) *Predictor {
	p := &Predictor{
		holidayChecker:      holidayChecker,
		decayLambda:         0.1,  // exp(-0.1*7) ~= 0.5
		minWeight:           0.5,  // minimum weight for prediction confidence
		maxRecentCount:      40,   // ~ last hour 3600 / 90 = 40
		confidenceThreshold: 20.0, // weight threshold for max confidence
	}

	// initialize the statistics array
	for d := range dayTypesCount {
		for h := range hoursInDay {
			p.stats[d][h] = &HourlyStats{}
		}
	}

	return p
}

// AddEvent adds a new event to the predictor and updates the statistics.
func (p *Predictor) AddEvent(event databaser.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	dayType := p.getDayType(event.Timestamp)
	hour := event.Timestamp.Hour()
	stats := p.stats[dayType][hour]

	if !stats.LastUpdate.IsZero() {
		daysSinceUpdate := event.Timestamp.Sub(stats.LastUpdate).Hours() / hoursInDay
		if daysSinceUpdate > 0 {
			decayFactor := math.Exp(-p.decayLambda * daysSinceUpdate)
			stats.WeightedSum *= decayFactor
			stats.TotalWeight *= decayFactor
		}
	}

	stats.WeightedSum += event.FloatLoad()
	stats.TotalWeight += 1.0
	stats.Count++
	stats.LastUpdate = event.Timestamp

	p.recentEvents = append(p.recentEvents, event)
	if len(p.recentEvents) > p.maxRecentCount {
		p.recentEvents = p.recentEvents[1:]
	}
}

// Predict returns a load prediction for the specified number of hours ahead.
func (p *Predictor) Predict(hoursAhead uint8) Prediction {
	var basePrediction, confidence float64
	p.mu.RLock()
	defer p.mu.RUnlock()

	now := time.Now().UTC()
	targetTime := now.Add(time.Duration(hoursAhead) * time.Hour)

	dayType := p.getDayType(targetTime)
	hour := targetTime.Hour()
	stats := p.stats[dayType][hour] // day-hour stats
	basePrediction = p.predictWithBlending(targetTime, hour)

	switch {
	case stats.TotalWeight >= p.minWeight:
		confidence = p.calculateConfidence(stats, dayType)
	case dayType == Holiday:
		sundayStats := p.stats[Sunday][hour]
		if sundayStats.TotalWeight >= p.minWeight {
			confidence = 0.5
		} else {
			confidence = 0.3
		}
	default:
		basePrediction = p.fallbackPrediction(int(dayType))
		confidence = 0.3
	}

	// trend correction for short-term predictions
	if hoursAhead <= 3 && len(p.recentEvents) >= 20 {
		trend := p.calculateTrend()
		trendWeight := 0.3 / float64(hoursAhead)
		basePrediction += trend * trendWeight * float64(hoursAhead)
	}

	basePrediction = max(0.0, min(100.0, basePrediction))

	return Prediction{
		TargetTime: targetTime,
		Hour:       hour,
		Load:       basePrediction,
		Confidence: confidence,
		IsHoliday:  dayType == Holiday,
	}
}

// PredictRange returns load predictions for the next maxHours hours.
func (p *Predictor) PredictRange(maxHours uint8) []Prediction {
	var h uint8
	predictions := make([]Prediction, maxHours)

	for h = 1; h <= maxHours; h++ {
		predictions[h-1] = p.Predict(h)
	}

	return predictions
}

// String implements the Stringer interface for Predictor.
// It returns statistics for all day types and hours.
func (p *Predictor) String() string {
	var s strings.Builder

	for i := range dayTypesCount {
		for j := range hoursInDay {
			stats := p.stats[i][j]
			s.WriteString(fmt.Sprintf("DayType %d Hour %02d: Count=%d WeightedSum=%.2f TotalWeight=%.2f LastUpdate=%s\n",
				i, j, stats.Count, stats.WeightedSum, stats.TotalWeight, stats.LastUpdate.Format(time.RFC3339)))
		}
	}

	return s.String()
}

// GetTypicalLoad returns the typical load for the given time based on historical data.
func (p *Predictor) GetTypicalLoad(t time.Time) float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	dayType := p.getDayType(t)
	hour := t.Hour()
	stats := p.stats[dayType][hour]

	if stats.TotalWeight >= p.minWeight {
		return stats.WeightedSum / stats.TotalWeight
	}

	return p.fallbackPrediction(int(dayType))
}

// getDayType determines the DayType for the given time.
func (p *Predictor) getDayType(t time.Time) DayType {
	if p.holidayChecker != nil && p.holidayChecker.IsHoliday(t) {
		return Holiday
	}
	// #nosec G115 -- Weekday() returns 0-6, always fits in uint8
	return DayType(t.Weekday())
}

// calculateTrend calculates the trend of recent events using linear regression.
func (p *Predictor) calculateTrend() float64 {
	n := len(p.recentEvents)
	if n < 3 {
		return 0
	}

	// linear regression to find the trend = (last - first) / counted
	first := p.recentEvents[0]
	last := p.recentEvents[n-1]
	hoursDiff := last.Timestamp.Sub(first.Timestamp).Hours()

	if hoursDiff < 0.1 {
		return 0 // too small interval
	}

	return (last.FloatLoad() - first.FloatLoad()) / hoursDiff
}

// fallbackPrediction returns a fallback prediction for the given day of the week.
func (p *Predictor) fallbackPrediction(dayOfWeek int) float64 {
	var sum, weight float64

	for h := range hoursInDay {
		stats := p.stats[dayOfWeek][h]
		if stats.TotalWeight > 0 {
			sum += stats.WeightedSum
			weight += stats.TotalWeight
		}
	}

	if weight > 0 {
		return sum / weight
	}

	return averageLoad
}

func (p *Predictor) calculateConfidence(stats *HourlyStats, dayType DayType) float64 {
	// base confidence based on total weight
	base := math.Min(1.0, stats.TotalWeight/p.confidenceThreshold)

	// small penalty for holidays
	if dayType == Holiday {
		base *= 0.7
	}

	// penalty for stale data
	if !stats.LastUpdate.IsZero() {
		daysSince := time.Since(stats.LastUpdate).Hours() / 24
		freshness := math.Exp(-0.05 * daysSince) // 2 weeks -> ~0.37
		base *= freshness
	}

	return base
}

func (p *Predictor) getWeightedAverage(dayType DayType, hour int) float64 {
	stats := p.stats[dayType][hour]
	if stats.TotalWeight < 0.1 {
		return averageLoad
	}

	return stats.WeightedSum / stats.TotalWeight
}

func (p *Predictor) predictWithBlending(targetTime time.Time, hour int) float64 {
	isHoliday := p.holidayChecker != nil && p.holidayChecker.IsHoliday(targetTime)

	if !isHoliday {
		// #nosec G115 -- Weekday() returns 0-6, always fits in uint8
		dayType := DayType(targetTime.Weekday())
		return p.getWeightedAverage(dayType, hour)
	}

	// holiday — blend holiday and Sunday stats
	holidayStats := p.stats[Holiday][hour]
	sundayStats := p.stats[Sunday][hour]

	holidayWeight := holidayStats.TotalWeight
	sundayWeight := sundayStats.TotalWeight * 0.5 // sunday has less weight

	totalWeight := holidayWeight + sundayWeight
	if totalWeight < 0.1 {
		return averageLoad
	}

	holidayAvg := 0.0
	if holidayStats.TotalWeight > 0 {
		holidayAvg = holidayStats.WeightedSum / holidayStats.TotalWeight
	}

	sundayAvg := 0.0
	if sundayStats.TotalWeight > 0 {
		sundayAvg = sundayStats.WeightedSum / sundayStats.TotalWeight
	}

	return (holidayAvg*holidayWeight + sundayAvg*sundayWeight) / totalWeight
}
