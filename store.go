package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sqlite"
)

type note struct {
	ID        int64
	Markdown  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type noteSummary struct {
	ID        int64
	Title     string
	UpdatedAt time.Time
}

type store struct {
	db   *sqlite.DB
	path string
}

func openStore() (*store, error) {
	dir := os.Getenv("MOTE_DATA_DIR")
	if dir == "" {
		var err error
		dir, err = mygo.App.Path(mygo.PathUserData)
		if err != nil {
			return nil, err
		}
		if err := migrateLegacyData(dir); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	databasePath := filepath.Join(dir, "notes.db")
	db, err := sqlite.Open(context.Background(), databasePath, sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	s := &store{db: db, path: databasePath}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func migrateLegacyData(newDir string) error {
	appData, err := mygo.App.Path(mygo.PathAppData)
	if err != nil {
		return err
	}
	return migrateLegacyDataFrom(appData, newDir)
}

func migrateLegacyDataFrom(appData, newDir string) error {
	newDatabase := filepath.Join(newDir, "notes.db")
	if _, err := os.Stat(newDatabase); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	oldDir := filepath.Join(appData, "Markdown 备忘录")
	oldDatabase := filepath.Join(oldDir, "notes.db")
	if _, err := os.Stat(oldDatabase); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		return err
	}
	if err := os.Rename(oldDatabase, newDatabase); err != nil {
		return fmt.Errorf("move legacy database to Mote: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		oldSidecar := oldDatabase + suffix
		newSidecar := newDatabase + suffix
		if err := os.Rename(oldSidecar, newSidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("move legacy database sidecar to Mote: %w", err)
		}
	}
	_ = os.Remove(oldDir)
	return nil
}

func openMemoryStore() (*store, error) {
	db, err := sqlite.Open(context.Background(), ":memory:", sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	s := &store{db: db, path: ":memory:"}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) migrate(ctx context.Context) error {
	statements := []sqlite.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			markdown TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`},
		{SQL: `CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS notes_updated_at ON notes(updated_at DESC)`},
		{SQL: `CREATE TABLE IF NOT EXISTS focus_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			phase TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL,
			completed_at INTEGER NOT NULL
		)`},
		{SQL: `CREATE INDEX IF NOT EXISTS focus_sessions_completed_at ON focus_sessions(completed_at DESC)`},
	}
	_, err := s.db.Transaction(ctx, statements)
	return err
}

func (s *store) Path() string {
	if s == nil || s.path == "" {
		return "notes.db"
	}
	return s.path
}

func (s *store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *store) LoadCurrent(ctx context.Context) (note, error) {
	rows, err := s.db.Query(ctx, `SELECT n.id, n.markdown, n.created_at, n.updated_at
		FROM notes n
		LEFT JOIN app_state a ON a.key = 'current_note_id' AND CAST(a.value AS INTEGER) = n.id
		ORDER BY CASE WHEN a.value IS NOT NULL THEN 0 ELSE 1 END, n.updated_at DESC
		LIMIT 1`)
	if err != nil {
		return note{}, err
	}
	if len(rows.Values) == 0 {
		return note{}, nil
	}
	return scanNote(rows.Values[0])
}

func (s *store) Load(ctx context.Context, id int64) (note, error) {
	rows, err := s.db.Query(ctx, `SELECT id, markdown, created_at, updated_at FROM notes WHERE id = ?`, id)
	if err != nil {
		return note{}, err
	}
	if len(rows.Values) == 0 {
		return note{}, os.ErrNotExist
	}
	if err := s.setCurrent(ctx, id); err != nil {
		return note{}, err
	}
	return scanNote(rows.Values[0])
}

func (s *store) Save(ctx context.Context, id int64, markdown string) (note, error) {
	now := time.Now().UnixMilli()
	if id == 0 {
		result, err := s.db.Execute(ctx, `INSERT INTO notes(markdown, created_at, updated_at) VALUES (?, ?, ?)`, markdown, now, now)
		if err != nil {
			return note{}, err
		}
		id = result.LastInsertID
	} else {
		result, err := s.db.Execute(ctx, `UPDATE notes SET markdown = ?, updated_at = ? WHERE id = ?`, markdown, now, id)
		if err != nil {
			return note{}, err
		}
		if result.Changes == 0 {
			return note{}, os.ErrNotExist
		}
	}
	if err := s.setCurrent(ctx, id); err != nil {
		return note{}, err
	}
	return s.Load(ctx, id)
}

func (s *store) Delete(ctx context.Context, id int64) error {
	if id == 0 {
		return nil
	}
	result, err := s.db.Execute(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if result.Changes == 0 {
		return os.ErrNotExist
	}
	_, err = s.db.Execute(ctx, `DELETE FROM app_state WHERE key = 'current_note_id' AND value = ?`, strconv.FormatInt(id, 10))
	return err
}

func (s *store) Recent(ctx context.Context, limit int) ([]noteSummary, error) {
	rows, err := s.db.Query(ctx, `SELECT id, markdown, updated_at FROM notes ORDER BY updated_at DESC, id DESC LIMIT ?`, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]noteSummary, 0, len(rows.Values))
	for _, row := range rows.Values {
		if len(row) != 3 {
			return nil, errors.New("sqlite: unexpected recent note row")
		}
		id, ok := row[0].(int64)
		if !ok {
			return nil, fmt.Errorf("sqlite: note id has type %T", row[0])
		}
		markdown, ok := row[1].(string)
		if !ok {
			return nil, fmt.Errorf("sqlite: markdown has type %T", row[1])
		}
		updated, ok := row[2].(int64)
		if !ok {
			return nil, fmt.Errorf("sqlite: updated_at has type %T", row[2])
		}
		out = append(out, noteSummary{ID: id, Title: noteTitle(markdown), UpdatedAt: time.UnixMilli(updated)})
	}
	return out, nil
}

func (s *store) LoadTimerState(ctx context.Context) (timerState, error) {
	rows, err := s.db.Query(ctx, `SELECT value FROM app_state WHERE key = 'timer_state'`)
	if err != nil {
		return timerState{}, err
	}
	if len(rows.Values) == 0 {
		return defaultTimerState(), nil
	}
	value, ok := rows.Values[0][0].(string)
	if !ok {
		return timerState{}, fmt.Errorf("sqlite: timer state has type %T", rows.Values[0][0])
	}
	var state timerState
	if err := json.Unmarshal([]byte(value), &state); err != nil {
		return timerState{}, err
	}
	return normalizeTimerState(state), nil
}

func (s *store) SaveTimerState(ctx context.Context, state timerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.Execute(ctx, `INSERT INTO app_state(key, value) VALUES ('timer_state', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(data))
	return err
}

func (s *store) RecordTimerCompletion(ctx context.Context, phase string, durationSeconds int, completedAt time.Time) error {
	_, err := s.db.Execute(ctx, `INSERT INTO focus_sessions(phase, duration_seconds, completed_at) VALUES (?, ?, ?)`,
		phase, durationSeconds, completedAt.UnixMilli())
	return err
}

func (s *store) TodayPomodoros(ctx context.Context, now time.Time) (int, error) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
	end := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location()).UnixMilli()
	rows, err := s.db.Query(ctx, `SELECT COUNT(*) FROM focus_sessions
		WHERE phase = 'focus' AND completed_at >= ? AND completed_at < ?`, start, end)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) != 1 || len(rows.Values[0]) != 1 {
		return 0, errors.New("sqlite: unexpected focus session count")
	}
	count, ok := rows.Values[0][0].(int64)
	if !ok {
		return 0, fmt.Errorf("sqlite: focus session count has type %T", rows.Values[0][0])
	}
	return int(count), nil
}

func (s *store) setCurrent(ctx context.Context, id int64) error {
	_, err := s.db.Execute(ctx, `INSERT INTO app_state(key, value) VALUES ('current_note_id', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.FormatInt(id, 10))
	return err
}

func scanNote(row []any) (note, error) {
	if len(row) != 4 {
		return note{}, errors.New("sqlite: unexpected note row")
	}
	id, ok := row[0].(int64)
	if !ok {
		return note{}, fmt.Errorf("sqlite: note id has type %T", row[0])
	}
	markdown, ok := row[1].(string)
	if !ok {
		return note{}, fmt.Errorf("sqlite: markdown has type %T", row[1])
	}
	created, ok := row[2].(int64)
	if !ok {
		return note{}, fmt.Errorf("sqlite: created_at has type %T", row[2])
	}
	updated, ok := row[3].(int64)
	if !ok {
		return note{}, fmt.Errorf("sqlite: updated_at has type %T", row[3])
	}
	return note{ID: id, Markdown: markdown, CreatedAt: time.UnixMilli(created), UpdatedAt: time.UnixMilli(updated)}, nil
}

func noteTitle(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#> -*+0123456789.[]xX")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		const maxRunes = 28
		runes := []rune(line)
		if len(runes) > maxRunes {
			return string(runes[:maxRunes])
		}
		return line
	}
	return "无标题记录"
}
