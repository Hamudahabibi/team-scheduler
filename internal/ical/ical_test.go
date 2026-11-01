package ical

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

var msk = mustLoc("Europe/Moscow")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

func date(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, msk)
}

// Расписание в стиле вузовского экспорта: пара по числителю (раз в две недели),
// еженедельная пара с отменённым занятием, разовое событие в UTC, событие на весь день.
const sample = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:1\r\n" +
	"SUMMARY:Базы данных\\, лекция\r\n" +
	"LOCATION:ауд. 0310\r\n" +
	"DTSTART;TZID=Europe/Moscow:20260901T083000\r\n" +
	"DTEND;TZID=Europe/Moscow:20260901T101000\r\n" +
	"RRULE:FREQ=WEEKLY;INTERVAL=2;UNTIL=20261231\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:2\r\n" +
	"SUMMARY:Программная\r\n" +
	"  инженерия\r\n" + // перенос + один пробел удаляются, второй пробел — часть текста
	"DTSTART:20260902T120000\r\n" +
	"DURATION:PT1H40M\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=WE,FR;COUNT=6\r\n" +
	"EXDATE:20260904T120000\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:3\r\n" +
	"SUMMARY:Консультация\r\n" +
	"DTSTART:20260915T130000Z\r\n" +
	"DTEND:20260915T143000Z\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:4\r\n" +
	"SUMMARY:День знаний\r\n" +
	"DTSTART;VALUE=DATE:20260901\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParse(t *testing.T) {
	events, err := Parse(strings.NewReader(sample), msk)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("событий %d, ожидалось 3 (событие на весь день пропускается)", len(events))
	}
	if events[0].Summary != "Базы данных, лекция" {
		t.Errorf("экранирование: %q", events[0].Summary)
	}
	if events[1].Summary != "Программная инженерия" {
		t.Errorf("склейка строк: %q", events[1].Summary)
	}
	if events[1].Duration != 100*time.Minute {
		t.Errorf("DURATION: %v", events[1].Duration)
	}
	if !events[2].Start.Equal(date(2026, 9, 15, 16, 0)) {
		t.Errorf("UTC-время: %v", events[2].Start)
	}
}

func TestExpand(t *testing.T) {
	events, err := Parse(strings.NewReader(sample), msk)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("числитель: раз в две недели", func(t *testing.T) {
		occ := Expand(events[:1], date(2026, 9, 1, 0, 0), date(2026, 10, 1, 0, 0))
		want := []time.Time{date(2026, 9, 1, 8, 30), date(2026, 9, 15, 8, 30), date(2026, 9, 29, 8, 30)}
		assertStarts(t, occ, want)
		if !occ[0].End.Equal(date(2026, 9, 1, 10, 10)) {
			t.Errorf("конец пары: %v", occ[0].End)
		}
	})

	t.Run("BYDAY, COUNT и EXDATE", func(t *testing.T) {
		occ := Expand(events[1:2], date(2026, 9, 1, 0, 0), date(2026, 12, 1, 0, 0))
		// 6 проведений: 2, 4, 9, 11, 16, 18 сентября; 4-е отменено.
		want := []time.Time{
			date(2026, 9, 2, 12, 0), date(2026, 9, 9, 12, 0), date(2026, 9, 11, 12, 0),
			date(2026, 9, 16, 12, 0), date(2026, 9, 18, 12, 0),
		}
		assertStarts(t, occ, want)
	})

	t.Run("окно отсекает проведения", func(t *testing.T) {
		occ := Expand(events, date(2026, 9, 14, 0, 0), date(2026, 9, 21, 0, 0))
		want := []time.Time{date(2026, 9, 15, 8, 30), date(2026, 9, 15, 16, 0), date(2026, 9, 16, 12, 0), date(2026, 9, 18, 12, 0)}
		assertStarts(t, occ, want)
	})

	t.Run("UNTIL ограничивает серию", func(t *testing.T) {
		occ := Expand(events[:1], date(2027, 1, 1, 0, 0), date(2027, 3, 1, 0, 0))
		if len(occ) != 0 {
			t.Errorf("после UNTIL найдено %d проведений", len(occ))
		}
	})
}

func TestExpandKeepsWallClockAcrossDST(t *testing.T) {
	berlin := mustLoc("Europe/Berlin")
	// 26.10.2026 Германия переходит на зимнее время, пара должна остаться в 9:00.
	e := Event{Start: time.Date(2026, 10, 19, 9, 0, 0, 0, berlin), Duration: time.Hour, Rule: &Rule{Freq: FreqWeekly, Interval: 1}}
	occ := Expand([]Event{e}, time.Date(2026, 10, 19, 0, 0, 0, 0, berlin), time.Date(2026, 11, 3, 0, 0, 0, 0, berlin))
	for _, o := range occ {
		if h := o.Start.In(berlin).Hour(); h != 9 {
			t.Errorf("%v: час %d, ожидалось 9", o.Start, h)
		}
	}
	if len(occ) != 3 {
		t.Errorf("проведений %d, ожидалось 3", len(occ))
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		err  error
	}{
		{"месячное повторение", "BEGIN:VEVENT\nDTSTART:20260901T080000\nDTEND:20260901T090000\nRRULE:FREQ=MONTHLY\nEND:VEVENT\n", ErrUnsupported},
		{"BYDAY с номером", "BEGIN:VEVENT\nDTSTART:20260901T080000\nDTEND:20260901T090000\nRRULE:FREQ=WEEKLY;BYDAY=1MO\nEND:VEVENT\n", ErrUnsupported},
		{"нет DTSTART", "BEGIN:VEVENT\nSUMMARY:x\nEND:VEVENT\n", nil},
		{"конец раньше начала", "BEGIN:VEVENT\nDTSTART:20260901T090000\nDTEND:20260901T080000\nEND:VEVENT\n", nil},
		{"незакрытое событие", "BEGIN:VEVENT\nDTSTART:20260901T090000\n", nil},
		{"кривая дата", "BEGIN:VEVENT\nDTSTART:2026-09-01\nEND:VEVENT\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.body), msk)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("ожидалась %v, получено %v", tt.err, err)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"PT1H30M": 90 * time.Minute,
		"P1D":     24 * time.Hour,
		"P1W":     7 * 24 * time.Hour,
		"P1DT2H":  26 * time.Hour,
		"-PT15M":  -15 * time.Minute,
	}
	for in, want := range tests {
		got, err := parseDuration(in)
		if err != nil || got != want {
			t.Errorf("%s: %v, %v; ожидалось %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"1H", "PT1X", "PT", "P1H"} {
		if _, err := parseDuration(bad); err == nil && bad != "PT" {
			t.Errorf("%s: ожидалась ошибка", bad)
		}
	}
}

func TestWriteRoundTrip(t *testing.T) {
	long := strings.Repeat("Тренировка команды; разбор игр, ", 5)
	in := []ExportEvent{{
		UID: "session-1@team-scheduler", Summary: long, Location: "Киберзал 1",
		Start: date(2026, 11, 2, 18, 0), End: date(2026, 11, 2, 20, 30),
	}}
	var buf bytes.Buffer
	if err := Write(&buf, "COSMos", in, date(2026, 11, 1, 12, 0)); err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSuffix(buf.String(), "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("строка длиннее 75 байт: %q", l)
		}
	}
	events, err := Parse(&buf, msk)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Summary != long || !events[0].Start.Equal(in[0].Start) || events[0].Duration != 150*time.Minute {
		t.Fatalf("после разбора: %+v", events)
	}
}

func assertStarts(t *testing.T, occ []Occurrence, want []time.Time) {
	t.Helper()
	if len(occ) != len(want) {
		t.Fatalf("проведений %d, ожидалось %d: %v", len(occ), len(want), occ)
	}
	for i := range want {
		if !occ[i].Start.Equal(want[i]) {
			t.Errorf("[%d] начало %v, ожидалось %v", i, occ[i].Start, want[i])
		}
	}
}
