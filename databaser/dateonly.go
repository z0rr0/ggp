package databaser

import (
	"database/sql/driver"
	"errors"
	"time"
)

// DateOnly is a custom type that stores only the date part of time.Time.
type DateOnly time.Time

func (d *DateOnly) Time() time.Time {
	return time.Time(*d)
}

// Format formats the DateOnly using the provided layout.
func (d *DateOnly) Format(layout string) string {
	return time.Time(*d).Format(layout)
}

func (d *DateOnly) String() string {
	return time.Time(*d).Format(time.DateOnly)
}

func (d *DateOnly) Date() (year int, month time.Month, day int) {
	return time.Time(*d).Date()
}

func (d *DateOnly) Before(u *DateOnly) bool {
	return time.Time(*d).Before(time.Time(*u))
}

func (d *DateOnly) After(u *DateOnly) bool {
	return time.Time(*d).After(time.Time(*u))
}

// Value implements driver.Valuer interface.
func (d *DateOnly) Value() (driver.Value, error) {
	if d == nil {
		return nil, errors.New("nil date only")
	}

	return time.Time(*d).Format(time.DateOnly), nil
}

func (d *DateOnly) StartOfYear() string {
	year, _, _ := d.Date()
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	return start.Format(time.DateOnly)
}

func (d *DateOnly) EndOfYear() string {
	year, _, _ := d.Date()
	end := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	return end.Format(time.DateOnly)
}

func (d *DateOnly) SetLocation(loc *time.Location) {
	y, m, day := d.Date()
	t := time.Date(y, m, day, 0, 0, 0, 0, loc)
	*d = DateOnly(t)
}

// Scan implements sql.Scanner interface.
func (d *DateOnly) Scan(value any) error {
	if value == nil {
		*d = DateOnly{}
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		*d = DateOnly(v)
	case string:
		t, err := time.Parse(time.DateOnly, v)
		if err != nil {
			if t, err = time.Parse(time.RFC3339, v); err != nil {
				return err
			}
		}
		*d = DateOnly(t)
	}
	return nil
}
