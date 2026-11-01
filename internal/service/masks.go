package service

import (
	"context"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// playerMasks — доступность игрока на неделю в виде масок слотов.
type playerMasks struct {
	// Free — когда игрок может прийти: личные правила и исключения минус пары.
	Free timegrid.Mask
	// Preferred — подмножество Free, отмеченное игроком как предпочтительное.
	Preferred timegrid.Mask
	// Classes — слоты, занятые парами с учётом буфера.
	Classes timegrid.Mask
}

// buildPlayerMasks собирает доступность игрока из трёх источников.
//
//  1. Еженедельные правила. Если правил нет, игрок свободен всё время (ФТ-3.3).
//  2. Исключения: «могу» добавляет слоты, «не могу» убирает.
//  3. Пары группы с буфером до и после — всегда убираются: пропускать пары нельзя.
func buildPlayerMasks(w timegrid.Week, rules []domain.AvailabilityRule, exceptions []domain.AvailabilityException,
	classes []domain.ClassEvent, buffer time.Duration,
) playerMasks {
	var m playerMasks
	if len(rules) == 0 {
		m.Free = timegrid.Full()
	}
	for _, r := range rules {
		slots := timegrid.Weekly(r.Weekday, r.StartMin, r.EndMin)
		m.Free = m.Free.Or(slots)
		if r.Kind == domain.RulePreferred {
			m.Preferred = m.Preferred.Or(slots)
		}
	}
	for _, e := range exceptions {
		if e.Kind == domain.ExceptionAvailable {
			m.Free = m.Free.Or(w.Inside(e.StartsAt, e.EndsAt))
		}
	}
	// «Не могу» применяется после «могу»: при пересечении побеждает запрет.
	for _, e := range exceptions {
		if e.Kind == domain.ExceptionUnavailable {
			m.Free = m.Free.AndNot(w.Cover(e.StartsAt, e.EndsAt))
		}
	}
	for _, c := range classes {
		m.Classes = m.Classes.Or(w.Cover(c.StartsAt.Add(-buffer), c.EndsAt.Add(buffer)))
	}
	m.Free = m.Free.AndNot(m.Classes)
	m.Preferred = m.Preferred.And(m.Free)
	return m
}

// availabilitySource — данные, из которых строятся маски игроков.
type availabilitySource interface {
	RulesByUsers(ctx context.Context, userIDs []int64) ([]domain.AvailabilityRule, error)
	ExceptionsByUsers(ctx context.Context, userIDs []int64, from, to time.Time) ([]domain.AvailabilityException, error)
	ClassesByGroups(ctx context.Context, groupIDs []int64, from, to time.Time) ([]domain.ClassEvent, error)
}

// playerRef — игрок и его учебная группа.
type playerRef struct {
	UserID  int64
	GroupID *int64
}

// loadPlayerMasks строит маски для набора игроков тремя запросами к БД
// (правила, исключения, пары), без запроса на каждого игрока.
func loadPlayerMasks(ctx context.Context, src availabilitySource, w timegrid.Week, players []playerRef,
	buffer time.Duration,
) (map[int64]playerMasks, error) {
	if len(players) == 0 {
		return map[int64]playerMasks{}, nil
	}
	userIDs := make([]int64, 0, len(players))
	groupSet := map[int64]bool{}
	for _, p := range players {
		userIDs = append(userIDs, p.UserID)
		if p.GroupID != nil {
			groupSet[*p.GroupID] = true
		}
	}
	groupIDs := make([]int64, 0, len(groupSet))
	for id := range groupSet {
		groupIDs = append(groupIDs, id)
	}

	rules, err := src.RulesByUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	exceptions, err := src.ExceptionsByUsers(ctx, userIDs, w.Start(), w.End())
	if err != nil {
		return nil, err
	}
	// Пара, закончившаяся в воскресенье до полуночи предыдущей недели, буфером
	// может задеть понедельник, поэтому окно расширяется на буфер.
	classes, err := src.ClassesByGroups(ctx, groupIDs, w.Start().Add(-buffer), w.End().Add(buffer))
	if err != nil {
		return nil, err
	}

	rulesBy := map[int64][]domain.AvailabilityRule{}
	for _, r := range rules {
		rulesBy[r.UserID] = append(rulesBy[r.UserID], r)
	}
	excBy := map[int64][]domain.AvailabilityException{}
	for _, e := range exceptions {
		excBy[e.UserID] = append(excBy[e.UserID], e)
	}
	classesBy := map[int64][]domain.ClassEvent{}
	for _, c := range classes {
		classesBy[c.GroupID] = append(classesBy[c.GroupID], c)
	}

	res := make(map[int64]playerMasks, len(players))
	for _, p := range players {
		var cls []domain.ClassEvent
		if p.GroupID != nil {
			cls = classesBy[*p.GroupID]
		}
		res[p.UserID] = buildPlayerMasks(w, rulesBy[p.UserID], excBy[p.UserID], cls, buffer)
	}
	return res, nil
}
