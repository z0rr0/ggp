package databaser

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
)

// Holiday represents a holiday with a date and title.
type Holiday struct {
	Day   *DateOnly `db:"day"`
	Title string    `db:"title"`
}

// LogValue implements slog.LogValuer for Event.
func (h *Holiday) LogValue() slog.Value {
	return slog.StringValue(fmt.Sprintf("{date: '%s', title: '%s'}", h.Day.String(), h.Title))
}

// SaveManyHolidaysTx stores multiple holidays in the database within a transaction.
func SaveManyHolidaysTx(ctx context.Context, tx *sqlx.Tx, holidays []Holiday) error {
	if len(holidays) == 0 {
		return nil
	}

	minDay, maxDay := holidays[0].Day, holidays[0].Day
	for _, h := range holidays[1:] {
		if h.Day.Before(minDay) {
			minDay = h.Day
		}
		if h.Day.After(maxDay) {
			maxDay = h.Day
		}
	}

	const (
		queryDelete = `DELETE FROM holidays WHERE day BETWEEN ? AND ?;`
		queryInsert = `INSERT OR REPLACE INTO holidays (day, title) VALUES (:day, :title);`
	)

	resultDelete, err := tx.ExecContext(ctx, queryDelete, minDay.StartOfYear(), maxDay.EndOfYear())
	if err != nil {
		return fmt.Errorf("delete existing holidays: %w", err)
	}

	rowsAffected, err := resultDelete.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected for delete holidays: %w", err)
	}
	slog.InfoContext(ctx, "deleted holidays", "rows", rowsAffected,
		"min_day", minDay.StartOfYear(), "max_day", maxDay.EndOfYear(),
	)

	resultInsert, err := tx.NamedExecContext(ctx, queryInsert, holidays)
	if err != nil {
		return fmt.Errorf("insert holidays: %w", err)
	}

	if rowsAffected, err = resultInsert.RowsAffected(); err != nil {
		return fmt.Errorf("get rows affected for insert holidays: %w", err)
	}
	slog.InfoContext(ctx, "inserted holidays", "rows", rowsAffected)

	return nil
}

// GetHolidays retrieves holidays for the specified year and location.
func (db *DB) GetHolidays(ctx context.Context, year int, location *time.Location) ([]Holiday, error) {
	day := DateOnly(time.Date(year, 1, 1, 0, 0, 0, 0, location))

	const query = `SELECT day, title FROM holidays WHERE day BETWEEN ? AND ? ORDER BY day;`
	var holidays []Holiday

	slog.DebugContext(ctx, "GetHolidays", "query", query, "start", day.StartOfYear(), "end", day.EndOfYear())
	err := db.SelectContext(ctx, &holidays, query, day.StartOfYear(), day.EndOfYear())

	if err != nil {
		return nil, fmt.Errorf("failed select holidays: %w", err)
	}

	for i := range holidays {
		holidays[i].Day.SetLocation(location)
	}

	return holidays, nil
}
