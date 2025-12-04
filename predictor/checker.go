package predictor

import (
	"context"
	"fmt"
	"time"

	"github.com/z0rr0/ggp/databaser"
)

// DayType defines the type of day.
type DayType uint8

const (
	Monday DayType = iota
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
	Sunday
	Holiday // predefined holiday
)

// HolidayChecker checks if a given date is a holiday and retrieves the holiday title.
type HolidayChecker interface {
	IsHoliday(t time.Time) bool
	HolidayTitle(t time.Time) string
}

// monthDay represents a month and day combination.
type monthDay struct {
	month uint8
	day   uint8
}

type RussianHolidayChecker struct {
	fixedHolidays map[monthDay]string
}

// NewRussianHolidayChecker creates a new RussianHolidayChecker with holidays loaded from the database.
func NewRussianHolidayChecker(ctx context.Context, db *databaser.DB, location *time.Location) (*RussianHolidayChecker, error) {
	year, _, _ := time.Now().In(location).Date()
	holidays, err := db.GetHolidays(ctx, year, location)

	if err != nil {
		return nil, fmt.Errorf("failed to get holidays: %w", err)
	}

	fixedHolidays := make(map[monthDay]string)
	for _, h := range holidays {
		_, m, d := h.Day.Date()
		fixedHolidays[monthDay{month: uint8(m), day: uint8(d)}] = h.Title
	}

	return &RussianHolidayChecker{fixedHolidays: fixedHolidays}, nil
}

// IsHoliday checks if the given date is a holiday.
func (c *RussianHolidayChecker) IsHoliday(t time.Time) bool {
	_, m, d := t.Date()
	md := monthDay{month: uint8(m), day: uint8(d)}

	_, isFixed := c.fixedHolidays[md]
	return isFixed
}

// HolidayTitle returns the title of the holiday for the given date.
func (c *RussianHolidayChecker) HolidayTitle(t time.Time) string {
	_, m, d := t.Date()
	md := monthDay{month: uint8(m), day: uint8(d)}
	return c.fixedHolidays[md]
}
