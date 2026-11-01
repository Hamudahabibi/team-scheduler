package ical

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// ExportEvent — событие для выгрузки в календарь.
type ExportEvent struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
}

// Write записывает календарь с событиями. Время выгружается в UTC,
// календарь клиента сам переведёт его в местное.
func Write(w io.Writer, name string, events []ExportEvent, now time.Time) error {
	bw := bufio.NewWriter(w)
	line := func(s string) { writeFolded(bw, s) }

	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//team-scheduler//RU")
	line("CALSCALE:GREGORIAN")
	line("X-WR-CALNAME:" + escapeText(name))
	stamp := now.UTC().Format("20060102T150405Z")
	for _, e := range events {
		line("BEGIN:VEVENT")
		line("UID:" + e.UID)
		line("DTSTAMP:" + stamp)
		line("DTSTART:" + e.Start.UTC().Format("20060102T150405Z"))
		line("DTEND:" + e.End.UTC().Format("20060102T150405Z"))
		line("SUMMARY:" + escapeText(e.Summary))
		if e.Description != "" {
			line("DESCRIPTION:" + escapeText(e.Description))
		}
		if e.Location != "" {
			line("LOCATION:" + escapeText(e.Location))
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("запись календаря: %w", err)
	}
	return nil
}

// writeFolded пишет строку, перенося её по 75 байт (RFC 5545, раздел 3.1).
// Переносы не разрывают многобайтовые символы UTF-8.
func writeFolded(w *bufio.Writer, s string) {
	const limit = 75
	first := true
	for len(s) > 0 {
		max := limit
		if !first {
			max = limit - 1 // пробел в начале строки продолжения
		}
		cut := len(s)
		if cut > max {
			cut = max
			for cut > 0 && !isRuneStart(s[cut]) {
				cut--
			}
		}
		if !first {
			w.WriteByte(' ')
		}
		w.WriteString(s[:cut])
		w.WriteString("\r\n")
		s = s[cut:]
		first = false
	}
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func escapeText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}
