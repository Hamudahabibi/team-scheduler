package ical

import (
	"slices"
	"time"
)

// Occurrence — конкретное проведение события.
type Occurrence struct {
	Summary  string
	Location string
	Start    time.Time
	End      time.Time
}

// maxOccurrences защищает от бесконечной развёртки при ошибочных данных.
const maxOccurrences = 5000

// Expand разворачивает события в проведения, пересекающие [from, to).
// Результат отсортирован по времени начала.
func Expand(events []Event, from, to time.Time) []Occurrence {
	var res []Occurrence
	for _, e := range events {
		for _, start := range e.starts(to) {
			end := start.Add(e.Duration)
			if start.Before(to) && end.After(from) {
				res = append(res, Occurrence{Summary: e.Summary, Location: e.Location, Start: start, End: end})
			}
		}
	}
	slices.SortFunc(res, func(a, b Occurrence) int { return a.Start.Compare(b.Start) })
	return res
}

// starts возвращает моменты начала проведений, начинающихся раньше limit.
// Время суток сохраняется по «настенным часам» часового пояса DTSTART,
// поэтому переход на летнее время не сдвигает пары.
func (e Event) starts(limit time.Time) []time.Time {
	if e.Rule == nil {
		if e.excluded(e.Start) {
			return nil
		}
		return []time.Time{e.Start}
	}

	r := e.Rule
	loc := e.Start.Location()
	y, m, d := e.Start.Date()
	hh, mm, ss := e.Start.Clock()
	at := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, hh, mm, ss, 0, loc)
	}

	var res []time.Time
	count := 0
	// emit учитывает проведение. Возвращает false, когда развёртку пора прекратить.
	emit := func(t time.Time) bool {
		if !r.Until.IsZero() && t.After(r.Until) {
			return false
		}
		if !t.Before(limit) {
			return false
		}
		count++
		if !e.excluded(t) {
			res = append(res, t)
		}
		return r.Count == 0 || count < r.Count
	}

	switch r.Freq {
	case FreqDaily:
		for i := 0; i < maxOccurrences; i++ {
			if !emit(at(y, m, d+i*r.Interval)) {
				break
			}
		}
	case FreqWeekly:
		days := r.ByDay
		if len(days) == 0 {
			days = []time.Weekday{e.Start.Weekday()}
		}
		// Смещения дней от понедельника недели DTSTART, по порядку.
		offsets := make([]int, 0, len(days))
		for _, wd := range days {
			offsets = append(offsets, (int(wd)+6)%7)
		}
		slices.Sort(offsets)
		offsets = slices.Compact(offsets)
		mondayShift := -((int(e.Start.Weekday()) + 6) % 7)

		n := 0
	weeks:
		for week := 0; n < maxOccurrences; week += r.Interval {
			for _, off := range offsets {
				t := at(y, m, d+mondayShift+week*7+off)
				if t.Before(e.Start) {
					continue
				}
				n++
				if !emit(t) {
					break weeks
				}
			}
		}
	}
	return res
}

func (e Event) excluded(t time.Time) bool {
	for _, ex := range e.ExDates {
		if ex.Equal(t) {
			return true
		}
	}
	return false
}
