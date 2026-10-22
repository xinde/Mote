package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreatesUpdatesAndRestoresRecords(t *testing.T) {
	st, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	first, err := st.Save(ctx, 0, "# 旅行清单\n\n- 充电器")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 {
		t.Fatal("new note has no id")
	}
	updated, err := st.Save(ctx, first.ID, "# 旅行清单\n\n- 充电器\n- 护照")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID {
		t.Fatalf("update created id %d, want %d", updated.ID, first.ID)
	}

	second, err := st.Save(ctx, 0, "## 下周工作\n\n完成发布说明。")
	if err != nil {
		t.Fatal(err)
	}
	current, err := st.LoadCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != second.ID || current.Markdown != second.Markdown {
		t.Fatalf("current note %#v, want %#v", current, second)
	}

	recent, err := st.Recent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent count %d, want 2", len(recent))
	}
	if recent[0].ID != second.ID || recent[0].Title != "下周工作" {
		t.Fatalf("first recent %#v", recent[0])
	}
}

func TestMigrateLegacyData(t *testing.T) {
	appData := t.TempDir()
	oldDir := filepath.Join(appData, "Markdown 备忘录")
	newDir := filepath.Join(appData, "Mote")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"notes.db":     "database",
		"notes.db-wal": "wal",
		"notes.db-shm": "shm",
	} {
		if err := os.WriteFile(filepath.Join(oldDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyDataFrom(appData, newDir); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"notes.db":     "database",
		"notes.db-wal": "wal",
		"notes.db-shm": "shm",
	} {
		got, err := os.ReadFile(filepath.Join(newDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s content %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("legacy directory still exists: %v", err)
	}
}

func TestMigrateLegacyDataDoesNotOverwriteMote(t *testing.T) {
	appData := t.TempDir()
	oldDir := filepath.Join(appData, "Markdown 备忘录")
	newDir := filepath.Join(appData, "Mote")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "notes.db"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "notes.db"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyDataFrom(appData, newDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(newDir, "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("new database overwritten with %q", got)
	}
}

func TestDeleteRemovesRecordAndResetsEditor(t *testing.T) {
	st, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	saved, err := st.Save(ctx, 0, "# 待删除")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Save(ctx, 0, "# 保留"); err != nil {
		t.Fatal(err)
	}

	s := newApp(st, saved, nil, defaultTimerState(), 0)
	s.autoSaveTimer = time.AfterFunc(time.Hour, func() {})
	s.deleteCurrent()

	if s.currentID != 0 || s.draft != "" || s.savedText != "" || s.phase != phaseIdle {
		t.Fatalf("editor not reset after delete: %#v", s)
	}
	recent, err := st.Recent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Title != "保留" {
		t.Fatalf("records after delete: %#v", recent)
	}
	current, err := st.LoadCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID == saved.ID {
		t.Fatal("deleted record is still the current record")
	}
}

func TestDeleteIsIdempotentWithoutRecord(t *testing.T) {
	st, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := newApp(st, note{}, nil, defaultTimerState(), 0)
	s.deleteCurrent()
	if s.phase != phaseIdle || s.currentID != 0 {
		t.Fatalf("empty delete changed state: %#v", s)
	}
}

func TestNoteTitle(t *testing.T) {
	tests := map[string]string{
		"# 今天想到的事\n正文":  "今天想到的事",
		"\n- [x] 买咖啡\n": "买咖啡",
		"123. 整理桌面":     "整理桌面",
		"":              "无标题记录",
		"# 这是一个超过二十八个汉字因此应该被安静截断而不是显示省略号的标题": "这是一个超过二十八个汉字因此应该被安静截断而不是显示省略",
	}
	for input, want := range tests {
		if got := noteTitle(input); got != want {
			t.Errorf("noteTitle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNewAppRestoresSavedState(t *testing.T) {
	current := note{ID: 7, Markdown: "# SQLite 记录"}
	s := newApp(nil, current, []noteSummary{{ID: 7, Title: "SQLite 记录"}}, defaultTimerState(), 0)
	if s.phase != phaseSaved {
		t.Fatalf("phase %v, want saved", s.phase)
	}
	if s.currentID != 7 || s.draft != current.Markdown {
		t.Fatalf("restored id %d draft %q", s.currentID, s.draft)
	}
}
