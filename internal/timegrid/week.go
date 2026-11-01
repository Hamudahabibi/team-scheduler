package timegrid

import (
	"errors"
	"time"
)

// Week — неделя с понедельника 00:00 в заданном часовом поясе.
// Слоты считаются по «настенным часам», а не по смещению от начала
// недели в UTC, поэтому переход на летнее время не сдвигает сетку.
type Week struct {
	start time.Time
}

// ErrNotMonday — неделя должна начинаться с понедельника.
var ErrNotMonday = errors.New("неделя должна начинаться с понедельника")

// NewWeek создаёт неделю, начинающуюся в день date (должен быть понедельник).
// Время суток в date игнорируется.
func NewWeek(date time.Time, loc *time.Location) (Week, error) {
	y, m, d := date.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if start.Weekday() != time.Monday {
		return Week{}, ErrNotMonday
	}
	return Week{start: start}, nil
}

// WeekOf возвращает неделю, в которую попадает момент t.
func WeekOf(t time.Time, loc *time.Location) Week {
	t = t.In(loc)
	y, m, d := t.Date()
	shift := (int(t.Weekday()) + 6) % 7
	return Week{start: time.Date(y, m, d-shift, 0, 0, 0, 0, loc)}
}

// Start — понедельник 00:00.
func (w Week) Start() time.Time { return w.start }

// End — понедельник следующей недели 00:00.
func (w Week) End() time.Time {
	y, m, d := w.start.Date()
	return time.Date(y, m, d+Days, 0, 0, 0, 0, w.start.Location())
}

// Location — часовой пояс недели.
func (w Week) Location() *time.Location { return w.start.Location() }

// dayIndex — номер дня недели момента t относительно начала недели
// (может быть вне 0..6, если t вне недели).
func (w Week) dayIndex(t time.Time) int {
	y, m, d := t.In(w.Location()).Date()
	sy, sm, sd := w.start.Date()
	// Разница календарных дат через UTC-полночи не зависит от DST.
	a := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	b := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)
	return int(a.Sub(b).Hours() / 24)
}

// SlotFloor — слот, в который попадает момент t (округление вниз).
// Для моментов вне недели результат меньше 0 или не меньше SlotsPerWeek.
func (w Week) SlotFloor(t time.Time) int {
	lt := t.In(w.Location())
	minutes := lt.Hour()*60 + lt.Minute()
	return w.dayIndex(t)*SlotsPerDay + minutes/SlotMinutes
}

// SlotCeil — первый слот, начинающийся не раньше t (округление вверх).
func (w Week) SlotCeil(t time.Time) int {
	lt := t.In(w.Location())
	minutes := lt.Hour()*60 + lt.Minute()
	slot := w.dayIndex(t)*SlotsPerDay + minutes/SlotMinutes
	if minutes%SlotMinutes != 0 || lt.Second() != 0 || lt.Nanosecond() != 0 {
		slot++
	}
	return slot
}

// SlotTime — момент начала слота.
func (w Week) SlotTime(slot int) time.Time {
	y, m, d := w.start.Date()
	day, rem := slot/SlotsPerDay, slot%SlotsPerDay
	return time.Date(y, m, d+day, 0, rem*SlotMinutes, 0, 0, w.Location())
}

// Cover — слоты, хотя бы частично занятые интервалом [from, to).
// Подходит для занятости: если пара заканчивается в 10:10, слот 10:00–10:30 занят.
func (w Week) Cover(from, to time.Time) Mask {
	return Range(w.SlotFloor(from), w.SlotCeil(to))
}

// Inside — слоты, целиком лежащие внутри [from, to).
// Подходит для доступности: свободен с 15:10 — значит, слот 15:00 не свободен.
func (w Week) Inside(from, to time.Time) Mask {
	return Range(w.SlotCeil(from), w.SlotFloor(to))
}

// Weekly — слоты дня недели weekday (1 = понедельник) с startMin до endMin
// минут от полуночи, округлённые внутрь.
func Weekly(weekday, startMin, endMin int) Mask {
	base := (weekday - 1) * SlotsPerDay
	from := (startMin + SlotMinutes - 1) / SlotMinutes
	to := endMin / SlotMinutes
	return Range(base+from, base+to)
}

// DayOf — номер дня (0 = понедельник) слота.
func DayOf(slot int) int { return slot / SlotsPerDay }

// SlotOfDay — номер слота внутри дня.
func SlotOfDay(slot int) int { return slot % SlotsPerDay }
