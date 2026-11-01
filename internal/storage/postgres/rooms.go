package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Hamudahabibi/team-scheduler/internal/domain"
)

// CreateRoom сохраняет зал.
func (s *Store) CreateRoom(ctx context.Context, r domain.Room) (domain.Room, error) {
	err := s.db(ctx).QueryRow(ctx, `INSERT INTO rooms (name, seats) VALUES ($1, $2) RETURNING id`,
		r.Name, r.Seats).Scan(&r.ID)
	return r, mapErr(err, "зал")
}

// UpdateRoom меняет название и вместимость зала.
func (s *Store) UpdateRoom(ctx context.Context, r domain.Room) error {
	tag, err := s.db(ctx).Exec(ctx, `UPDATE rooms SET name = $2, seats = $3 WHERE id = $1`, r.ID, r.Name, r.Seats)
	return expectOne(tag, err, "зал")
}

// DeleteRoom удаляет зал.
func (s *Store) DeleteRoom(ctx context.Context, id int64) error {
	tag, err := s.db(ctx).Exec(ctx, `DELETE FROM rooms WHERE id = $1`, id)
	return expectOne(tag, err, "зал")
}

// ListRooms возвращает залы вместе с часами работы.
func (s *Store) ListRooms(ctx context.Context) ([]domain.Room, error) {
	rows, err := s.db(ctx).Query(ctx, `SELECT id, name, seats FROM rooms ORDER BY id`)
	if err != nil {
		return nil, mapErr(err, "залы")
	}
	rooms, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Room, error) {
		var room domain.Room
		err := r.Scan(&room.ID, &room.Name, &room.Seats)
		room.Hours = []domain.RoomHours{}
		return room, err
	})
	if err != nil {
		return nil, mapErr(err, "залы")
	}

	byID := make(map[int64]*domain.Room, len(rooms))
	for i := range rooms {
		byID[rooms[i].ID] = &rooms[i]
	}
	hrows, err := s.db(ctx).Query(ctx, `SELECT room_id, weekday, open_min, close_min FROM room_hours ORDER BY room_id, weekday`)
	if err != nil {
		return nil, mapErr(err, "часы работы")
	}
	defer hrows.Close()
	for hrows.Next() {
		var roomID int64
		var h domain.RoomHours
		if err := hrows.Scan(&roomID, &h.Weekday, &h.OpenMin, &h.CloseMin); err != nil {
			return nil, mapErr(err, "часы работы")
		}
		if r := byID[roomID]; r != nil {
			r.Hours = append(r.Hours, h)
		}
	}
	return rooms, mapErr(hrows.Err(), "часы работы")
}

// RoomByID возвращает зал с часами работы.
func (s *Store) RoomByID(ctx context.Context, id int64) (domain.Room, error) {
	var r domain.Room
	err := s.db(ctx).QueryRow(ctx, `SELECT id, name, seats FROM rooms WHERE id = $1`, id).Scan(&r.ID, &r.Name, &r.Seats)
	if err != nil {
		return r, mapErr(err, "зал")
	}
	rows, err := s.db(ctx).Query(ctx, `SELECT weekday, open_min, close_min FROM room_hours WHERE room_id = $1 ORDER BY weekday`, id)
	if err != nil {
		return r, mapErr(err, "часы работы")
	}
	r.Hours, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.RoomHours, error) {
		var h domain.RoomHours
		err := row.Scan(&h.Weekday, &h.OpenMin, &h.CloseMin)
		return h, err
	})
	return r, mapErr(err, "часы работы")
}

// ReplaceRoomHours заменяет расписание работы зала целиком.
func (s *Store) ReplaceRoomHours(ctx context.Context, roomID int64, hours []domain.RoomHours) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		// Блокировка строки зала заодно проверяет, что зал существует.
		var id int64
		if err := s.db(ctx).QueryRow(ctx, `SELECT id FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(&id); err != nil {
			return mapErr(err, "зал")
		}
		if _, err := s.db(ctx).Exec(ctx, `DELETE FROM room_hours WHERE room_id = $1`, roomID); err != nil {
			return mapErr(err, "часы работы")
		}
		for _, h := range hours {
			if _, err := s.db(ctx).Exec(ctx,
				`INSERT INTO room_hours (room_id, weekday, open_min, close_min) VALUES ($1, $2, $3, $4)`,
				roomID, h.Weekday, h.OpenMin, h.CloseMin); err != nil {
				return mapErr(err, "часы работы")
			}
		}
		return nil
	})
}

// CreateRoomBlock добавляет блокировку зала.
func (s *Store) CreateRoomBlock(ctx context.Context, b domain.RoomBlock) (domain.RoomBlock, error) {
	err := s.db(ctx).QueryRow(ctx, `
		INSERT INTO room_blocks (room_id, starts_at, ends_at, reason) VALUES ($1, $2, $3, $4)
		RETURNING id`, b.RoomID, b.StartsAt, b.EndsAt, b.Reason).Scan(&b.ID)
	return b, mapErr(err, "блокировка зала")
}

// DeleteRoomBlock удаляет блокировку зала.
func (s *Store) DeleteRoomBlock(ctx context.Context, roomID, blockID int64) error {
	tag, err := s.db(ctx).Exec(ctx, `DELETE FROM room_blocks WHERE id = $1 AND room_id = $2`, blockID, roomID)
	return expectOne(tag, err, "блокировка зала")
}

// RoomBlocks возвращает блокировки, пересекающие [from, to). roomID = 0 — все залы.
func (s *Store) RoomBlocks(ctx context.Context, roomID int64, from, to time.Time) ([]domain.RoomBlock, error) {
	rows, err := s.db(ctx).Query(ctx, `
		SELECT id, room_id, starts_at, ends_at, reason FROM room_blocks
		WHERE ($1 = 0 OR room_id = $1) AND starts_at < $3 AND ends_at > $2
		ORDER BY starts_at, id`, roomID, from, to)
	if err != nil {
		return nil, mapErr(err, "блокировки залов")
	}
	res, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.RoomBlock, error) {
		var b domain.RoomBlock
		err := r.Scan(&b.ID, &b.RoomID, &b.StartsAt, &b.EndsAt, &b.Reason)
		return b, err
	})
	return res, mapErr(err, "блокировки залов")
}
