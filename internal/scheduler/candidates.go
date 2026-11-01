package scheduler

import (
	"cmp"
	"slices"

	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// candidate — допустимое время начала тренировки команды (без учёта
// других тренировок) и залы, где на это время хватает мест.
type candidate struct {
	team       int
	start      int
	day        int
	quality    float64
	attendance float64
	attendees  []int64
	rooms      []int
}

// teamCandidates — кандидаты команды и диагностика для объяснения недобора.
type teamCandidates struct {
	list []*candidate
	// compositions — сколько стартов, где собирается основной состав (без учёта залов).
	compositions int
	coreSize     int
}

// generateCandidates перебирает все старты в окне каждого дня
// (docs/research.md, раздел 4.1). Результат отсортирован по убыванию качества.
func generateCandidates(p Problem, ti int) teamCandidates {
	t := p.Teams[ti]
	w := p.Weights
	var res teamCandidates

	totalWeight := 0.0
	for _, m := range t.Members {
		if m.Core {
			res.coreSize++
			totalWeight++
		} else {
			totalWeight += w.SubstituteWeight
		}
	}
	if t.Duration <= 0 || totalWeight == 0 {
		return res
	}

	for day := range timegrid.Days {
		for sod := t.DayStart; sod+t.Duration <= t.DayEnd && sod+t.Duration <= timegrid.SlotsPerDay; sod++ {
			start := day*timegrid.SlotsPerDay + sod
			interval := timegrid.Range(start, start+t.Duration)

			core, weight, prefSum, prefCount := 0, 0.0, 0.0, 0
			var attendees []int64
			for _, m := range t.Members {
				if !m.Free.Contains(interval) {
					continue
				}
				attendees = append(attendees, m.UserID)
				if m.Core {
					core++
					weight++
				} else {
					weight += w.SubstituteWeight
				}
				if !m.Preferred.IsEmpty() {
					prefSum += float64(m.Preferred.And(interval).Count()) / float64(t.Duration)
					prefCount++
				}
			}
			if core < t.MinCore {
				continue
			}
			res.compositions++

			var rooms []int
			for ri := range p.Rooms {
				if roomHasSeats(&p.Rooms[ri], start, t.Duration, t.Seats) {
					rooms = append(rooms, ri)
				}
			}
			if len(rooms) == 0 {
				continue
			}

			att := weight / totalWeight
			pref := 0.0
			if prefCount > 0 {
				pref = prefSum / float64(prefCount)
			}
			late := 0
			for s := sod; s < sod+t.Duration; s++ {
				if s >= w.LateSlot {
					late++
				}
			}
			res.list = append(res.list, &candidate{
				team:       ti,
				start:      start,
				day:        day,
				quality:    w.Attendance*att + w.Preference*pref - w.Late*float64(late)/float64(t.Duration),
				attendance: att,
				attendees:  attendees,
				rooms:      rooms,
			})
		}
	}
	sortCandidates(res.list)
	return res
}

func roomHasSeats(r *Room, start, duration, seats int) bool {
	for s := start; s < start+duration; s++ {
		if r.Free[s] < seats {
			return false
		}
	}
	return true
}

// sortCandidates задаёт полный порядок: качество по убыванию, затем время.
// От него зависит детерминированность обоих алгоритмов.
func sortCandidates(list []*candidate) {
	slices.SortStableFunc(list, func(a, b *candidate) int {
		if c := cmp.Compare(b.quality, a.quality); c != 0 {
			return c
		}
		return cmp.Compare(a.start, b.start)
	})
}

// topPerDay оставляет не больше k лучших кандидатов на каждый день.
// Ограничение по дням, а не общее, сохраняет разнообразие: иначе все
// лучшие кандидаты могут оказаться в одном-двух днях, а ставить
// несколько тренировок в день нельзя.
func topPerDay(list []*candidate, k int) []*candidate {
	var perDay [timegrid.Days]int
	res := make([]*candidate, 0, min(len(list), k*timegrid.Days))
	for _, c := range list {
		if perDay[c.day] < k {
			perDay[c.day]++
			res = append(res, c)
		}
	}
	return res
}
