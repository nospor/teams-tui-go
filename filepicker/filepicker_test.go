package filepicker

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeDirEntry struct {
	name  string
	isDir bool
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return f.isDir }
func (f fakeDirEntry) Type() os.FileMode          { return 0 }
func (f fakeDirEntry) Info() (os.FileInfo, error) { return nil, os.ErrInvalid }

func TestReadDirMsgClampsSelection(t *testing.T) {
	m := New()
	m.Height = 10
	m.readDirSeq = 1
	m.CurrentDirectory = "/tmp"
	m.selected = 1

	entries := []os.DirEntry{fakeDirEntry{name: "only", isDir: false}}
	updated, _ := m.Update(readDirMsg{
		id:      m.id,
		seq:     1,
		path:    "/tmp",
		entries: entries,
	})

	if updated.selected != 0 {
		t.Fatalf("expected selected 0, got %d", updated.selected)
	}
	if len(updated.files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(updated.files))
	}
}

func TestReadDirMsgIgnoresStaleSeq(t *testing.T) {
	m := New()
	m.readDirSeq = 2
	m.CurrentDirectory = "/tmp"
	m.files = []os.DirEntry{
		fakeDirEntry{name: "keep", isDir: false},
		fakeDirEntry{name: "me", isDir: false},
	}
	m.selected = 1

	stale := readDirMsg{
		id:      m.id,
		seq:     1,
		path:    "/tmp",
		entries: []os.DirEntry{fakeDirEntry{name: "stale", isDir: false}},
	}
	updated, _ := m.Update(stale)

	if len(updated.files) != 2 || updated.files[0].Name() != "keep" {
		t.Fatalf("stale readDirMsg should not replace files")
	}
	if updated.selected != 1 {
		t.Fatalf("expected selected unchanged, got %d", updated.selected)
	}
}

func TestReadDirMsgIgnoresWrongPath(t *testing.T) {
	m := New()
	m.readDirSeq = 1
	m.CurrentDirectory = "/here"
	m.files = []os.DirEntry{fakeDirEntry{name: "a", isDir: false}}

	updated, _ := m.Update(readDirMsg{
		id:      m.id,
		seq:     1,
		path:    "/elsewhere",
		entries: []os.DirEntry{fakeDirEntry{name: "b", isDir: false}},
	})

	if len(updated.files) != 1 || updated.files[0].Name() != "a" {
		t.Fatalf("readDirMsg for wrong path should be ignored")
	}
}

func TestOpenDoesNotPanicWithOutOfRangeSelection(t *testing.T) {
	m := New()
	m.Height = 10
	m.files = []os.DirEntry{fakeDirEntry{name: "one", isDir: false}}
	m.selected = 3
	m.DirAllowed = true
	m.FileAllowed = true

	openKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	updated, _ := m.Update(openKey)

	if updated.selected != 0 {
		t.Fatalf("expected clamped selection 0, got %d", updated.selected)
	}
}
