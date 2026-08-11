// Package cronparser implements a minimal, dependency-free parser for the
// standard 5-field cron expression format:
//
//	minute(0-59) hour(0-23) day-of-month(1-31) month(1-12) day-of-week(0-6, 0=Sunday)
//
// Supported syntax per field: "*", a single value, comma lists ("1,2,5"),
// ranges ("1-5"), and steps ("*/15", "1-30/5"). These four forms cover the
// vast majority of real-world cron expressions.
//
// It's written by hand (no external cron library) so the orchestrator has
// zero third-party dependencies for its core scheduling logic — and so the
// "how does a cron field actually expand" question has a real answer.
package cronparser

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Schedule struct {
	minute  map[int]bool
	hour    map[int]bool
	dom     map[int]bool // day of month
	month   map[int]bool
	dow     map[int]bool // day of week, 0=Sunday
	domStar bool         // true if day-of-month field was "*"
	dowStar bool         // true if day-of-week field was "*"
}

// Parse validates and compiles a 5-field cron expression.
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields (minute hour dom month dow), got %d: %q", len(fields), expr)
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}

	return &Schedule{
		minute:  minute,
		hour:    hour,
		dom:     dom,
		month:   month,
		dow:     dow,
		domStar: strings.TrimSpace(fields[2]) == "*",
		dowStar: strings.TrimSpace(fields[4]) == "*",
	}, nil
}

// parseField expands one cron field (e.g. "*/15", "1-5,10") into a set of
// matching integer values within [min, max].
func parseField(field string, min, max int) (map[int]bool, error) {
	result := make(map[int]bool)

	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty segment in field %q", field)
		}

		step := 1
		base := part
		if idx := strings.Index(part, "/"); idx != -1 {
			base = part[:idx]
			s, err := strconv.Atoi(part[idx+1:])
			if err != nil || s <= 0 {
				return nil, fmt.Errorf("invalid step in %q", part)
			}
			step = s
		}

		var lo, hi int
		switch {
		case base == "*":
			lo, hi = min, max
		case strings.Contains(base, "-"):
			bounds := strings.SplitN(base, "-", 2)
			l, err1 := strconv.Atoi(bounds[0])
			h, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range %q", base)
			}
			lo, hi = l, h
		default:
			v, err := strconv.Atoi(base)
			if err != nil {
				return nil, fmt.Errorf("invalid value %q", base)
			}
			lo, hi = v, v
		}

		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("value out of range [%d-%d] in %q", min, max, part)
		}

		for v := lo; v <= hi; v += step {
			result[v] = true
		}
	}

	return result, nil
}

// Next returns the next time strictly after `from` that matches the
// schedule, truncated to whole minutes. It searches forward minute by
// minute, capped at two years out as a safety valve against malformed
// schedules that never match (e.g. Feb 30).
func (s *Schedule) Next(from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.AddDate(2, 0, 0)

	for t.Before(limit) {
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	// Should not happen for valid expressions; return far future as a guard.
	return limit
}

func (s *Schedule) matches(t time.Time) bool {
	if !s.minute[t.Minute()] || !s.hour[t.Hour()] || !s.month[int(t.Month())] {
		return false
	}

	dom := t.Day()
	dow := int(t.Weekday())

	// Standard cron quirk: when BOTH day-of-month and day-of-week are
	// restricted, a match on either is sufficient (OR). When only one is
	// restricted, that one alone must match.
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return s.dow[dow]
	case s.dowStar:
		return s.dom[dom]
	default:
		return s.dom[dom] || s.dow[dow]
	}
}
