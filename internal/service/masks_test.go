package service

import (
	"testing"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

var msk, _ = time.LoadLocation("Europe/Moscow")

// nov — момент в неделе 02.11.2026 (понедельник) по Москве.
func nov(day, hh, mm int) time.Time { return time.Date(2026, 11, day, hh, mm, 0, 0, msk) }

func testWeek(t *testing.T) timegrid.Week {
	t.Helper()
	w, err := timegrid.NewWeek(nov(2, 0, 0), msk)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestBuildPlayerMasks(t *testing.T) {
	w := testWeek(t)
	buffer := 10 * time.Minute
	// Пара в понедельник 14:10–15:50.
	classes := []domain.ClassEvent{{StartsAt: nov(2, 14, 10), EndsAt: nov(2, 15, 50)}}

	t.Run("без правил свободен всё время, кроме пар", func(t *testing.T) {
		m := buildPlayerMasks(w, nil, nil, classes, buffer)
		// С буфером занято 14:00–16:00: слоты 28..31.
		if m.Classes != timegrid.Range(28, 32) {
			t.Errorf("пары: %d слотов", m.Classes.Count())
		}
		if m.Free.Count() != timegrid.SlotsPerWeek-4 || m.Free.Has(28) || !m.Free.Has(32) {
			t.Errorf("свободно %d слотов", m.Free.Count())
		}
		if !m.Preferred.IsEmpty() {
			t.Error("предпочтения без правил")
		}
	})

	t.Run("правила, предпочтения и пары", func(t *testing.T) {
		rules := []domain.AvailabilityRule{
			{Weekday: 1, StartMin: 13 * 60, EndMin: 18 * 60, Kind: domain.RuleAvailable},
			{Weekday: 1, StartMin: 15 * 60, EndMin: 17 * 60, Kind: domain.RulePreferred},
		}
		m := buildPlayerMasks(w, rules, nil, classes, buffer)
		// 13:00–18:00 = слоты 26..35, минус пара 28..31 = 26,27,32..35.
		want := timegrid.Range(26, 28).Or(timegrid.Range(32, 36))
		if m.Free != want {
			t.Errorf("свободно: %d слотов, ожидалось %d", m.Free.Count(), want.Count())
		}
		// Предпочтение 15:00–17:00 (30..33) урезается парой до 32,33.
		if m.Preferred != timegrid.Range(32, 34) {
			t.Errorf("предпочтения: %d слотов", m.Preferred.Count())
		}
	})

	t.Run("исключения", func(t *testing.T) {
		rules := []domain.AvailabilityRule{{Weekday: 2, StartMin: 18 * 60, EndMin: 22 * 60, Kind: domain.RuleAvailable}}
		exc := []domain.AvailabilityException{
			// Во вторник не может после 20:00.
			{StartsAt: nov(3, 20, 0), EndsAt: nov(4, 0, 0), Kind: domain.ExceptionUnavailable},
			// В субботу может днём, хотя правил на субботу нет.
			{StartsAt: nov(7, 12, 0), EndsAt: nov(7, 16, 0), Kind: domain.ExceptionAvailable},
			// «Могу» и «не могу» пересекаются — побеждает «не могу».
			{StartsAt: nov(7, 15, 0), EndsAt: nov(7, 16, 0), Kind: domain.ExceptionUnavailable},
		}
		m := buildPlayerMasks(w, rules, exc, nil, buffer)
		tue := timegrid.SlotsPerDay
		sat := 5 * timegrid.SlotsPerDay
		want := timegrid.Range(tue+36, tue+40).Or(timegrid.Range(sat+24, sat+30))
		if m.Free != want {
			t.Errorf("свободно: %d слотов, ожидалось %d", m.Free.Count(), want.Count())
		}
	})
}

func TestMaskIntervals(t *testing.T) {
	w := testWeek(t)
	m := timegrid.Range(0, 2).Or(timegrid.Range(334, 336))
	got := maskIntervals(w, m)
	if len(got) != 2 {
		t.Fatalf("интервалов %d", len(got))
	}
	if !got[0].Start.Equal(nov(2, 0, 0)) || !got[0].End.Equal(nov(2, 1, 0)) {
		t.Errorf("первый интервал: %+v", got[0])
	}
	if !got[1].End.Equal(nov(9, 0, 0)) {
		t.Errorf("последний интервал заканчивается %v", got[1].End)
	}
}
