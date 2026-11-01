package scheduler_test

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/scheduler"
	"github.com/Hamudahabibi/team-scheduler/internal/scheduler/synth"
	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

const spd = timegrid.SlotsPerDay

// hour — слот дня по часу и минутам.
func hour(h, m int) int { return h*2 + m/30 }

// daily — маска одинакового интервала [from, to) в указанные дни (0 = пн).
func daily(from, to int, days ...int) timegrid.Mask {
	var m timegrid.Mask
	for _, d := range days {
		m = m.Or(timegrid.Range(d*spd+from, d*spd+to))
	}
	return m
}

func weekdays() []int { return []int{0, 1, 2, 3, 4} }

// team5 — команда из пяти игроков основного состава с одинаковой доступностью.
func team5(id int64, free timegrid.Mask) scheduler.Team {
	t := scheduler.Team{
		ID: id, Sessions: 3, Duration: 4, MinCore: 5,
		DayStart: hour(12, 0), DayEnd: hour(23, 0), Priority: 1, Seats: 5,
	}
	for i := range 5 {
		t.Members = append(t.Members, scheduler.Member{UserID: id*100 + int64(i), Core: true, Free: free})
	}
	return t
}

// openRoom — зал с местами seats с 10:00 до 22:00 каждый день.
func openRoom(id int64, seats int) scheduler.Room {
	r := scheduler.Room{ID: id}
	for d := range timegrid.Days {
		for s := hour(10, 0); s < hour(22, 0); s++ {
			r.Free[d*spd+s] = seats
		}
	}
	return r
}

func solve(t *testing.T, p scheduler.Problem, alg scheduler.Algorithm) scheduler.Result {
	t.Helper()
	res, err := scheduler.Solve(context.Background(), p, scheduler.Options{Algorithm: alg, TimeLimit: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	verify(t, p, res)
	return res
}

// verify независимо от решателя проверяет все жёсткие ограничения
// (docs/research.md, разделы 3.3 и 3.5).
func verify(t *testing.T, p scheduler.Problem, res scheduler.Result) {
	t.Helper()
	teams := map[int64]scheduler.Team{}
	for _, tm := range p.Teams {
		teams[tm.ID] = tm
	}
	rooms := map[int64]scheduler.Room{}
	for _, r := range p.Rooms {
		rooms[r.ID] = r
	}
	used := map[int64][]int{}
	byTeam := map[int64][]scheduler.Session{}

	for _, s := range res.Sessions {
		tm := teams[s.TeamID]
		day, sod := s.Start/spd, s.Start%spd
		if sod < tm.DayStart || sod+tm.Duration > tm.DayEnd {
			t.Errorf("команда %d: тренировка %d вне окна дня", s.TeamID, s.Start)
		}
		if (s.Start+tm.Duration-1)/spd != day {
			t.Errorf("команда %d: тренировка %d переходит через полночь", s.TeamID, s.Start)
		}
		interval := timegrid.Range(s.Start, s.Start+tm.Duration)
		core := 0
		for _, m := range tm.Members {
			if m.Core && m.Free.Contains(interval) {
				core++
			}
		}
		if core < tm.MinCore {
			t.Errorf("команда %d: в %d свободно %d игроков основного состава из %d нужных", s.TeamID, s.Start, core, tm.MinCore)
		}
		if used[s.RoomID] == nil {
			used[s.RoomID] = make([]int, timegrid.SlotsPerWeek)
		}
		for slot := s.Start; slot < s.Start+tm.Duration; slot++ {
			used[s.RoomID][slot] += tm.Seats
			if used[s.RoomID][slot] > rooms[s.RoomID].Free[slot] {
				t.Errorf("зал %d: в слоте %d занято %d мест из %d", s.RoomID, slot, used[s.RoomID][slot], rooms[s.RoomID].Free[slot])
			}
		}
		byTeam[s.TeamID] = append(byTeam[s.TeamID], s)
	}

	for id, ss := range byTeam {
		tm := teams[id]
		if len(ss) > tm.Sessions {
			t.Errorf("команда %d: %d тренировок при плане %d", id, len(ss), tm.Sessions)
		}
		for i := range ss {
			for j := i + 1; j < len(ss); j++ {
				a, b := ss[i].Start, ss[j].Start
				if a/spd == b/spd {
					t.Errorf("команда %d: две тренировки в день %d", id, a/spd)
				}
				gap := tm.Duration + tm.MinRest
				if a < b+gap && b < a+gap {
					t.Errorf("команда %d: перерыв между %d и %d меньше %d слотов", id, a, b, tm.MinRest)
				}
			}
		}
	}
	if res.Stats.Scheduled != len(res.Sessions) {
		t.Errorf("Stats.Scheduled = %d, тренировок %d", res.Stats.Scheduled, len(res.Sessions))
	}
}

func days(res scheduler.Result, teamID int64) []int {
	var d []int
	for _, s := range res.Sessions {
		if s.TeamID == teamID {
			d = append(d, s.Start/spd)
		}
	}
	slices.Sort(d)
	return d
}

func TestSpreadsSessionsOverWeek(t *testing.T) {
	p := scheduler.Problem{
		Teams:   []scheduler.Team{team5(1, daily(hour(18, 0), hour(22, 0), weekdays()...))},
		Rooms:   []scheduler.Room{openRoom(1, 10)},
		Weights: scheduler.DefaultWeights(),
	}
	for _, alg := range []scheduler.Algorithm{scheduler.Greedy, scheduler.BranchAndBound} {
		t.Run(string(alg), func(t *testing.T) {
			res := solve(t, p, alg)
			// Три тренировки на пять будних дней без соседних дней: только пн, ср, пт.
			if got := days(res, 1); !slices.Equal(got, []int{0, 2, 4}) {
				t.Errorf("дни %v, ожидались [0 2 4]", got)
			}
			if len(res.Stats.Shortfalls) != 0 {
				t.Errorf("недобор: %+v", res.Stats.Shortfalls)
			}
		})
	}
}

func TestAvoidsClassesOfCorePlayer(t *testing.T) {
	tm := team5(1, daily(hour(14, 0), hour(22, 0), weekdays()...))
	tm.Sessions = 1
	// У одного игрока основного состава пары с 14:00 до 20:00 каждый будний день.
	tm.Members[0].Free = tm.Members[0].Free.AndNot(daily(hour(14, 0), hour(20, 0), weekdays()...))
	p := scheduler.Problem{Teams: []scheduler.Team{tm}, Rooms: []scheduler.Room{openRoom(1, 10)}, Weights: scheduler.DefaultWeights()}

	res := solve(t, p, scheduler.Greedy)
	if len(res.Sessions) != 1 {
		t.Fatalf("тренировок %d", len(res.Sessions))
	}
	if sod := res.Sessions[0].Start % spd; sod < hour(20, 0) {
		t.Errorf("тренировка в %d:%02d — у игрока пары", sod/2, sod%2*30)
	}
}

func TestPrefersFullAttendance(t *testing.T) {
	tm := team5(1, daily(hour(16, 0), hour(22, 0), 0))
	tm.Sessions, tm.MinCore = 1, 4
	// Запасной свободен только с 18:00. Лучше начать в 18:00, когда придут все.
	tm.Members = append(tm.Members, scheduler.Member{UserID: 999, Free: daily(hour(18, 0), hour(22, 0), 0)})
	p := scheduler.Problem{Teams: []scheduler.Team{tm}, Rooms: []scheduler.Room{openRoom(1, 10)}, Weights: scheduler.DefaultWeights()}

	res := solve(t, p, scheduler.Greedy)
	s := res.Sessions[0]
	if s.Start%spd < hour(18, 0) || len(s.Attendees) != 6 || s.Attendance != 1 {
		t.Errorf("старт %d, придут %d, посещаемость %.2f", s.Start%spd, len(s.Attendees), s.Attendance)
	}
}

func TestRoomCapacityAndPriority(t *testing.T) {
	// Обе команды свободны только в понедельник 18:00–20:00, в зале 5 мест.
	free := daily(hour(18, 0), hour(20, 0), 0)
	a, b := team5(1, free), team5(2, free)
	a.Sessions, b.Sessions = 1, 1
	b.Priority = 2
	p := scheduler.Problem{Teams: []scheduler.Team{a, b}, Rooms: []scheduler.Room{openRoom(1, 5)}, Weights: scheduler.DefaultWeights()}

	for _, alg := range []scheduler.Algorithm{scheduler.Greedy, scheduler.BranchAndBound} {
		t.Run(string(alg), func(t *testing.T) {
			res := solve(t, p, alg)
			if len(res.Sessions) != 1 || res.Sessions[0].TeamID != 2 {
				t.Fatalf("ожидалась одна тренировка команды с приоритетом 2: %+v", res.Sessions)
			}
			if len(res.Stats.Shortfalls) != 1 || res.Stats.Shortfalls[0].TeamID != 1 {
				t.Errorf("недобор: %+v", res.Stats.Shortfalls)
			}
		})
	}

	// В зале на 10 мест помещаются обе.
	p.Rooms = []scheduler.Room{openRoom(1, 10)}
	if res := solve(t, p, scheduler.Greedy); len(res.Sessions) != 2 {
		t.Errorf("в зале на 10 мест поставлено %d тренировок", len(res.Sessions))
	}
}

func TestBestFitRoom(t *testing.T) {
	tm := team5(1, daily(hour(18, 0), hour(20, 0), 0))
	tm.Sessions = 1
	p := scheduler.Problem{
		Teams:   []scheduler.Team{tm},
		Rooms:   []scheduler.Room{openRoom(1, 20), openRoom(2, 6)},
		Weights: scheduler.DefaultWeights(),
	}
	res := solve(t, p, scheduler.Greedy)
	if res.Sessions[0].RoomID != 2 {
		t.Errorf("выбран зал %d, ожидался малый зал 2", res.Sessions[0].RoomID)
	}
}

func TestMinRest(t *testing.T) {
	// Свободны пн 20:00–22:00 и вт 12:00–14:00: между ними 14 часов.
	tm := team5(1, daily(hour(20, 0), hour(22, 0), 0).Or(daily(hour(12, 0), hour(14, 0), 1)))
	tm.Sessions = 2
	tm.MinRest = 16 * 2
	p := scheduler.Problem{Teams: []scheduler.Team{tm}, Rooms: []scheduler.Room{openRoom(1, 10)}, Weights: scheduler.DefaultWeights()}
	if res := solve(t, p, scheduler.BranchAndBound); len(res.Sessions) != 1 {
		t.Errorf("при отдыхе 16 часов поставлено %d тренировок", len(res.Sessions))
	}
	p.Teams[0].MinRest = 12 * 2
	if res := solve(t, p, scheduler.BranchAndBound); len(res.Sessions) != 2 {
		t.Errorf("при отдыхе 12 часов поставлено %d тренировок", len(res.Sessions))
	}
}

func TestShortfallReasons(t *testing.T) {
	evenings := daily(hour(18, 0), hour(22, 0), weekdays()...)
	tests := []struct {
		name   string
		mutate func(p *scheduler.Problem)
		reason string
	}{
		{"мало игроков", func(p *scheduler.Problem) { p.Teams[0].Members = p.Teams[0].Members[:3] }, "в основном составе 3 игроков"},
		{"состав не собирается", func(p *scheduler.Problem) {
			p.Teams[0].Members[0].Free = daily(hour(8, 0), hour(12, 0), weekdays()...)
		}, "нет ни одного интервала"},
		{"залы заняты", func(p *scheduler.Problem) { p.Rooms[0] = scheduler.Room{ID: 1} }, "в залах нет свободных мест"},
		{"не хватает дней", func(p *scheduler.Problem) {
			p.Teams[0].Sessions = 6
		}, "совместимых между собой"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := scheduler.Problem{Teams: []scheduler.Team{team5(1, evenings)}, Rooms: []scheduler.Room{openRoom(1, 10)}, Weights: scheduler.DefaultWeights()}
			tt.mutate(&p)
			res := solve(t, p, scheduler.Greedy)
			if len(res.Stats.Shortfalls) != 1 {
				t.Fatalf("недобор: %+v", res.Stats.Shortfalls)
			}
			if got := res.Stats.Shortfalls[0].Reason; !strings.Contains(got, tt.reason) {
				t.Errorf("причина %q не содержит %q", got, tt.reason)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestBranchAndBoundBeatsGreedy — пример, где жадный выбор неоптимален.
// Команда A одинаково довольна понедельником и вторником и берёт более
// ранний понедельник. Команде B остаётся только поздний четверг со штрафом.
// Оптимум: A во вторник, B в понедельник.
func TestBranchAndBoundBeatsGreedy(t *testing.T) {
	mon := daily(hour(18, 0), hour(20, 0), 0)
	tue := daily(hour(18, 0), hour(20, 0), 1)
	thuLate := daily(hour(21, 0), hour(23, 0), 3)

	a := team5(1, mon.Or(tue))
	for i := range a.Members {
		a.Members[i].Preferred = mon.Or(tue)
	}
	b := team5(2, mon.Or(thuLate))
	a.Sessions, b.Sessions = 1, 1
	room := openRoom(1, 5)
	for s := hour(22, 0); s < hour(23, 0); s++ {
		room.Free[3*spd+s] = 5
	}
	p := scheduler.Problem{Teams: []scheduler.Team{a, b}, Rooms: []scheduler.Room{room}, Weights: scheduler.DefaultWeights()}

	g := solve(t, p, scheduler.Greedy)
	bb := solve(t, p, scheduler.BranchAndBound)
	if !bb.Stats.Optimal {
		t.Error("метод ветвей и границ не завершил перебор")
	}
	if bb.Stats.Objective <= g.Stats.Objective {
		t.Fatalf("ветви и границы %.1f не лучше жадного %.1f", bb.Stats.Objective, g.Stats.Objective)
	}
	if got := days(bb, 1); !slices.Equal(got, []int{1}) {
		t.Errorf("команда A в день %v, ожидался вторник", got)
	}
	if got := days(bb, 2); !slices.Equal(got, []int{0}) {
		t.Errorf("команда B в день %v, ожидался понедельник", got)
	}
}

// TestRandomInstances — свойства на синтетических задачах: оба решения
// допустимы, метод ветвей и границ не хуже жадного, результат детерминирован.
func TestRandomInstances(t *testing.T) {
	for seed := uint64(1); seed <= 25; seed++ {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			p := synth.Generate(synth.Default(seed, 2+int(seed%4)))
			g := solve(t, p, scheduler.Greedy)
			bb := solve(t, p, scheduler.BranchAndBound)
			if bb.Stats.Objective < g.Stats.Objective-1e-6 {
				t.Errorf("ветви и границы %.2f хуже жадного %.2f", bb.Stats.Objective, g.Stats.Objective)
			}
			if bb.Stats.Scheduled < g.Stats.Scheduled {
				t.Errorf("ветви и границы поставили %d < %d", bb.Stats.Scheduled, g.Stats.Scheduled)
			}
			g2 := solve(t, p, scheduler.Greedy)
			if g2.Stats.Objective != g.Stats.Objective || !slices.EqualFunc(g.Sessions, g2.Sessions, sameSession) {
				t.Error("жадный алгоритм недетерминирован")
			}
		})
	}
}

func sameSession(a, b scheduler.Session) bool {
	return a.TeamID == b.TeamID && a.RoomID == b.RoomID && a.Start == b.Start
}

func TestLimitsReturnValidSolution(t *testing.T) {
	p := synth.Generate(synth.Default(7, 8))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := scheduler.Solve(ctx, p, scheduler.Options{Algorithm: scheduler.BranchAndBound})
	if err != nil {
		t.Fatal(err)
	}
	verify(t, p, res)
	if res.Stats.Optimal {
		t.Error("отменённый поиск помечен как оптимальный")
	}

	res, err = scheduler.Solve(context.Background(), p, scheduler.Options{Algorithm: scheduler.BranchAndBound, MaxNodes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	verify(t, p, res)
	if res.Stats.Optimal || res.Stats.Nodes > 1000 {
		t.Errorf("лимит узлов: optimal=%v, узлов %d", res.Stats.Optimal, res.Stats.Nodes)
	}
}

func TestValidateProblem(t *testing.T) {
	good := team5(1, timegrid.Full())
	tests := []struct {
		name   string
		mutate func(p *scheduler.Problem)
	}{
		{"нет весов", func(p *scheduler.Problem) { p.Weights = scheduler.Weights{} }},
		{"повтор команды", func(p *scheduler.Problem) { p.Teams = append(p.Teams, p.Teams[0]) }},
		{"0 тренировок", func(p *scheduler.Problem) { p.Teams[0].Sessions = 0 }},
		{"пустое окно", func(p *scheduler.Problem) { p.Teams[0].DayEnd = p.Teams[0].DayStart }},
		{"0 мест", func(p *scheduler.Problem) { p.Teams[0].Seats = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := scheduler.Problem{Teams: []scheduler.Team{good}, Weights: scheduler.DefaultWeights()}
			tt.mutate(&p)
			if _, err := scheduler.Solve(context.Background(), p, scheduler.Options{}); err == nil {
				t.Error("ожидалась ошибка")
			}
		})
	}
	if _, err := scheduler.Solve(context.Background(), scheduler.Problem{Weights: scheduler.DefaultWeights()},
		scheduler.Options{Algorithm: "magic"}); err == nil {
		t.Error("неизвестный алгоритм принят")
	}
}

func TestEmptyProblem(t *testing.T) {
	res := solve(t, scheduler.Problem{Weights: scheduler.DefaultWeights()}, scheduler.BranchAndBound)
	if len(res.Sessions) != 0 || res.Stats.Objective != 0 || math.IsNaN(res.Stats.AvgAttendance) {
		t.Errorf("пустая задача: %+v", res.Stats)
	}
}

func BenchmarkSolve(b *testing.B) {
	for _, teams := range []int{4, 8} {
		p := synth.Generate(synth.Default(42, teams))
		for _, alg := range []scheduler.Algorithm{scheduler.Greedy, scheduler.BranchAndBound} {
			b.Run(fmt.Sprintf("%s/teams=%d", alg, teams), func(b *testing.B) {
				opt := scheduler.Options{Algorithm: alg, TimeLimit: 2 * time.Second}
				for b.Loop() {
					if _, err := scheduler.Solve(context.Background(), p, opt); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
