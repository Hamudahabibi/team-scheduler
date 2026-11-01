#!/usr/bin/env bash
# Сценарий приёмки (ТЗ, раздел 6) на чистой базе:
# расписания групп → залы → команды и планы → доступность →
# генерация → публикация → календарь → посещаемость.
#
# Нужны curl и jq. Сервер должен быть запущен с BOOTSTRAP_ADMIN_EMAIL=admin@stankin.ru:
#   make up
#   ./scripts/demo.sh
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
WEEK="${WEEK:-2026-11-09}"
PERIOD_FROM="2026-11-02"
PERIOD_TO="2026-12-28"
PASSWORD="demo-password"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

# call МЕТОД ПУТЬ [JSON] — запрос от имени $TOKEN; при ошибке HTTP печатает ответ и возвращает 1.
call() {
	local method=$1 path=$2 body=${3:-}
	local args=(-sS -X "$method" "$API$path" -H "Authorization: Bearer ${TOKEN:-}" -w '\n%{http_code}')
	[[ -n $body ]] && args+=(-H 'Content-Type: application/json' -d "$body")
	local out code
	out=$(curl "${args[@]}")
	code=${out##*$'\n'}
	out=${out%$'\n'*}
	if ((code >= 400)); then
		echo "Ошибка $code на $method $path: $out" >&2
		return 1
	fi
	printf '%s' "$out"
}

register() { # email ФИО группа → id
	local id
	id=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
		-d "{\"email\":\"$1\",\"password\":\"$PASSWORD\",\"full_name\":\"$2\",\"group_code\":\"$3\"}" | jq -r '.id // empty')
	if [[ -z $id ]]; then # уже зарегистрирован — повторный запуск сценария
		id=$(login "$1" | jq -r '.user.id')
	fi
	echo "$id"
}

login() { curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$1\",\"password\":\"$PASSWORD\"}"; }

step "Администратор"
register admin@stankin.ru "Администратор киберцентра" "" >/dev/null
TOKEN=$(login admin@stankin.ru | jq -r .token)
ADMIN_TOKEN=$TOKEN

step "Учебные группы и расписание пар"
for pair in ИДБ-22-04:idb-22-04 ИДБ-23-01:idb-23-01 МДБ-26-01:mdb-26-01; do
	code=${pair%%:*} file=${pair#*:}
	call POST /groups "{\"code\":\"$code\"}" >/dev/null 2>&1 || true
	gid=$(call GET /groups | jq -r --arg c "$code" '.[] | select(.code == $c) | .id')
	n=$(curl -sS -X PUT "$API/groups/$gid/classes?from=$PERIOD_FROM&to=$PERIOD_TO" \
		-H "Authorization: Bearer $TOKEN" -H 'Content-Type: text/calendar' \
		--data-binary "@$ROOT/examples/$file.ics" | jq -r .imported)
	echo "$code: загружено пар — $n"
done

step "Залы"
ROOMS=$(call GET /rooms)
if [[ $(jq length <<<"$ROOMS") -eq 0 ]]; then
	for room in '{"name":"Киберзал (ауд. 0101)","seats":10}' '{"name":"Малый зал (ауд. 0105)","seats":5}'; do
		rid=$(call POST /rooms "$room" | jq -r .id)
		call PUT "/rooms/$rid/hours" '[
			{"weekday":1,"open_min":600,"close_min":1320},{"weekday":2,"open_min":600,"close_min":1320},
			{"weekday":3,"open_min":600,"close_min":1320},{"weekday":4,"open_min":600,"close_min":1320},
			{"weekday":5,"open_min":600,"close_min":1320},{"weekday":6,"open_min":720,"close_min":1200}]' >/dev/null
	done
	BIG=$(call GET /rooms | jq -r '.[0].id')
	call POST "/rooms/$BIG/blocks" '{"starts_at":"2026-11-14T12:00:00+03:00","ends_at":"2026-11-14T20:00:00+03:00","reason":"Отборочные кубка вузов"}' >/dev/null
fi
call GET /rooms | jq -r '.[] | "\(.id). \(.name), мест: \(.seats)"'

step "Игроки"
PLAYERS=(
	"mlbb1@stankin.ru|Сорокин Артём|ИДБ-22-04" "mlbb2@stankin.ru|Соловьёв Даниил|ИДБ-22-04"
	"mlbb3@stankin.ru|Кузнецова Софья|ИДБ-22-04" "mlbb4@stankin.ru|Орлов Илья|ИДБ-23-01"
	"mlbb5@stankin.ru|Морозов Кирилл|ИДБ-23-01" "mlbb6@stankin.ru|Волков Егор|МДБ-26-01"
	"cs1@stankin.ru|Новиков Павел|ИДБ-23-01" "cs2@stankin.ru|Фёдоров Максим|ИДБ-23-01"
	"cs3@stankin.ru|Лебедев Никита|ИДБ-22-04" "cs4@stankin.ru|Козлов Тимур|МДБ-26-01"
	"cs5@stankin.ru|Попов Арсений|МДБ-26-01"
)
for p in "${PLAYERS[@]}"; do
	IFS='|' read -r email name group <<<"$p"
	register "$email" "$name" "$group" >/dev/null
done
echo "Зарегистрировано игроков: ${#PLAYERS[@]}"
uid() { login "$1@stankin.ru" | jq -r .user.id; }
COACH_ID=$(register coach@stankin.ru "Тренер Иванов" "")
call PUT "/users/$COACH_ID/role" '{"role":"coach"}' >/dev/null

step "Команды, составы и тренировочные планы"
team_id() { call GET /teams | jq -r --arg n "$1" '.[] | select(.name == $n) | .id'; }
MLBB=$(team_id "COSMos MLBB")
[[ -z $MLBB ]] && MLBB=$(call POST /teams "{\"name\":\"COSMos MLBB\",\"discipline_id\":1,\"coach_id\":$COACH_ID}" | jq -r .id)
CS=$(team_id "COSMos CS2")
[[ -z $CS ]] && CS=$(call POST /teams '{"name":"COSMos CS2","discipline_id":2}' | jq -r .id)

for e in mlbb1 mlbb2 mlbb3 mlbb4 mlbb5; do call PUT "/teams/$MLBB/members/$(uid $e)" '{"role":"core"}' >/dev/null; done
call PUT "/teams/$MLBB/members/$(uid mlbb6)" '{"role":"substitute"}' >/dev/null
for e in cs1 cs2 cs3 cs4 cs5; do call PUT "/teams/$CS/members/$(uid $e)" '{"role":"core"}' >/dev/null; done

# MLBB: 3 тренировки по 2,5 часа с 15:00 до 22:00, отдых 12 часов.
call PUT "/teams/$MLBB/plan" '{"sessions_per_week":3,"duration_min":150,"min_core_players":5,"day_start_min":900,"day_end_min":1320,"min_rest_min":720,"priority":2}' >/dev/null
# CS2: 4 тренировки по 2 часа с 12:00 до 22:00.
call PUT "/teams/$CS/plan" '{"sessions_per_week":4,"duration_min":120,"min_core_players":5,"day_start_min":720,"day_end_min":1320,"min_rest_min":0,"priority":1}' >/dev/null
call GET /teams | jq -r '.[] | "\(.id). \(.name)"'

step "Доступность игроков"
# Капитан MLBB вечерами свободен по будням и предпочитает 18:00–21:00.
TOKEN=$(login mlbb1@stankin.ru | jq -r .token)
call PUT /me/availability '[
	{"weekday":1,"start_min":960,"end_min":1380,"kind":"available"},{"weekday":2,"start_min":960,"end_min":1380,"kind":"available"},
	{"weekday":3,"start_min":960,"end_min":1380,"kind":"available"},{"weekday":4,"start_min":960,"end_min":1380,"kind":"available"},
	{"weekday":5,"start_min":960,"end_min":1380,"kind":"available"},
	{"weekday":1,"start_min":1080,"end_min":1260,"kind":"preferred"},{"weekday":3,"start_min":1080,"end_min":1260,"kind":"preferred"},
	{"weekday":5,"start_min":1080,"end_min":1260,"kind":"preferred"}]' >/dev/null
# Игрок CS2 работает по вторникам и четвергам.
TOKEN=$(login cs1@stankin.ru | jq -r .token)
call POST /me/exceptions '{"starts_at":"2026-11-10T00:00:00+03:00","ends_at":"2026-11-11T00:00:00+03:00","kind":"unavailable","note":"работа"}' >/dev/null
call POST /me/exceptions '{"starts_at":"2026-11-12T00:00:00+03:00","ends_at":"2026-11-13T00:00:00+03:00","kind":"unavailable","note":"работа"}' >/dev/null
echo "Правила и исключения сохранены"

step "Тепловая карта MLBB на понедельник (игроков основного состава по получасам с 12:00)"
TOKEN=$(login coach@stankin.ru | jq -r .token)
call GET "/teams/$MLBB/heatmap?week=$WEEK" | jq -c '.core_free[0][24:46]'

step "Генерация: жадный алгоритм и метод ветвей и границ"
TOKEN=$ADMIN_TOKEN
GREEDY=$(call POST /schedule/runs "{\"week_start\":\"$WEEK\",\"algorithm\":\"greedy\"}")
BNB=$(call POST /schedule/runs "{\"week_start\":\"$WEEK\",\"algorithm\":\"bnb\"}")
for run in "$GREEDY" "$BNB"; do
	jq -r '"[\(.stats.algorithm)] поставлено \(.stats.scheduled) из \(.stats.requested), целевая функция \(.objective | floor), оптимум доказан: \(.stats.optimal)"' <<<"$run"
done
RUN_ID=$(jq -r .id <<<"$BNB")
jq -r '.sessions[] | "  \(.starts_at[0:16] | sub("T"; " ")) – \(.ends_at[11:16])  \(.team_name)  \(.room_name)  посещаемость \(.expected_attendance * 100 | floor)%"' <<<"$BNB"
jq -r '.stats.shortfalls[] | "  Недобор у команды \(.team_id): \(.reason)"' <<<"$BNB"

step "Публикация черновика $RUN_ID"
call POST "/schedule/runs/$RUN_ID/publish" | jq -r '"Статус: \(.status)"'
call POST "/schedule/runs/$RUN_ID/publish" >/dev/null 2>&1 && echo "Повторная публикация прошла — это ошибка" || echo "Повторная публикация отклонена (409), как и должно быть"

step "Расписание игрока на неделю"
TOKEN=$(login mlbb1@stankin.ru | jq -r .token)
call GET "/me/schedule?week=$WEEK" | jq -r '"Тренировок: \(.trainings | length), пар: \(.classes | length)"'

step "Экспорт в календарь"
curl -sS "$API/teams/$MLBB/calendar.ics?from=$WEEK&to=2026-11-16" -H "Authorization: Bearer $TOKEN" | grep -E '^(SUMMARY|DTSTART|LOCATION)' | head -6

echo
echo "Готово."
