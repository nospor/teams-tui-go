package main

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func strp(s string) *string { return &s }

// newTestModel builds a Model with enough state for View() to render fully.
func newTestModel(t testing.TB, numChats, numMsgs int) Model {
	t.Helper()
	app := NewApp()

	now := time.Now()
	for i := 0; i < numChats; i++ {
		lu := now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		app.Chats = append(app.Chats, Chat{
			ID:                fmt.Sprintf("chat-%d", i),
			ChatType:          "oneOnOne",
			LastUpdated:       &lu,
			CachedDisplayName: strp(fmt.Sprintf("Person %d", i)),
		})
	}
	for i := 0; i < numMsgs; i++ {
		ts := now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		app.Messages = append(app.Messages, Message{
			ID:              fmt.Sprintf("msg-%d", i),
			CreatedDateTime: ts,
			MessageType:     "message",
			From: &MessageFrom{User: &MessageUser{
				ID:          strp(fmt.Sprintf("user-%d", i%3)),
				DisplayName: strp(fmt.Sprintf("Person %d", i%3)),
			}},
			Body: &MessageBody{Content: strp(
				"<p>A message body long enough to wrap across a few lines in the panel.</p>")},
		})
	}
	app.SelectedIndex = 0

	m := NewModel(app, "client", "user-0")
	m.width, m.height = 180, 45
	for i := range app.Chats {
		m.stableChatOrder = append(m.stableChatOrder, app.Chats[i].ID)
	}
	return m
}

// A tick that changes nothing must not invalidate the cached view.
func TestIdleTickReusesCachedView(t *testing.T) {
	m := newTestModel(t, 20, 60)

	first := m.View()
	if m.viewCache.dirty {
		t.Fatal("cache should be clean immediately after View()")
	}

	updated, _ := m.Update(MsgTick{})
	m2 := updated.(Model)

	if m2.viewCache.dirty {
		t.Error("an idle tick must not mark the view dirty")
	}
	if got := m2.View(); got != first {
		t.Error("cached view differs from freshly rendered view")
	}
}

// A tick that expires a status message must repaint.
func TestTickExpiringStatusInvalidatesCache(t *testing.T) {
	m := newTestModel(t, 5, 10)
	_ = m.View()

	past := time.Now().Add(-time.Second)
	m.app.Status = "sending..."
	m.app.StatusUntil = &past
	m.viewCache.dirty = false // simulate a clean cache before the tick

	updated, _ := m.Update(MsgTick{})
	m2 := updated.(Model)

	if !m2.viewCache.dirty {
		t.Fatal("tick that cleared an expired status must mark the view dirty")
	}
	if m2.app.Status != "" {
		t.Errorf("status not cleared: %q", m2.app.Status)
	}
}

// Non-tick messages must always invalidate, so real state changes repaint.
func TestKeypressInvalidatesCache(t *testing.T) {
	m := newTestModel(t, 20, 60)
	_ = m.View()
	if m.viewCache.dirty {
		t.Fatal("expected clean cache after View()")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if !updated.(Model).viewCache.dirty {
		t.Error("keypress must mark the view dirty")
	}
}

// A resize must repaint even if the dirty flag was somehow stale.
func TestResizeBypassesStaleCache(t *testing.T) {
	m := newTestModel(t, 10, 30)
	narrow := m.View()

	m.width, m.height = 120, 30
	m.viewCache.dirty = false // stale flag: dimensions changed underneath it

	if wide := m.View(); wide == narrow {
		t.Error("resize must produce a new render, not the cached one")
	}
}

// The cached path must be materially cheaper than a full render.
func BenchmarkViewCached(b *testing.B) {
	app := NewApp()
	for i := 0; i < 100; i++ {
		ts := time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		app.Chats = append(app.Chats, Chat{
			ID: fmt.Sprintf("c-%d", i), ChatType: "oneOnOne",
			LastUpdated: &ts, CachedDisplayName: strp(fmt.Sprintf("P%d", i)),
		})
		app.Messages = append(app.Messages, Message{
			ID: fmt.Sprintf("m-%d", i), CreatedDateTime: ts, MessageType: "message",
			From: &MessageFrom{User: &MessageUser{ID: strp("u"), DisplayName: strp("P")}},
			Body: &MessageBody{Content: strp("<p>hello there, a message body</p>")},
		})
	}
	app.SelectedIndex = 0
	m := NewModel(app, "client", "u")
	m.width, m.height = 180, 45

	b.Run("uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m.viewCache.dirty = true
			_ = m.View()
		}
	})
	b.Run("cached", func(b *testing.B) {
		_ = m.View()
		for i := 0; i < b.N; i++ {
			_ = m.View()
		}
	})
}

// writeAppState must still run when a non-tick message arrives, so unread
// counts and the state.json badge file stay current.
func TestNonTickStillUpdatesAppState(t *testing.T) {
	m := newTestModel(t, 5, 10)
	m.lastWrittenMessages = -1
	m.lastWrittenReactions = -1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m2 := updated.(Model)

	if m2.lastWrittenMessages == -1 {
		t.Error("writeAppState must run for non-tick messages")
	}
}

// A tick that DID work must still refresh app state, not just the view.
func TestWorkingTickUpdatesAppState(t *testing.T) {
	m := newTestModel(t, 5, 10)
	m.lastWrittenMessages = -1
	m.lastWrittenReactions = -1

	past := time.Now().Add(-time.Second)
	m.app.Status = "x"
	m.app.StatusUntil = &past

	updated, _ := m.Update(MsgTick{})
	if updated.(Model).lastWrittenMessages == -1 {
		t.Error("a tick that did work must still run writeAppState")
	}
}
