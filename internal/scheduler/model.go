// Package scheduler составляет недельное расписание тренировок.
//
// Пакет не знает про БД и HTTP: на вход — задача в слотах и битовых
// масках (Problem), на выход — выбранные тренировки и статистика.
// Формальная постановка и описание алгоритмов — в docs/research.md.
package scheduler

import (
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/timegrid"
)

// Member — игрок команды и его доступность на неделю.
type Member struct {
	UserID int64
	Core   bool
	// Free — слоты, когда игрок может прийти.
	Free timegrid.Mask
	// Preferred — предпочтительные слоты (подмножество Free).
	Preferred timegrid.Mask
}

// Team — команда и требования её тренировочного плана.
type Team struct {
	ID      int64
	Members []Member
	// Sessions — сколько тренировок нужно на неделе.
	Sessions int
	// Duration — длительность тренировки в слотах.
	Duration int
	// MinCore — минимум игроков основного состава.
	MinCore int
	// DayStart, DayEnd — допустимое окно внутри дня в слотах дня [DayStart, DayEnd).
	DayStart, DayEnd int
	// MinRest — минимальный перерыв между тренировками в слотах.
	MinRest int
	// Priority — множитель качества, 1..10.
	Priority int
	// Seats — сколько мест в зале занимает команда.
	Seats int
}

// Room — зал и число свободных мест в каждом слоте недели с учётом часов
// работы, блокировок и уже опубликованных тренировок других команд.
type Room struct {
	ID   int64
	Free [timegrid.SlotsPerWeek]int
}

// Weights — коэффициенты целевой функции.
type Weights struct {
	// Attendance — вес ожидаемой посещаемости (α).
	Attendance float64
	// Preference — вес попадания в предпочтительное время (β).
	Preference float64
	// Late — штраф за долю тренировки после LateSlot (γ).
	Late float64
	// Adjacent — штраф за тренировки одной команды в соседние дни (δ).
	Adjacent float64
	// Unit — ценность одной поставленной тренировки (M). Должна быть
	// больше любой суммы качеств, чтобы число тренировок было важнее качества.
	Unit float64
	// LateSlot — слот дня, с которого время считается поздним.
	LateSlot int
	// SubstituteWeight — вес запасного игрока в посещаемости (у основного — 1).
	SubstituteWeight float64
}

// DefaultWeights — коэффициенты по умолчанию (docs/research.md, раздел 3.4).
func DefaultWeights() Weights {
	return Weights{
		Attendance:       100,
		Preference:       20,
		Late:             15,
		Adjacent:         10,
		Unit:             10000,
		LateSlot:         21 * 60 / timegrid.SlotMinutes,
		SubstituteWeight: 0.5,
	}
}

// Problem — задача планирования на одну неделю.
type Problem struct {
	Teams   []Team
	Rooms   []Room
	Weights Weights
}

// Algorithm — метод решения.
type Algorithm string

const (
	Greedy         Algorithm = "greedy"
	BranchAndBound Algorithm = "bnb"
)

// Options — параметры решателя.
type Options struct {
	Algorithm Algorithm
	// TimeLimit — лимит времени метода ветвей и границ. 0 — без лимита.
	TimeLimit time.Duration
	// MaxNodes — лимит узлов дерева поиска. 0 — без лимита.
	MaxNodes int64
	// TopPerDay — сколько лучших кандидатов на день оставляет метод
	// ветвей и границ для каждой команды. 0 — значение по умолчанию.
	TopPerDay int
}

// DefaultTopPerDay — сколько кандидатов на день оставлять для перебора.
const DefaultTopPerDay = 6

// Session — запланированная тренировка.
type Session struct {
	TeamID int64
	RoomID int64
	// Start — первый слот недели, Duration — длительность в слотах.
	Start    int
	Duration int
	Seats    int
	// Quality — качество времени по целевой функции, без приоритета команды.
	Quality float64
	// Attendance — ожидаемая посещаемость: доля взвешенного состава, 0..1.
	Attendance float64
	// Attendees — игроки, которые свободны всё время тренировки.
	Attendees []int64
}

// Shortfall — команда, которой поставлено меньше тренировок, чем нужно.
type Shortfall struct {
	TeamID    int64  `json:"team_id"`
	Requested int    `json:"requested"`
	Scheduled int    `json:"scheduled"`
	Reason    string `json:"reason"`
}

// Stats — статистика решения.
type Stats struct {
	Algorithm     Algorithm `json:"algorithm"`
	Requested     int       `json:"requested"`
	Scheduled     int       `json:"scheduled"`
	Objective     float64   `json:"objective"`
	AvgAttendance float64   `json:"avg_attendance"`
	Candidates    int       `json:"candidates"`
	Nodes         int64     `json:"nodes"`
	// Optimal — метод ветвей и границ завершил перебор и доказал оптимальность
	// среди отобранных кандидатов. Для жадного алгоритма всегда false.
	Optimal    bool          `json:"optimal"`
	Elapsed    time.Duration `json:"elapsed_ns"`
	Shortfalls []Shortfall   `json:"shortfalls"`
}

// Result — решение задачи.
type Result struct {
	Sessions []Session
	Stats    Stats
}
