package scheduler

import (
	"context"
	"slices"
	"time"
)

// bnb — метод ветвей и границ (docs/research.md, раздел 4.3).
type bnb struct {
	s     *state
	order []int
	lists [][]*candidate
	// tail[k] — оптимистичная оценка для команд order[k:], если им ещё ничего не поставлено.
	tail []float64
	// roomClass[r] — первый зал с тем же профилем свободных мест, что у r.
	// Залы одного класса с одинаковой текущей загрузкой взаимозаменяемы.
	roomClass []int

	best      []placement
	bestValue float64

	ctx      context.Context
	deadline time.Time
	maxNodes int64
	nodes    int64
	aborted  bool
}

// eps защищает от ложных улучшений из-за погрешности сложения float64.
const eps = 1e-9

func solveBnB(ctx context.Context, p *Problem, cands []teamCandidates, order []int, incumbent *state, opt Options) *bnb {
	k := opt.TopPerDay
	if k <= 0 {
		k = DefaultTopPerDay
	}
	b := &bnb{
		s:         newState(p),
		order:     order,
		lists:     make([][]*candidate, len(p.Teams)),
		tail:      make([]float64, len(order)+1),
		roomClass: make([]int, len(p.Rooms)),
		best:      incumbent.snapshot(),
		bestValue: incumbent.value,
		ctx:       ctx,
		maxNodes:  opt.MaxNodes,
	}
	if opt.TimeLimit > 0 {
		b.deadline = time.Now().Add(opt.TimeLimit)
	}
	for ti := range p.Teams {
		b.lists[ti] = topPerDay(cands[ti].list, k)
	}
	for i := len(order) - 1; i >= 0; i-- {
		ti := order[i]
		b.tail[i] = b.tail[i+1] + b.optimistic(ti, 0, p.Teams[ti].Sessions, 0)
	}
	for r := range p.Rooms {
		b.roomClass[r] = r
		for q := range r {
			if p.Rooms[q].Free == p.Rooms[r].Free {
				b.roomClass[r] = q
				break
			}
		}
	}

	b.search(0, 0)
	return b
}

// optimistic — верхняя оценка вклада команды: не больше need кандидатов
// с индексами от from, по одному на день, в дни, ещё не занятые
// тренировками команды. Список отсортирован по убыванию качества, поэтому
// первое вхождение дня — лучший кандидат этого дня, и достаточно одного
// прохода. Штрафы за соседние дни и ограничения отдыха и вместимости
// отбрасываются — оценка остаётся оптимистичной.
func (b *bnb) optimistic(team, from, need int, usedDays uint8) float64 {
	if need <= 0 {
		return 0
	}
	t := &b.s.p.Teams[team]
	unit := b.s.p.Weights.Unit
	sum := 0.0
	for _, c := range b.lists[team][from:] {
		if usedDays&(1<<c.day) != 0 {
			continue
		}
		usedDays |= 1 << c.day
		sum += unit + float64(t.Priority)*c.quality
		if need--; need == 0 {
			break
		}
	}
	return sum
}

// stop проверяет лимиты. Время и контекст проверяются раз в 1024 узла:
// time.Now дороже, чем шаг перебора.
func (b *bnb) stop() bool {
	if b.aborted {
		return true
	}
	if b.maxNodes > 0 && b.nodes >= b.maxNodes {
		b.aborted = true
	} else if b.nodes&1023 == 0 {
		if b.ctx.Err() != nil || (!b.deadline.IsZero() && time.Now().After(b.deadline)) {
			b.aborted = true
		}
	}
	return b.aborted
}

// search перебирает решения для команды order[k], выбирая кандидатов
// с индексами не меньше from (сочетания без повторов).
func (b *bnb) search(k, from int) {
	b.nodes++
	if b.stop() {
		return
	}

	bound := b.s.value + b.tail[min(k+1, len(b.order))]
	if k < len(b.order) {
		ti := b.order[k]
		need := b.s.p.Teams[ti].Sessions - b.s.placed[ti]
		bound += b.optimistic(ti, from, need, b.s.days[ti])
	}
	if bound <= b.bestValue+eps {
		return
	}
	if k == len(b.order) {
		b.best = b.s.snapshot()
		b.bestValue = b.s.value
		return
	}

	ti := b.order[k]
	if b.s.placed[ti] < b.s.p.Teams[ti].Sessions {
		list := b.lists[ti]
		tried := make([]int, 0, len(b.s.p.Rooms))
		for i := from; i < len(list); i++ {
			c := list[i]
			if !b.s.teamAllows(c) {
				continue
			}
			tried = tried[:0]
			for _, room := range c.rooms {
				if b.s.slack(c, room) < 0 || b.symmetric(room, tried) {
					continue
				}
				tried = append(tried, room)
				b.s.place(c, room)
				b.search(k, i+1)
				b.s.undo()
				if b.aborted {
					return
				}
			}
		}
	}
	// Ветка «команде больше ничего не ставим».
	b.search(k+1, 0)
}

// symmetric сообщает, что зал room неотличим от уже опробованного:
// одинаковые часы и места и одинаковая текущая загрузка. Ветка с ним
// дала бы то же значение целевой функции.
func (b *bnb) symmetric(room int, tried []int) bool {
	for _, r := range tried {
		if b.roomClass[r] == b.roomClass[room] && slices.Equal(b.s.used[r], b.s.used[room]) {
			return true
		}
	}
	return false
}
