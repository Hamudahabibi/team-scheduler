// Package synth генерирует синтетические задачи планирования для тестов,
// бенчмарков и вычислительного эксперимента (docs/research.md, раздел 5).
// Генерация детерминирована: одинаковый Config даёт одинаковую задачу.
package synth

import (
	"math/rand/v2"

	"github.com/Hamudahabibi/team-scheduler/internal/scheduler"
	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// Config — параметры генерации.
type Config struct {
	Seed      uint64
	Teams     int
	Groups    int
	Rooms     int
	RoomSeats int
	// SessionsMin, SessionsMax — диапазон числа тренировок в неделю.
	SessionsMin, SessionsMax int
	// Substitutes — максимум запасных в команде.
	Substitutes int
	// PersonalRules — доля игроков, указавших личную доступность (только вечера).
	PersonalRules float64
	// RoomLoad — доля часов работы залов, занятых внешними мероприятиями (0..1).
	RoomLoad float64
}

// Default — типичная конфигурация университетского киберцентра.
func Default(seed uint64, teams int) Config {
	return Config{
		Seed: seed, Teams: teams, Groups: 12, Rooms: 2, RoomSeats: 10,
		SessionsMin: 2, SessionsMax: 4, Substitutes: 2,
		PersonalRules: 0.4, RoomLoad: 0.1,
	}
}

// Условная сетка пар: начало и конец в минутах от полуночи.
var pairs = [][2]int{
	{8*60 + 30, 10*60 + 10},
	{10*60 + 20, 12 * 60},
	{12*60 + 20, 14 * 60},
	{14*60 + 10, 15*60 + 50},
	{16 * 60, 17*60 + 40},
	{18 * 60, 19*60 + 40},
}

const bufferMin = 10

// Generate строит задачу.
func Generate(cfg Config) scheduler.Problem {
	rng := rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15))

	groups := make([]timegrid.Mask, cfg.Groups)
	for g := range groups {
		groups[g] = groupClasses(rng)
	}

	p := scheduler.Problem{Weights: scheduler.DefaultWeights()}
	var userID int64
	for ti := range cfg.Teams {
		t := scheduler.Team{
			ID:       int64(ti + 1),
			Sessions: cfg.SessionsMin + rng.IntN(cfg.SessionsMax-cfg.SessionsMin+1),
			Duration: 4 + rng.IntN(2), // 2–2,5 часа
			MinCore:  5,
			DayStart: 12 * 2,
			DayEnd:   23 * 2,
			Priority: 1,
			Seats:    5,
		}
		if rng.Float64() < 0.3 {
			t.MinCore = 4
		}
		if rng.Float64() < 0.5 {
			t.MinRest = 12 * 2
		}
		if rng.Float64() < 0.2 {
			t.Priority = 2
		}
		members := 5 + rng.IntN(cfg.Substitutes+1)
		for mi := range members {
			userID++
			m := scheduler.Member{UserID: userID, Core: mi < 5}
			free := timegrid.Full()
			if rng.Float64() < cfg.PersonalRules {
				free, m.Preferred = personalRules(rng)
			}
			// В команде игроки из разных групп и курсов.
			m.Free = free.AndNot(groups[rng.IntN(len(groups))])
			m.Preferred = m.Preferred.And(m.Free)
			t.Members = append(t.Members, m)
		}
		p.Teams = append(p.Teams, t)
	}

	for ri := range cfg.Rooms {
		r := scheduler.Room{ID: int64(ri + 1)}
		for day := range timegrid.Days {
			open, closeSlot := 10*2, 22*2
			if day == 6 {
				open, closeSlot = 12*2, 20*2
			}
			for s := open; s < closeSlot; s++ {
				r.Free[day*timegrid.SlotsPerDay+s] = cfg.RoomSeats
			}
		}
		// Внешние мероприятия блоками по 2–4 часа.
		blocks := int(cfg.RoomLoad * 7 * 12 / 3)
		for range blocks {
			day := rng.IntN(timegrid.Days)
			start := day*timegrid.SlotsPerDay + 20 + rng.IntN(20)
			for s := start; s < start+4+rng.IntN(5) && s < (day+1)*timegrid.SlotsPerDay; s++ {
				r.Free[s] = 0
			}
		}
		p.Rooms = append(p.Rooms, r)
	}
	return p
}

// groupClasses — занятость группы: 5–6 учебных дней, 2–4 пары подряд.
func groupClasses(rng *rand.Rand) timegrid.Mask {
	var busy timegrid.Mask
	days := 5 + rng.IntN(2)
	for day := range days {
		first := rng.IntN(3)
		count := 2 + rng.IntN(3)
		for p := first; p < first+count && p < len(pairs); p++ {
			from := (pairs[p][0] - bufferMin) / timegrid.SlotMinutes
			to := (pairs[p][1] + bufferMin + timegrid.SlotMinutes - 1) / timegrid.SlotMinutes
			busy = busy.Or(timegrid.Range(day*timegrid.SlotsPerDay+from, day*timegrid.SlotsPerDay+to))
		}
	}
	return busy
}

// personalRules — игрок свободен только вечерами в часть дней недели,
// в часть из них предпочитает время.
func personalRules(rng *rand.Rand) (free, preferred timegrid.Mask) {
	for day := range timegrid.Days {
		if rng.Float64() < 0.65 {
			from := 16*2 + rng.IntN(4)
			free = free.Or(timegrid.Range(day*timegrid.SlotsPerDay+from, day*timegrid.SlotsPerDay+23*2))
			if rng.Float64() < 0.5 {
				preferred = preferred.Or(timegrid.Range(day*timegrid.SlotsPerDay+18*2, day*timegrid.SlotsPerDay+21*2))
			}
		}
	}
	return free, preferred
}
