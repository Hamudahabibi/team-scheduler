package timegrid

import (
	"testing"
	"time"
)

var msk, _ = time.LoadLocation("Europe/Moscow")

func at(d, hh, mm int) time.Time { return time.Date(2026, 11, d, hh, mm, 0, 0, msk) }

func TestMaskOps(t *testing.T) {
	a := Range(10, 20)
	b := Range(15, 30)

	if got := a.And(b).Count(); got != 5 {
		t.Errorf("And: %d", got)
	}
	if got := a.Or(b).Count(); got != 20 {
		t.Errorf("Or: %d", got)
	}
	if got := a.AndNot(b); got != Range(10, 15) {
		t.Errorf("AndNot: %v", got)
	}
	if !a.Contains(Range(12, 18)) || a.Contains(Range(18, 22)) {
		t.Error("Contains")
	}
	if !(Mask{}).IsEmpty() || a.IsEmpty() {
		t.Error("IsEmpty")
	}
	if Full().Count() != SlotsPerWeek {
		t.Errorf("Full: %d", Full().Count())
	}
	// Операции не меняют исходную маску.
	if a.Count() != 10 {
		t.Error("And изменил операнд")
	}
	// Границы обрезаются, слоты вне недели игнорируются.
	if got := Range(-5, 3).Count(); got != 3 {
		t.Errorf("Range с отрицательным началом: %d", got)
	}
	var m Mask
	m.Set(SlotsPerWeek)
	m.Set(-1)
	if !m.IsEmpty() || m.Has(SlotsPerWeek) {
		t.Error("слоты вне недели")
	}
	// Граница слов uint64.
	m.SetRange(63, 65)
	if !m.Has(63) || !m.Has(64) || m.Has(65) || m.Count() != 2 {
		t.Error("граница слов")
	}
}

func TestWeek(t *testing.T) {
	if _, err := NewWeek(at(3, 0, 0), msk); err != ErrNotMonday {
		t.Errorf("вторник принят за начало недели: %v", err)
	}
	w, err := NewWeek(at(2, 15, 45), msk)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Start().Equal(at(2, 0, 0)) || !w.End().Equal(at(9, 0, 0)) {
		t.Errorf("границы: %v — %v", w.Start(), w.End())
	}
	if got := WeekOf(at(8, 23, 59), msk); !got.Start().Equal(w.Start()) {
		t.Errorf("WeekOf(воскресенье): %v", got.Start())
	}

	tests := []struct {
		name        string
		t           time.Time
		floor, ceil int
	}{
		{"начало недели", at(2, 0, 0), 0, 0},
		{"пн 08:30", at(2, 8, 30), 17, 17},
		{"пн 10:10", at(2, 10, 10), 20, 21},
		{"вт 00:00", at(3, 0, 0), 48, 48},
		{"вс 23:30", at(8, 23, 30), 335, 335},
		{"следующий пн", at(9, 0, 0), 336, 336},
		{"предыдущее вс 23:00", at(1, 23, 0), -2, -2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := w.SlotFloor(tt.t); got != tt.floor {
				t.Errorf("SlotFloor = %d, ожидалось %d", got, tt.floor)
			}
			if got := w.SlotCeil(tt.t); got != tt.ceil {
				t.Errorf("SlotCeil = %d, ожидалось %d", got, tt.ceil)
			}
		})
	}

	if got := w.SlotTime(17); !got.Equal(at(2, 8, 30)) {
		t.Errorf("SlotTime(17) = %v", got)
	}
	if got := w.SlotTime(335); !got.Equal(at(8, 23, 30)) {
		t.Errorf("SlotTime(335) = %v", got)
	}
}

func TestCoverAndInside(t *testing.T) {
	w, _ := NewWeek(at(2, 0, 0), msk)
	// Пара 08:30–10:10 занимает слоты 08:30, 09:00, 09:30, 10:00.
	if got := w.Cover(at(2, 8, 30), at(2, 10, 10)); got != Range(17, 21) {
		t.Errorf("Cover: %d слотов", got.Count())
	}
	// Свободен 15:10–18:00 — целых слотов с 15:30 по 17:30.
	if got := w.Inside(at(2, 15, 10), at(2, 18, 0)); got != Range(31, 36) {
		t.Errorf("Inside: %d слотов", got.Count())
	}
	// Интервал, выходящий за неделю, обрезается.
	if got := w.Cover(at(8, 23, 0), at(9, 2, 0)); got != Range(334, 336) {
		t.Errorf("Cover через границу недели: %d слотов", got.Count())
	}
	// Еженедельное правило: вторник 18:15–21:00 → слоты 18:30…20:30.
	if got := Weekly(2, 18*60+15, 21*60); got != Range(48+37, 48+42) {
		t.Errorf("Weekly: %d слотов", got.Count())
	}
}

func TestWeekAcrossDST(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	// Неделя 26.10.2026, в воскресенье 25.10 уже перевели часы; проверим неделю 19.10,
	// в которую переход попадает (25.10, воскресенье).
	w, err := NewWeek(time.Date(2026, 10, 19, 0, 0, 0, 0, berlin), berlin)
	if err != nil {
		t.Fatal(err)
	}
	sunday18 := time.Date(2026, 10, 25, 18, 0, 0, 0, berlin)
	if got := w.SlotFloor(sunday18); got != 6*SlotsPerDay+36 {
		t.Errorf("вс 18:00 после перевода часов: слот %d", got)
	}
	if got := w.SlotTime(6*SlotsPerDay + 36); !got.Equal(sunday18) {
		t.Errorf("SlotTime после перевода часов: %v", got)
	}
}
