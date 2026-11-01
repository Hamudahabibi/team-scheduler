package scheduler

import (
	"cmp"
	"slices"
)

// teamOrder — порядок команд для обоих алгоритмов: сначала высокий
// приоритет, затем «самый ограниченный — первым» (меньше кандидатов).
func teamOrder(p *Problem, cands []teamCandidates) []int {
	order := make([]int, len(p.Teams))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		ta, tb := p.Teams[a], p.Teams[b]
		if c := cmp.Compare(tb.Priority, ta.Priority); c != 0 {
			return c
		}
		if c := cmp.Compare(len(cands[a].list), len(cands[b].list)); c != 0 {
			return c
		}
		return cmp.Compare(ta.ID, tb.ID)
	})
	return order
}

// solveGreedy — жадный алгоритм по раундам (docs/research.md, раздел 4.2).
// В каждом раунде каждая команда, которой ещё нужны тренировки, берёт
// лучшего совместимого кандидата. Раунды не дают первой команде занять
// всё лучшее время недели.
func solveGreedy(p *Problem, cands []teamCandidates, order []int) *state {
	s := newState(p)
	for progress := true; progress; {
		progress = false
		for _, ti := range order {
			if s.placed[ti] >= p.Teams[ti].Sessions {
				continue
			}
			if c, room := bestCandidate(s, cands[ti].list); c != nil {
				s.place(c, room)
				progress = true
			}
		}
	}
	return s
}

// bestCandidate ищет кандидата с наибольшим приростом целевой функции.
// Кандидаты отсортированы по качеству, а прирост не больше Unit + Priority·quality,
// поэтому перебор останавливается, как только лучше уже не найти.
func bestCandidate(s *state, list []*candidate) (*candidate, int) {
	var (
		best     *candidate
		bestRoom = -1
		bestGain float64
	)
	for _, c := range list {
		t := &s.p.Teams[c.team]
		upper := s.p.Weights.Unit + float64(t.Priority)*c.quality
		if best != nil && upper <= bestGain {
			break
		}
		if !s.teamAllows(c) {
			continue
		}
		room := s.bestFitRoom(c)
		if room < 0 {
			continue
		}
		if g := s.gain(c); best == nil || g > bestGain {
			best, bestRoom, bestGain = c, room, g
		}
	}
	return best, bestRoom
}
