package schema

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// ScheduleFromRecurrence projects a domain RecurrenceRule into a schema.org
// Schedule suitable for JSON-LD output. Returns nil if the recurrence data is
// nil or cannot be projected (e.g. HOURLY/MINUTELY/SECONDLY frequencies).
func ScheduleFromRecurrence(rec *events.RecurrenceRule) *Schedule {
	if rec == nil || rec.RRule == "" {
		return nil
	}

	params := parseRRuleParams(rec.RRule)
	freq := strings.ToUpper(params["FREQ"])
	if freq == "" {
		return nil
	}

	repeatFreq, ok := freqToISO8601(freq, params)
	if !ok {
		slog.Warn("skipping eventSchedule projection: unsupported FREQ value", "freq", freq, "rrule", rec.RRule)
		return nil
	}

	s := &Schedule{
		AtType:          "Schedule",
		RepeatFrequency: repeatFreq,
	}

	if byDay, ok := params["BYDAY"]; ok {
		s.ByDay = expandByDay(byDay)
	}
	if byMD, ok := params["BYMONTHDAY"]; ok {
		s.ByMonthDay = parseByMonthDay(byMD)
	}
	if rec.TZID != "" {
		s.ScheduleTimezone = rec.TZID
	}

	return s
}

// FREQ → ISO 8601 duration mapping per spec-phase3.md Task 3:
//
//	DAILY → P1D, WEEKLY → P1W, MONTHLY → P1M, YEARLY → P1Y
//
// With INTERVAL=N, the number replaces 1 (e.g. INTERVAL=2;FREQ=WEEKLY → P2W).
// HOURLY/MINUTELY/SECONDLY are not expected in event data and are skipped with a warning.
func freqToISO8601(freq string, params map[string]string) (string, bool) {
	interval := 1
	if iv, ok := params["INTERVAL"]; ok {
		if n, err := parseInt(iv); err == nil && n > 0 {
			interval = n
		}
	}

	switch freq {
	case "DAILY":
		return fmt.Sprintf("P%dD", interval), true
	case "WEEKLY":
		return fmt.Sprintf("P%dW", interval), true
	case "MONTHLY":
		return fmt.Sprintf("P%dM", interval), true
	case "YEARLY":
		return fmt.Sprintf("P%dY", interval), true
	case "HOURLY", "MINUTELY", "SECONDLY":
		return "", false
	default:
		return "", false
	}
}

// expandByDay converts an RRULE BYDAY value (e.g. "MO,WE" or "2MO") into
// schema.org day-of-week strings. schema.org uses full day names
// (Monday, Tuesday, etc.) per https://schema.org/DayOfWeek.
func expandByDay(raw string) []string {
	dayMap := map[string]string{
		"MO": "Monday",
		"TU": "Tuesday",
		"WE": "Wednesday",
		"TH": "Thursday",
		"FR": "Friday",
		"SA": "Saturday",
		"SU": "Sunday",
	}

	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		var code string
		for i, ch := range p {
			if ch >= 'A' && ch <= 'Z' {
				code = p[i:]
				break
			}
		}
		if code == "" {
			code = p
		}
		if name, ok := dayMap[strings.ToUpper(code)]; ok {
			result = append(result, name)
		}
	}
	return result
}

// parseByMonthDay converts an RRULE BYMONTHDAY value (e.g. "15,-15") into
// a slice of ints.
func parseByMonthDay(raw string) []int {
	parts := strings.Split(raw, ",")
	var result []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if n, err := parseInt(p); err == nil {
			result = append(result, n)
		}
	}
	return result
}

// parseRRuleParams splits an RRULE value string like "FREQ=WEEKLY;BYDAY=MO,WE"
// into key-value pairs. The "RRULE:" prefix is stripped if present.
func parseRRuleParams(rrule string) map[string]string {
	result := make(map[string]string)
	raw := strings.TrimPrefix(rrule, "RRULE:")
	for _, part := range strings.Split(raw, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			result[strings.ToUpper(strings.TrimSpace(kv[0]))] = kv[1]
		}
	}
	return result
}

func parseInt(s string) (int, error) {
	var n int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		} else if ch == '-' && n == 0 {
			negative := true
			rest := s[1:]
			v, err := parseInt(rest)
			if err != nil {
				return 0, fmt.Errorf("parseInt %q: %w", s, err)
			}
			if negative {
				return -v, nil
			}
			return v, nil
		} else {
			return 0, fmt.Errorf("parseInt %q: unexpected char %c", s, ch)
		}
	}
	return n, nil
}
