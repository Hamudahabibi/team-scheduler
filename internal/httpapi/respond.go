package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// maxBodyBytes ограничивает размер тела запроса. Файлы расписания
// iCalendar за семестр укладываются в этот лимит с большим запасом.
const maxBodyBytes = 4 << 20

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// respond отдаёт v со статусом status или ошибку, если она есть.
func respond(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, v)
}

// writeError переводит доменную ошибку в HTTP-ответ. Неизвестные ошибки
// логируются, клиент получает 500 без подробностей.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{errorDetail{
			Code: "validation", Message: ve.Message, Field: ve.Field,
		}})
	case errors.Is(err, domain.ErrValidation):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{errorDetail{Code: "validation", Message: err.Error()}})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{errorDetail{Code: "not_found", Message: err.Error()}})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, errorBody{errorDetail{Code: "conflict", Message: err.Error()}})
	case errors.Is(err, domain.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorBody{errorDetail{Code: "forbidden", Message: err.Error()}})
	case errors.Is(err, domain.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errorBody{errorDetail{Code: "unauthorized", Message: err.Error()}})
	default:
		loggerFrom(r.Context()).Error("внутренняя ошибка", slog.Any("err", err))
		writeJSON(w, http.StatusInternalServerError, errorBody{errorDetail{Code: "internal", Message: "внутренняя ошибка сервера"}})
	}
}

// decodeJSON читает тело запроса в dst. Лишние поля считаются ошибкой,
// чтобы опечатки в названиях полей не проходили молча.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return domain.Invalid("body", "пустое тело запроса")
		}
		return domain.Invalid("body", fmt.Sprintf("некорректный JSON: %v", err))
	}
	if dec.More() {
		return domain.Invalid("body", "в теле запроса больше одного JSON-объекта")
	}
	return nil
}
