// Package ical разбирает и формирует календари iCalendar (RFC 5545).
//
// Поддерживается подмножество, достаточное для учебных расписаний:
// VEVENT с DTSTART/DTEND/DURATION, SUMMARY, LOCATION, RRULE
// (FREQ=DAILY|WEEKLY, INTERVAL, COUNT, UNTIL, BYDAY) и EXDATE.
// События на весь день (VALUE=DATE) пропускаются: занятиями они не являются.
package ical

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Event — событие календаря до развёртки повторений.
type Event struct {
	UID      string
	Summary  string
	Location string
	Start    time.Time
	Duration time.Duration
	Rule     *Rule
	ExDates  []time.Time
}

// Freq — частота повторения.
type Freq string

const (
	FreqDaily  Freq = "DAILY"
	FreqWeekly Freq = "WEEKLY"
)

// Rule — правило повторения RRULE.
type Rule struct {
	Freq     Freq
	Interval int
	Count    int       // 0 — не ограничено
	Until    time.Time // нулевое — не ограничено
	ByDay    []time.Weekday
}

// ErrUnsupported возвращается для конструкций, которые пакет не поддерживает.
var ErrUnsupported = errors.New("неподдерживаемая конструкция iCalendar")

// property — строка содержимого: имя, параметры и значение.
type property struct {
	name   string
	params map[string]string
	value  string
}

// Parse читает календарь. defaultLoc используется для времени без TZID и без Z.
func Parse(r io.Reader, defaultLoc *time.Location) ([]Event, error) {
	lines, err := unfold(r)
	if err != nil {
		return nil, err
	}

	var (
		events  []Event
		cur     *Event
		hasEnd  bool
		end     time.Time
		allDay  bool
		lineNum int
	)
	for _, line := range lines {
		lineNum++
		if line == "" {
			continue
		}
		p, err := parseProperty(line)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", lineNum, err)
		}

		switch {
		case p.name == "BEGIN" && p.value == "VEVENT":
			cur, hasEnd, allDay = &Event{}, false, false
			continue
		case p.name == "END" && p.value == "VEVENT":
			if cur == nil {
				return nil, fmt.Errorf("строка %d: END:VEVENT без BEGIN", lineNum)
			}
			if !allDay {
				if cur.Start.IsZero() {
					return nil, fmt.Errorf("событие %q: нет DTSTART", cur.Summary)
				}
				if hasEnd {
					cur.Duration = end.Sub(cur.Start)
				}
				if cur.Duration <= 0 {
					return nil, fmt.Errorf("событие %q: неположительная длительность", cur.Summary)
				}
				events = append(events, *cur)
			}
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}

		switch p.name {
		case "UID":
			cur.UID = p.value
		case "SUMMARY":
			cur.Summary = unescapeText(p.value)
		case "LOCATION":
			cur.Location = unescapeText(p.value)
		case "DTSTART":
			if p.params["VALUE"] == "DATE" {
				allDay = true
				continue
			}
			if cur.Start, err = parseDateTime(p.value, p.params["TZID"], defaultLoc); err != nil {
				return nil, fmt.Errorf("строка %d: DTSTART: %w", lineNum, err)
			}
		case "DTEND":
			if p.params["VALUE"] == "DATE" {
				allDay = true
				continue
			}
			if end, err = parseDateTime(p.value, p.params["TZID"], defaultLoc); err != nil {
				return nil, fmt.Errorf("строка %d: DTEND: %w", lineNum, err)
			}
			hasEnd = true
		case "DURATION":
			if cur.Duration, err = parseDuration(p.value); err != nil {
				return nil, fmt.Errorf("строка %d: DURATION: %w", lineNum, err)
			}
		case "RRULE":
			if cur.Rule, err = parseRule(p.value, defaultLoc); err != nil {
				return nil, fmt.Errorf("строка %d: RRULE: %w", lineNum, err)
			}
		case "EXDATE":
			for _, v := range strings.Split(p.value, ",") {
				t, err := parseDateTime(v, p.params["TZID"], defaultLoc)
				if err != nil {
					return nil, fmt.Errorf("строка %d: EXDATE: %w", lineNum, err)
				}
				cur.ExDates = append(cur.ExDates, t)
			}
		}
	}
	if cur != nil {
		return nil, errors.New("незакрытый VEVENT")
	}
	return events, nil
}

// unfold склеивает перенесённые строки: строка, начинающаяся с пробела
// или табуляции, продолжает предыдущую (RFC 5545, раздел 3.1).
func unfold(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("чтение календаря: %w", err)
	}
	return lines, nil
}

// parseProperty разбирает «NAME;PARAM=VAL;PARAM2="VAL":VALUE».
func parseProperty(line string) (property, error) {
	// Двоеточие внутри кавычек в параметрах не считается разделителем.
	inQuotes := false
	colon := -1
	for i, ch := range line {
		if ch == '"' {
			inQuotes = !inQuotes
		}
		if ch == ':' && !inQuotes {
			colon = i
			break
		}
	}
	if colon < 0 {
		return property{}, fmt.Errorf("нет двоеточия в %q", line)
	}
	head, value := line[:colon], line[colon+1:]
	parts := strings.Split(head, ";")
	p := property{name: strings.ToUpper(parts[0]), params: map[string]string{}, value: value}
	for _, param := range parts[1:] {
		k, v, ok := strings.Cut(param, "=")
		if !ok {
			continue
		}
		p.params[strings.ToUpper(k)] = strings.Trim(v, `"`)
	}
	return p, nil
}

// parseDateTime понимает форматы 20260901T083000Z, 20260901T083000 (+TZID) и 20260901.
func parseDateTime(v, tzid string, defaultLoc *time.Location) (time.Time, error) {
	v = strings.TrimSpace(v)
	if strings.HasSuffix(v, "Z") {
		return time.Parse("20060102T150405Z", v)
	}
	loc := defaultLoc
	if tzid != "" {
		l, err := time.LoadLocation(tzid)
		if err != nil {
			return time.Time{}, fmt.Errorf("неизвестный часовой пояс %q", tzid)
		}
		loc = l
	}
	if len(v) == len("20060102") {
		return time.ParseInLocation("20060102", v, loc)
	}
	return time.ParseInLocation("20060102T150405", v, loc)
}

// parseDuration разбирает длительность вида P1W, P1DT2H, PT1H30M.
func parseDuration(v string) (time.Duration, error) {
	neg := strings.HasPrefix(v, "-")
	v = strings.TrimLeft(v, "+-")
	if !strings.HasPrefix(v, "P") {
		return 0, fmt.Errorf("ожидается P в начале: %q", v)
	}
	v = v[1:]
	var d time.Duration
	inTime := false
	num := ""
	for _, ch := range v {
		switch {
		case ch >= '0' && ch <= '9':
			num += string(ch)
			continue
		case ch == 'T':
			inTime = true
			continue
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			return 0, fmt.Errorf("некорректная длительность %q", v)
		}
		num = ""
		switch {
		case ch == 'W' && !inTime:
			d += time.Duration(n) * 7 * 24 * time.Hour
		case ch == 'D' && !inTime:
			d += time.Duration(n) * 24 * time.Hour
		case ch == 'H' && inTime:
			d += time.Duration(n) * time.Hour
		case ch == 'M' && inTime:
			d += time.Duration(n) * time.Minute
		case ch == 'S' && inTime:
			d += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("некорректная длительность %q", v)
		}
	}
	if num != "" {
		return 0, fmt.Errorf("некорректная длительность %q", v)
	}
	if neg {
		d = -d
	}
	return d, nil
}

var weekdays = map[string]time.Weekday{
	"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
}

func parseRule(v string, defaultLoc *time.Location) (*Rule, error) {
	r := &Rule{Interval: 1}
	for _, part := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("некорректная часть %q", part)
		}
		switch strings.ToUpper(k) {
		case "FREQ":
			r.Freq = Freq(strings.ToUpper(val))
			if r.Freq != FreqDaily && r.Freq != FreqWeekly {
				return nil, fmt.Errorf("FREQ=%s: %w", val, ErrUnsupported)
			}
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("некорректный INTERVAL %q", val)
			}
			r.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("некорректный COUNT %q", val)
			}
			r.Count = n
		case "UNTIL":
			t, err := parseDateTime(val, "", defaultLoc)
			if err != nil {
				return nil, fmt.Errorf("UNTIL: %w", err)
			}
			if len(val) == len("20060102") {
				// UNTIL в виде даты включает весь этот день.
				t = t.AddDate(0, 0, 1).Add(-time.Second)
			}
			r.Until = t
		case "BYDAY":
			for _, d := range strings.Split(val, ",") {
				wd, ok := weekdays[strings.ToUpper(d)]
				if !ok {
					// Формы вроде 1MO (первый понедельник месяца) не нужны для учебных расписаний.
					return nil, fmt.Errorf("BYDAY=%s: %w", d, ErrUnsupported)
				}
				r.ByDay = append(r.ByDay, wd)
			}
		case "WKST":
			// Неделя всегда начинается с понедельника.
		default:
			return nil, fmt.Errorf("%s: %w", k, ErrUnsupported)
		}
	}
	if r.Freq == "" {
		return nil, errors.New("нет FREQ")
	}
	if r.Freq == FreqDaily && len(r.ByDay) > 0 {
		return nil, fmt.Errorf("BYDAY вместе с FREQ=DAILY: %w", ErrUnsupported)
	}
	return r, nil
}

func unescapeText(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return r.Replace(s)
}
