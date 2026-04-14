package schema

import (
	"encoding/json"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

func TestScheduleFromRecurrence_NilInput(t *testing.T) {
	if result := ScheduleFromRecurrence(nil); result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestScheduleFromRecurrence_EmptyRRule(t *testing.T) {
	rec := &events.RecurrenceRule{RRule: ""}
	if result := ScheduleFromRecurrence(rec); result != nil {
		t.Errorf("expected nil for empty RRULE, got %v", result)
	}
}

func TestScheduleFromRecurrence_Daily(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=DAILY",
		TZID:  "America/Toronto",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if s.AtType != "Schedule" {
		t.Errorf("AtType: got %q, want %q", s.AtType, "Schedule")
	}
	if s.RepeatFrequency != "P1D" {
		t.Errorf("RepeatFrequency: got %q, want %q", s.RepeatFrequency, "P1D")
	}
	if s.ScheduleTimezone != "America/Toronto" {
		t.Errorf("ScheduleTimezone: got %q, want %q", s.ScheduleTimezone, "America/Toronto")
	}
}

func TestScheduleFromRecurrence_WeeklyWithByDay(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=WEEKLY;BYDAY=MO,WE",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if s.RepeatFrequency != "P1W" {
		t.Errorf("RepeatFrequency: got %q, want %q", s.RepeatFrequency, "P1W")
	}
	if len(s.ByDay) != 2 {
		t.Fatalf("ByDay length: got %d, want 2", len(s.ByDay))
	}
	if s.ByDay[0] != "Monday" {
		t.Errorf("ByDay[0]: got %q, want %q", s.ByDay[0], "Monday")
	}
	if s.ByDay[1] != "Wednesday" {
		t.Errorf("ByDay[1]: got %q, want %q", s.ByDay[1], "Wednesday")
	}
}

func TestScheduleFromRecurrence_MonthlyWithInterval(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=MONTHLY;INTERVAL=3",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if s.RepeatFrequency != "P3M" {
		t.Errorf("RepeatFrequency: got %q, want %q", s.RepeatFrequency, "P3M")
	}
}

func TestScheduleFromRecurrence_Yearly(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=YEARLY",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if s.RepeatFrequency != "P1Y" {
		t.Errorf("RepeatFrequency: got %q, want %q", s.RepeatFrequency, "P1Y")
	}
}

func TestScheduleFromRecurrence_HourlySkipped(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=HOURLY",
	}
	s := ScheduleFromRecurrence(rec)
	if s != nil {
		t.Errorf("expected nil Schedule for HOURLY, got %v", s)
	}
}

func TestScheduleFromRecurrence_MinutelySkipped(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=MINUTELY",
	}
	if s := ScheduleFromRecurrence(rec); s != nil {
		t.Errorf("expected nil for MINUTELY, got %v", s)
	}
}

func TestScheduleFromRecurrence_SecondlySkipped(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=SECONDLY",
	}
	if s := ScheduleFromRecurrence(rec); s != nil {
		t.Errorf("expected nil for SECONDLY, got %v", s)
	}
}

func TestScheduleFromRecurrence_ByMonthDay(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=MONTHLY;BYMONTHDAY=15,-15",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if len(s.ByMonthDay) != 2 {
		t.Fatalf("ByMonthDay length: got %d, want 2", len(s.ByMonthDay))
	}
	if s.ByMonthDay[0] != 15 {
		t.Errorf("ByMonthDay[0]: got %d, want 15", s.ByMonthDay[0])
	}
	if s.ByMonthDay[1] != -15 {
		t.Errorf("ByMonthDay[1]: got %d, want -15", s.ByMonthDay[1])
	}
}

func TestScheduleFromRecurrence_WeeklyWithInterval(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "FREQ=WEEKLY;INTERVAL=2;BYDAY=FR",
		TZID:  "Europe/London",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if s.RepeatFrequency != "P2W" {
		t.Errorf("RepeatFrequency: got %q, want %q", s.RepeatFrequency, "P2W")
	}
	if len(s.ByDay) != 1 || s.ByDay[0] != "Friday" {
		t.Errorf("ByDay: got %v, want [Friday]", s.ByDay)
	}
	if s.ScheduleTimezone != "Europe/London" {
		t.Errorf("ScheduleTimezone: got %q, want %q", s.ScheduleTimezone, "Europe/London")
	}
}

func TestScheduleFromRecurrence_NoFreqKey(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "BYDAY=MO,WE",
	}
	if s := ScheduleFromRecurrence(rec); s != nil {
		t.Errorf("expected nil for RRULE without FREQ, got %v", s)
	}
}

func TestScheduleFromRecurrence_WithRRULEPrefix(t *testing.T) {
	rec := &events.RecurrenceRule{
		RRule: "RRULE:FREQ=WEEKLY;BYDAY=TU",
	}
	s := ScheduleFromRecurrence(rec)
	if s == nil {
		t.Fatal("expected non-nil Schedule")
	}
	if s.RepeatFrequency != "P1W" {
		t.Errorf("RepeatFrequency: got %q, want %q", s.RepeatFrequency, "P1W")
	}
	if len(s.ByDay) != 1 || s.ByDay[0] != "Tuesday" {
		t.Errorf("ByDay: got %v, want [Tuesday]", s.ByDay)
	}
}

func TestFreqToISO8601(t *testing.T) {
	tests := []struct {
		freq     string
		interval string
		want     string
		ok       bool
	}{
		{"DAILY", "", "P1D", true},
		{"WEEKLY", "", "P1W", true},
		{"MONTHLY", "", "P1M", true},
		{"YEARLY", "", "P1Y", true},
		{"DAILY", "2", "P2D", true},
		{"WEEKLY", "3", "P3W", true},
		{"MONTHLY", "6", "P6M", true},
		{"YEARLY", "2", "P2Y", true},
		{"HOURLY", "", "", false},
		{"MINUTELY", "", "", false},
		{"SECONDLY", "", "", false},
		{"UNKNOWN", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.freq+"/int="+tt.interval, func(t *testing.T) {
			params := map[string]string{"FREQ": tt.freq}
			if tt.interval != "" {
				params["INTERVAL"] = tt.interval
			}
			got, ok := freqToISO8601(tt.freq, params)
			if ok != tt.ok {
				t.Errorf("ok: got %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScheduleOmittedWhenNil(t *testing.T) {
	e := NewEvent("Non-recurring Event")
	data, err := marshalToMap(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, ok := data["eventSchedule"]; ok {
		t.Error("expected eventSchedule to be omitted when nil")
	}
}

func TestSchedulePresentWhenSet(t *testing.T) {
	e := NewEvent("Recurring Event")
	e.EventSchedule = &Schedule{
		AtType:          "Schedule",
		RepeatFrequency: "P1W",
		ByDay:           []string{"Monday"},
	}
	data, err := marshalToMap(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	es, ok := data["eventSchedule"].(map[string]any)
	if !ok {
		t.Fatalf("eventSchedule not a map: %T %v", data["eventSchedule"], data["eventSchedule"])
	}
	if es["@type"] != "Schedule" {
		t.Errorf("@type: got %v, want Schedule", es["@type"])
	}
	if es["repeatFrequency"] != "P1W" {
		t.Errorf("repeatFrequency: got %v, want P1W", es["repeatFrequency"])
	}
}

func marshalToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
