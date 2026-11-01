package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// queryTime читает момент времени из query-параметра. Принимает RFC 3339
// или дату YYYY-MM-DD (полночь в часовом поясе киберцентра).
func (a *api) queryTime(r *http.Request, name string) (time.Time, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return time.Time{}, domain.Invalid(name, "обязательный параметр")
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(time.DateOnly, v, a.Loc); err == nil {
		return t, nil
	}
	return time.Time{}, domain.Invalid(name, "ожидается дата YYYY-MM-DD или время RFC 3339")
}

// queryPeriod читает пару параметров from и to.
func (a *api) queryPeriod(r *http.Request) (from, to time.Time, err error) {
	if from, err = a.queryTime(r, "from"); err != nil {
		return
	}
	to, err = a.queryTime(r, "to")
	return
}

// queryInt64 читает необязательный числовой параметр, по умолчанию 0.
func queryInt64(r *http.Request, name string) (int64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, domain.Invalid(name, "ожидается неотрицательное целое число")
	}
	return n, nil
}
