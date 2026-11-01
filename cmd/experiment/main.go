// Вычислительный эксперимент: сравнение жадного алгоритма и метода
// ветвей и границ на синтетических задачах (docs/research.md, раздел 5).
//
//	go run ./cmd/experiment -seeds 20 -teams 2,4,6,8,10 -limit 2s
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/scheduler"
	"github.com/Hamudahabibi/team-scheduler/internal/scheduler/synth"
)

type row struct {
	teams int

	gScheduled, bScheduled float64 // доля поставленных тренировок
	gQuality, bQuality     float64 // среднее качество тренировки
	gAtt, bAtt             float64 // средняя ожидаемая посещаемость
	gTime, bTime           time.Duration
	improved, optimal      int
	runs                   int
}

func main() {
	seeds := flag.Int("seeds", 20, "задач на каждую размерность")
	teamsFlag := flag.String("teams", "2,4,6,8,10", "числа команд через запятую")
	rooms := flag.Int("rooms", 2, "залов")
	seats := flag.Int("seats", 10, "мест в зале")
	load := flag.Float64("load", 0.1, "доля часов залов, занятых внешними мероприятиями")
	limit := flag.Duration("limit", 2*time.Second, "лимит времени метода ветвей и границ")
	topPerDay := flag.Int("top", scheduler.DefaultTopPerDay, "кандидатов на день для перебора")
	flag.Parse()

	var sizes []int
	for _, s := range strings.Split(*teamsFlag, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 {
			fmt.Fprintf(os.Stderr, "некорректное число команд %q\n", s)
			os.Exit(2)
		}
		sizes = append(sizes, n)
	}

	fmt.Printf("Задач на размерность: %d, залов: %d по %d мест, внешняя загрузка: %.0f%%, лимит B&B: %v, кандидатов на день: %d\n\n",
		*seeds, *rooms, *seats, *load*100, *limit, *topPerDay)
	fmt.Println("| Команд | Поставлено, жадный | Поставлено, B&B | Качество, жадный | Качество, B&B | Посещаемость, жадный | Посещаемость, B&B | B&B улучшил | Оптимум доказан | Время, жадный | Время, B&B |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|")

	for _, n := range sizes {
		r := row{teams: n}
		for seed := 1; seed <= *seeds; seed++ {
			cfg := synth.Default(uint64(seed), n)
			cfg.Rooms, cfg.RoomSeats, cfg.RoomLoad = *rooms, *seats, *load
			p := synth.Generate(cfg)

			g := run(p, scheduler.Options{Algorithm: scheduler.Greedy})
			b := run(p, scheduler.Options{Algorithm: scheduler.BranchAndBound, TimeLimit: *limit, TopPerDay: *topPerDay})

			r.runs++
			r.gScheduled += share(g)
			r.bScheduled += share(b)
			r.gQuality += quality(p, g)
			r.bQuality += quality(p, b)
			r.gAtt += g.Stats.AvgAttendance
			r.bAtt += b.Stats.AvgAttendance
			r.gTime += g.Stats.Elapsed
			r.bTime += b.Stats.Elapsed
			if b.Stats.Objective > g.Stats.Objective+1e-6 {
				r.improved++
			}
			if b.Stats.Optimal {
				r.optimal++
			}
		}
		k := float64(r.runs)
		fmt.Printf("| %d | %.1f%% | %.1f%% | %.1f | %.1f | %.1f%% | %.1f%% | %d из %d | %d из %d | %v | %v |\n",
			n, r.gScheduled/k*100, r.bScheduled/k*100, r.gQuality/k, r.bQuality/k,
			r.gAtt/k*100, r.bAtt/k*100, r.improved, r.runs, r.optimal, r.runs,
			(r.gTime / time.Duration(r.runs)).Round(time.Microsecond),
			(r.bTime / time.Duration(r.runs)).Round(time.Millisecond))
	}
}

func run(p scheduler.Problem, opt scheduler.Options) scheduler.Result {
	res, err := scheduler.Solve(context.Background(), p, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return res
}

func share(r scheduler.Result) float64 {
	if r.Stats.Requested == 0 {
		return 1
	}
	return float64(r.Stats.Scheduled) / float64(r.Stats.Requested)
}

// quality — средняя «качественная» часть целевой функции на тренировку:
// значение без вклада M за каждую поставленную тренировку.
func quality(p scheduler.Problem, r scheduler.Result) float64 {
	if r.Stats.Scheduled == 0 {
		return 0
	}
	return (r.Stats.Objective - p.Weights.Unit*float64(r.Stats.Scheduled)) / float64(r.Stats.Scheduled)
}
