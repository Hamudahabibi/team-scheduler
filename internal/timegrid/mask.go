// Package timegrid описывает неделю как сетку 30-минутных слотов
// и множества слотов как битовые маски.
package timegrid

import "math/bits"

const (
	// SlotMinutes — длина слота.
	SlotMinutes = 30
	// SlotsPerDay — слотов в сутках.
	SlotsPerDay = 24 * 60 / SlotMinutes
	// Days — дней в неделе.
	Days = 7
	// SlotsPerWeek — слотов в неделе.
	SlotsPerWeek = Days * SlotsPerDay

	words = (SlotsPerWeek + 63) / 64
)

// Mask — множество слотов недели. Нулевое значение — пустое множество.
// Маска — значение, а не указатель: операции возвращают новую маску.
type Mask [words]uint64

// Full возвращает маску со всеми слотами недели.
func Full() Mask {
	var m Mask
	m.SetRange(0, SlotsPerWeek)
	return m
}

// Range возвращает маску слотов [from, to). Границы обрезаются по неделе.
func Range(from, to int) Mask {
	var m Mask
	m.SetRange(from, to)
	return m
}

// Set добавляет слот.
func (m *Mask) Set(slot int) {
	if slot < 0 || slot >= SlotsPerWeek {
		return
	}
	m[slot/64] |= 1 << (slot % 64)
}

// SetRange добавляет слоты [from, to).
func (m *Mask) SetRange(from, to int) {
	from = max(from, 0)
	to = min(to, SlotsPerWeek)
	for s := from; s < to; s++ {
		m[s/64] |= 1 << (s % 64)
	}
}

// Has сообщает, входит ли слот в маску.
func (m Mask) Has(slot int) bool {
	if slot < 0 || slot >= SlotsPerWeek {
		return false
	}
	return m[slot/64]&(1<<(slot%64)) != 0
}

// And — пересечение.
func (m Mask) And(o Mask) Mask {
	for i := range m {
		m[i] &= o[i]
	}
	return m
}

// Or — объединение.
func (m Mask) Or(o Mask) Mask {
	for i := range m {
		m[i] |= o[i]
	}
	return m
}

// AndNot — разность m \ o.
func (m Mask) AndNot(o Mask) Mask {
	for i := range m {
		m[i] &^= o[i]
	}
	return m
}

// IsEmpty сообщает, пусто ли множество.
func (m Mask) IsEmpty() bool {
	for _, w := range m {
		if w != 0 {
			return false
		}
	}
	return true
}

// Contains сообщает, входят ли в m все слоты o.
func (m Mask) Contains(o Mask) bool {
	return o.AndNot(m).IsEmpty()
}

// Count возвращает число слотов в маске.
func (m Mask) Count() int {
	n := 0
	for _, w := range m {
		n += bits.OnesCount64(w)
	}
	return n
}
