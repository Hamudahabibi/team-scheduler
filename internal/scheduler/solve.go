package scheduler

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// Solve составляет расписание. Ошибка возвращается только для
// некорректной задачи; невозможность поставить тренировки — это
// не ошибка, а Shortfalls в статистике.
func Solve(ctx context.Context, p Problem, opt Options) (Result, error) {
	if err := validate(p); err != nil {
		return Result{}, err
	}
	if opt.Algorithm == "" {
		opt.Algorithm = Greedy
	}
	if opt.Algorithm != Greedy && opt.Algorithm != BranchAndBound {
		return Result{}, fmt.Errorf("неизвестный алгоритм %q", opt.Algorithm)
	}
	started := time.Now()

	cands := make([]teamCandidates, len(p.Teams))
	total := 0
	for ti := range p.Teams {
		cands[ti] = generateCandidates(p, ti)
		total += len(cands[ti].list)
	}
	order := teamOrder(&p, cands)

	// Жадное решение нужно в обоих случаях: для B&B это начальный рекорд.
	greedy := solveGreedy(&p, cands, order)
	chosen, value := greedy.snapshot(), greedy.value
	var nodes int64
	optimal := false

	if opt.Algorithm == BranchAndBound {
		b := solveBnB(ctx, &p, cands, order, greedy, opt)
		chosen, value = b.best, b.bestValue
		nodes, optimal = b.nodes, !b.aborted
	}

	res := buildResult(&p, cands, chosen)
	res.Stats.Algorithm = opt.Algorithm
	res.Stats.Candidates = total
	res.Stats.Nodes = nodes
	res.Stats.Optimal = optimal
	res.Stats.Objective = value
	res.Stats.Elapsed = time.Since(started)
	return res, nil
}

func validate(p Problem) error {
	if p.Weights.Unit <= 0 {
		return fmt.Errorf("веса целевой функции не заданы")
	}
	seen := map[int64]bool{}
	for _, t := range p.Teams {
		switch {
		case seen[t.ID]:
			return fmt.Errorf("команда %d указана дважды", t.ID)
		case t.Sessions < 1 || t.Sessions > timegrid.Days:
			return fmt.Errorf("команда %d: тренировок в неделю от 1 до 7", t.ID)
		case t.Duration < 1:
			return fmt.Errorf("команда %d: длительность не меньше слота", t.ID)
		case t.DayStart < 0 || t.DayEnd > timegrid.SlotsPerDay || t.DayStart >= t.DayEnd:
			return fmt.Errorf("команда %d: некорректное окно дня", t.ID)
		case t.Priority < 1:
			return fmt.Errorf("команда %d: приоритет не меньше 1", t.ID)
		case t.Seats < 1:
			return fmt.Errorf("команда %d: число мест не меньше 1", t.ID)
		case t.MinCore < 1:
			return fmt.Errorf("команда %d: минимум основного состава не меньше 1", t.ID)
		}
		seen[t.ID] = true
	}
	return nil
}

func buildResult(p *Problem, cands []teamCandidates, chosen []placement) Result {
	var res Result
	attSum := 0.0
	for _, pl := range chosen {
		t := p.Teams[pl.c.team]
		res.Sessions = append(res.Sessions, Session{
			TeamID:     t.ID,
			RoomID:     p.Rooms[pl.room].ID,
			Start:      pl.c.start,
			Duration:   t.Duration,
			Seats:      t.Seats,
			Quality:    pl.c.quality,
			Attendance: pl.c.attendance,
			Attendees:  pl.c.attendees,
		})
		attSum += pl.c.attendance
	}
	slices.SortFunc(res.Sessions, func(a, b Session) int {
		if c := cmp.Compare(a.Start, b.Start); c != 0 {
			return c
		}
		return cmp.Compare(a.TeamID, b.TeamID)
	})

	scheduled := make([]int, len(p.Teams))
	for _, pl := range chosen {
		scheduled[pl.c.team]++
	}
	res.Stats.Scheduled = len(chosen)
	res.Stats.Shortfalls = []Shortfall{}
	for ti, t := range p.Teams {
		res.Stats.Requested += t.Sessions
		if scheduled[ti] < t.Sessions {
			res.Stats.Shortfalls = append(res.Stats.Shortfalls, Shortfall{
				TeamID:    t.ID,
				Requested: t.Sessions,
				Scheduled: scheduled[ti],
				Reason:    shortfallReason(t, cands[ti]),
			})
		}
	}
	if len(chosen) > 0 {
		res.Stats.AvgAttendance = attSum / float64(len(chosen))
	}
	return res
}

// shortfallReason объясняет тренеру, почему тренировок меньше, чем нужно.
func shortfallReason(t Team, c teamCandidates) string {
	switch {
	case c.coreSize < t.MinCore:
		return fmt.Sprintf("в основном составе %d игроков, а план требует минимум %d", c.coreSize, t.MinCore)
	case c.compositions == 0:
		return "нет ни одного интервала в окне плана, когда свободен нужный минимум основного состава"
	case len(c.list) == 0:
		return "основной состав собирается, но в это время в залах нет свободных мест"
	default:
		return fmt.Sprintf("найдено %d подходящих интервалов, но совместимых между собой (раз в день, отдых, места в залах) не хватило", len(c.list))
	}
}
