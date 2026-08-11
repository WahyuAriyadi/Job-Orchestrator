package cronparser

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, expr string) *Schedule {
	t.Helper()
	s, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", expr, err)
	}
	return s
}

func at(t *testing.T, layout string) time.Time {
	tt, err := time.Parse("2006-01-02 15:04", layout)
	if err != nil {
		t.Fatalf("bad test time %q: %v", layout, err)
	}
	return tt
}

func TestEveryMinute(t *testing.T) {
	s := mustParse(t, "* * * * *")
	from := at(t, "2026-08-10 10:00")
	got := s.Next(from)
	want := at(t, "2026-08-10 10:01")
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEvery15Minutes(t *testing.T) {
	s := mustParse(t, "*/15 * * * *")
	got := s.Next(at(t, "2026-08-10 10:05"))
	want := at(t, "2026-08-10 10:15")
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDailyAtSpecificTime(t *testing.T) {
	s := mustParse(t, "30 9 * * *") // 09:30 every day
	got := s.Next(at(t, "2026-08-10 10:00"))
	want := at(t, "2026-08-11 09:30")
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWeekdaysOnly(t *testing.T) {
	s := mustParse(t, "0 9 * * 1-5") // 09:00 Mon-Fri
	// 2026-08-08 is a Saturday.
	got := s.Next(at(t, "2026-08-08 00:00"))
	want := at(t, "2026-08-10 09:00") // next Monday
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCommaList(t *testing.T) {
	s := mustParse(t, "0 8,12,18 * * *")
	got := s.Next(at(t, "2026-08-10 09:00"))
	want := at(t, "2026-08-10 12:00")
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDomDowOrLogic(t *testing.T) {
	// "1st of the month OR any Friday" - the standard (slightly surprising)
	// cron behavior when both day-of-month and day-of-week are restricted.
	s := mustParse(t, "0 0 1 * 5")
	// 2026-08-10 is a Monday; the next Friday is 2026-08-14, before the 1st of Sept.
	got := s.Next(at(t, "2026-08-10 00:00"))
	want := at(t, "2026-08-14 00:00")
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestInvalidExpressions(t *testing.T) {
	cases := []string{
		"* * * *",     // too few fields
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"* * * 13 *",  // month out of range
		"abc * * * *", // non-numeric
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", c)
		}
	}
}
