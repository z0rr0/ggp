package predictor

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/z0rr0/ggp/databaser"
)

func TestDayType(t *testing.T) {
	tests := []struct {
		name string
		want DayType
	}{
		{"Sunday", Sunday},
		{"Monday", Monday},
		{"Tuesday", Tuesday},
		{"Wednesday", Wednesday},
		{"Thursday", Thursday},
		{"Friday", Friday},
		{"Saturday", Saturday},
		{"Holiday", Holiday},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if DayType(i) != tt.want {
				t.Errorf("DayType(%d) = %v, want %v", i, DayType(i), tt.want)
			}
		})
	}
}

func TestNewRussianHolidayChecker(t *testing.T) {
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	location := time.UTC
	checker, err := NewRussianHolidayChecker(ctx, db, location)
	if err != nil {
		t.Fatalf("NewRussianHolidayChecker() error = %v", err)
	}

	if checker == nil {
		t.Fatal("expected non-nil checker")
	}

	if checker.fixedHolidays == nil {
		t.Error("fixedHolidays map not initialized")
	}
}

func TestNewRussianHolidayChecker_WithHolidays(t *testing.T) {
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	year, _, _ := time.Now().In(time.UTC).Date()
	day1 := databaser.DateOnly(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC))
	day2 := databaser.DateOnly(time.Date(year, 5, 1, 0, 0, 0, 0, time.UTC))
	holidays := []databaser.Holiday{
		{
			Day:     &day1,
			Title:   "New Year",
			Created: time.Now().UTC(),
		},
		{
			Day:     &day2,
			Title:   "Labor Day",
			Created: time.Now().UTC(),
		},
	}

	err = databaser.InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return databaser.SaveManyHolidaysTx(ctx, tx, holidays)
	})
	if err != nil {
		t.Fatalf("failed to add holidays: %v", err)
	}

	checker, err := NewRussianHolidayChecker(ctx, db, time.UTC)
	if err != nil {
		t.Fatalf("NewRussianHolidayChecker() error = %v", err)
	}

	if len(checker.fixedHolidays) != len(holidays) {
		t.Errorf("loaded %d holidays, want %d", len(checker.fixedHolidays), len(holidays))
	}
}

func TestRussianHolidayChecker_IsHoliday(t *testing.T) {
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	year, _, _ := time.Now().In(time.UTC).Date()
	holidayDate := databaser.DateOnly(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC))
	holidays := []databaser.Holiday{
		{
			Day:     &holidayDate,
			Title:   "New Year",
			Created: time.Now().UTC(),
		},
	}

	err = databaser.InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return databaser.SaveManyHolidaysTx(ctx, tx, holidays)
	})
	if err != nil {
		t.Fatalf("failed to add holiday: %v", err)
	}

	checker, err := NewRussianHolidayChecker(ctx, db, time.UTC)
	if err != nil {
		t.Fatalf("NewRussianHolidayChecker() error = %v", err)
	}

	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{
			name: "holiday date",
			date: time.Date(year, 1, 1, 12, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "regular date",
			date: time.Date(year, 2, 15, 12, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "holiday different year",
			date: time.Date(year+1, 1, 1, 12, 0, 0, 0, time.UTC),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.IsHoliday(tt.date)
			if got != tt.want {
				t.Errorf("IsHoliday(%v) = %v, want %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestRussianHolidayChecker_HolidayTitle(t *testing.T) {
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	year, _, _ := time.Now().In(time.UTC).Date()
	holidayDate := databaser.DateOnly(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC))
	title := "New Year"
	holidays := []databaser.Holiday{
		{
			Day:     &holidayDate,
			Title:   title,
			Created: time.Now().UTC(),
		},
	}

	err = databaser.InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return databaser.SaveManyHolidaysTx(ctx, tx, holidays)
	})
	if err != nil {
		t.Fatalf("failed to add holiday: %v", err)
	}

	checker, err := NewRussianHolidayChecker(ctx, db, time.UTC)
	if err != nil {
		t.Fatalf("NewRussianHolidayChecker() error = %v", err)
	}

	tests := []struct {
		name string
		date time.Time
		want string
	}{
		{
			name: "holiday date",
			date: time.Date(year, 1, 1, 12, 0, 0, 0, time.UTC),
			want: title,
		},
		{
			name: "regular date",
			date: time.Date(year, 2, 15, 12, 0, 0, 0, time.UTC),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.HolidayTitle(tt.date)
			if got != tt.want {
				t.Errorf("HolidayTitle(%v) = %v, want %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestMonthDay(t *testing.T) {
	tests := []struct {
		name  string
		month uint8
		day   uint8
	}{
		{"January 1st", 1, 1},
		{"February 29th", 2, 29},
		{"December 31st", 12, 31},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := monthDay{month: tt.month, day: tt.day}
			if md.month != tt.month {
				t.Errorf("month = %d, want %d", md.month, tt.month)
			}
			if md.day != tt.day {
				t.Errorf("day = %d, want %d", md.day, tt.day)
			}
		})
	}
}

func TestRussianHolidayChecker_MultipleYears(t *testing.T) {
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	year, _, _ := time.Now().In(time.UTC).Date()
	holidayDate := databaser.DateOnly(time.Date(year, 5, 9, 0, 0, 0, 0, time.UTC))
	holidays := []databaser.Holiday{
		{
			Day:     &holidayDate,
			Title:   "Victory Day",
			Created: time.Now().UTC(),
		},
	}

	err = databaser.InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		return databaser.SaveManyHolidaysTx(ctx, tx, holidays)
	})
	if err != nil {
		t.Fatalf("failed to add holiday: %v", err)
	}

	checker, err := NewRussianHolidayChecker(ctx, db, time.UTC)
	if err != nil {
		t.Fatalf("NewRussianHolidayChecker() error = %v", err)
	}

	testYears := []int{year - 1, year, year + 1}
	for _, y := range testYears {
		date := time.Date(y, 5, 9, 12, 0, 0, 0, time.UTC)
		if !checker.IsHoliday(date) {
			t.Errorf("IsHoliday(%v) = false, want true (year-independent check)", date)
		}

		title := checker.HolidayTitle(date)
		if title != "Victory Day" {
			t.Errorf("HolidayTitle(%v) = %v, want 'Victory Day'", date, title)
		}
	}
}

func TestRussianHolidayChecker_EmptyDatabase(t *testing.T) {
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	checker, err := NewRussianHolidayChecker(ctx, db, time.UTC)
	if err != nil {
		t.Fatalf("NewRussianHolidayChecker() error = %v", err)
	}

	testDate := time.Now().UTC()
	if checker.IsHoliday(testDate) {
		t.Errorf("IsHoliday() = true for empty database, want false")
	}

	if title := checker.HolidayTitle(testDate); title != "" {
		t.Errorf("HolidayTitle() = %v for empty database, want empty string", title)
	}
}
