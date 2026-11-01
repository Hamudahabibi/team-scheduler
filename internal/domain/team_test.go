package domain

import (
	"errors"
	"testing"
)

func TestTrainingPlanValidate(t *testing.T) {
	valid := TrainingPlan{
		SessionsPerWeek: 3, DurationMin: 150, MinCorePlayers: 5,
		DayStartMin: 15 * 60, DayEndMin: 23 * 60, MinRestMin: 12 * 60, Priority: 1,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("корректный план отклонён: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(p *TrainingPlan)
		field  string
	}{
		{"0 тренировок", func(p *TrainingPlan) { p.SessionsPerWeek = 0 }, "sessions_per_week"},
		{"8 тренировок", func(p *TrainingPlan) { p.SessionsPerWeek = 8 }, "sessions_per_week"},
		{"длительность не кратна 30", func(p *TrainingPlan) { p.DurationMin = 100 }, "duration_min"},
		{"нулевой состав", func(p *TrainingPlan) { p.MinCorePlayers = 0 }, "min_core_players"},
		{"начало не кратно 30", func(p *TrainingPlan) { p.DayStartMin = 15*60 + 10 }, "day_start_min"},
		{"конец после полуночи", func(p *TrainingPlan) { p.DayEndMin = 25 * 60 }, "day_end_min"},
		{"окно короче тренировки", func(p *TrainingPlan) { p.DayEndMin = 16 * 60 }, "day_end_min"},
		{"отрицательный отдых", func(p *TrainingPlan) { p.MinRestMin = -1 }, "min_rest_min"},
		{"приоритет 11", func(p *TrainingPlan) { p.Priority = 11 }, "priority"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.mutate(&p)
			var ve *ValidationError
			if err := p.Validate(); !errors.As(err, &ve) || ve.Field != tt.field {
				t.Fatalf("ожидалась ошибка поля %q, получено %v", tt.field, err)
			}
		})
	}
}
