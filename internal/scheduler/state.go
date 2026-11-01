package scheduler

// placement — выбранная тренировка: кандидат и зал.
type placement struct {
	c    *candidate
	room int
	gain float64
}

// state — частичное расписание. Поддерживает добавление и откат
// за O(длительность тренировки), что нужно перебору с возвратом.
type state struct {
	p *Problem
	// used[room][slot] — мест, занятых выбранными тренировками.
	used [][]int
	// days[team] — битовая маска дней, где у команды уже есть тренировка.
	days []uint8
	// starts[team] — слоты начала выбранных тренировок команды.
	starts [][]int
	placed []int
	stack  []placement
	value  float64
}

func newState(p *Problem) *state {
	s := &state{
		p:      p,
		used:   make([][]int, len(p.Rooms)),
		days:   make([]uint8, len(p.Teams)),
		starts: make([][]int, len(p.Teams)),
		placed: make([]int, len(p.Teams)),
	}
	for i := range s.used {
		s.used[i] = make([]int, len(p.Rooms[i].Free))
	}
	return s
}

// teamAllows проверяет ограничения команды: одна тренировка в день,
// перерыв не меньше MinRest и не больше Sessions тренировок.
func (s *state) teamAllows(c *candidate) bool {
	t := &s.p.Teams[c.team]
	if s.placed[c.team] >= t.Sessions || s.days[c.team]&(1<<c.day) != 0 {
		return false
	}
	gap := t.Duration + t.MinRest
	for _, other := range s.starts[c.team] {
		if c.start < other+gap && other < c.start+gap {
			return false
		}
	}
	return true
}

// slack возвращает, сколько мест останется в зале в самом загруженном
// слоте тренировки, или -1, если мест не хватает.
func (s *state) slack(c *candidate, room int) int {
	t := &s.p.Teams[c.team]
	free := s.p.Rooms[room].Free
	used := s.used[room]
	best := -1
	for slot := c.start; slot < c.start+t.Duration; slot++ {
		left := free[slot] - used[slot] - t.Seats
		if left < 0 {
			return -1
		}
		if best < 0 || left < best {
			best = left
		}
	}
	return best
}

// bestFitRoom выбирает зал, где после размещения останется меньше всего
// свободных мест (best fit): крупные залы остаются для крупных составов.
func (s *state) bestFitRoom(c *candidate) int {
	room, bestSlack := -1, -1
	for _, r := range c.rooms {
		sl := s.slack(c, r)
		if sl >= 0 && (room < 0 || sl < bestSlack) {
			room, bestSlack = r, sl
		}
	}
	return room
}

// gain — прирост целевой функции при добавлении кандидата.
func (s *state) gain(c *candidate) float64 {
	t := &s.p.Teams[c.team]
	w := &s.p.Weights
	g := w.Unit + float64(t.Priority)*c.quality
	days := s.days[c.team]
	if c.day > 0 && days&(1<<(c.day-1)) != 0 {
		g -= w.Adjacent
	}
	if c.day < 6 && days&(1<<(c.day+1)) != 0 {
		g -= w.Adjacent
	}
	return g
}

func (s *state) place(c *candidate, room int) {
	t := &s.p.Teams[c.team]
	g := s.gain(c)
	for slot := c.start; slot < c.start+t.Duration; slot++ {
		s.used[room][slot] += t.Seats
	}
	s.days[c.team] |= 1 << c.day
	s.starts[c.team] = append(s.starts[c.team], c.start)
	s.placed[c.team]++
	s.stack = append(s.stack, placement{c: c, room: room, gain: g})
	s.value += g
}

func (s *state) undo() {
	last := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	c := last.c
	t := &s.p.Teams[c.team]
	for slot := c.start; slot < c.start+t.Duration; slot++ {
		s.used[last.room][slot] -= t.Seats
	}
	s.days[c.team] &^= 1 << c.day
	s.starts[c.team] = s.starts[c.team][:len(s.starts[c.team])-1]
	s.placed[c.team]--
	s.value -= last.gain
}

// snapshot копирует текущий набор тренировок.
func (s *state) snapshot() []placement {
	return append([]placement(nil), s.stack...)
}
