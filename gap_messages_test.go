package main

import (
	"testing"
	"time"
)

func chatWithPreview(id, msgID string, when time.Time) Chat {
	return Chat{
		ID:                id,
		ChatType:          "oneOnOne",
		CachedDisplayName: strp(id),
		LastMessagePreview: &Message{
			ID:              msgID,
			CreatedDateTime: when.Format(time.RFC3339Nano),
			MessageType:     "message",
			From: &MessageFrom{User: &MessageUser{
				DisplayName: strp("Someone"),
			}},
			Body: &MessageBody{Content: strp("<p>" + msgID + "</p>")},
		},
	}
}

func TestInactiveChatQueuesFullPageInsteadOfPreview(t *testing.T) {
	app := NewApp()
	oldAt := time.Now().Add(-10 * time.Minute)
	newAt := time.Now()
	chat := chatWithPreview("chat-1", "old", oldAt)
	app.Chats = []Chat{chat}
	app.SelectedIndex = -1
	app.CachedMessages["chat-1"] = []Message{{
		ID:              "old",
		CreatedDateTime: oldAt.Format(time.RFC3339Nano),
	}}

	m := NewModel(app, "client", "user")
	m.stableChatOrder = []string{"chat-1"}
	m.lastMsgID["chat-1"] = "old"
	m.lastMsgTime["chat-1"] = oldAt
	m.focused = true

	fresh := chatWithPreview("chat-1", "newest", newAt)
	updated, _ := m.Update(MsgChatsLoaded{Chats: []Chat{fresh}})
	got := updated.(Model)

	cached := got.app.CachedMessages["chat-1"]
	if len(cached) != 1 || cached[0].ID != "old" {
		t.Fatalf("cache should keep the previous messages until the page arrives, got %+v", cached)
	}
	if !got.app.ChatCacheDirty["chat-1"] {
		t.Fatal("chat should stay dirty until the recent page is merged")
	}
	if !got.gapFetchInFlight["chat-1"] {
		t.Fatal("inactive chat with a new preview should start a recent-page fetch")
	}
	pending, ok := got.pendingBurstNotify["chat-1"]
	if !ok {
		t.Fatal("notification should wait for the recent page")
	}
	if pending.PrevID != "old" {
		t.Fatalf("cutoff id = %q", pending.PrevID)
	}
	if len(got.gapFetchQueue) != 0 {
		t.Fatalf("single chat should leave the queue, got %v", got.gapFetchQueue)
	}
}

func TestActiveChatDoesNotQueueGapFetch(t *testing.T) {
	app := NewApp()
	oldAt := time.Now().Add(-10 * time.Minute)
	newAt := time.Now()
	chat := chatWithPreview("chat-1", "old", oldAt)
	app.Chats = []Chat{chat}
	app.SelectedIndex = 0
	app.Messages = []Message{{
		ID:              "old",
		CreatedDateTime: oldAt.Format(time.RFC3339Nano),
	}}

	m := NewModel(app, "client", "user")
	m.stableChatOrder = []string{"chat-1"}
	m.lastMsgID["chat-1"] = "old"
	m.lastMsgTime["chat-1"] = oldAt
	m.focused = true

	fresh := chatWithPreview("chat-1", "newest", newAt)
	updated, _ := m.Update(MsgChatsLoaded{Chats: []Chat{fresh}})
	got := updated.(Model)

	if got.gapFetchInFlight["chat-1"] || len(got.gapFetchQueue) != 0 {
		t.Fatal("the focused selected chat is refreshed by the normal message load")
	}
	if len(got.pendingBurstNotify) != 0 {
		t.Fatal("the focused chat should not hold a burst notification")
	}
	if len(got.app.Messages) == 0 || got.app.Messages[0].ID != "newest" {
		t.Fatalf("active chat should show the newest preview immediately, got %+v", got.app.Messages)
	}
}

func TestGapFetchMergesEveryNewMessage(t *testing.T) {
	app := NewApp()
	oldAt := time.Now().Add(-10 * time.Minute)
	app.Chats = []Chat{{ID: "chat-1", ChatType: "oneOnOne", CachedDisplayName: strp("chat-1")}}
	app.SelectedIndex = -1
	app.CachedMessages["chat-1"] = []Message{{
		ID:              "old",
		CreatedDateTime: oldAt.Format(time.RFC3339Nano),
	}}
	app.ChatCacheDirty["chat-1"] = true

	m := NewModel(app, "client", "user")
	m.gapFetchInFlight["chat-1"] = true
	m.gapFetchQueue = []string{"chat-2"}

	page := []Message{
		{ID: "m3", CreatedDateTime: time.Now().Format(time.RFC3339Nano)},
		{ID: "m2", CreatedDateTime: time.Now().Add(-time.Second).Format(time.RFC3339Nano)},
		{ID: "m1", CreatedDateTime: time.Now().Add(-2 * time.Second).Format(time.RFC3339Nano)},
	}
	updated, _ := m.Update(MsgGapMessagesLoaded{
		ChatID:   "chat-1",
		Messages: page,
		NextLink: "next",
		OK:       true,
	})
	got := updated.(Model)

	ids := map[string]bool{}
	for _, msg := range got.app.CachedMessages["chat-1"] {
		ids[msg.ID] = true
	}
	for _, id := range []string{"old", "m1", "m2", "m3"} {
		if !ids[id] {
			t.Errorf("cache missing %s; got %+v", id, got.app.CachedMessages["chat-1"])
		}
	}
	if got.app.ChatCacheDirty["chat-1"] {
		t.Fatal("successful page should clear the dirty flag")
	}
	if !got.app.ChatMessagesLoadedOnce["chat-1"] {
		t.Fatal("successful page should mark the chat loaded")
	}
	if got.app.CachedNextLink["chat-1"] != "next" {
		t.Fatalf("next link = %q", got.app.CachedNextLink["chat-1"])
	}
	if got.gapFetchInFlight["chat-1"] {
		t.Fatal("completed fetch should leave the in-flight set")
	}
	if !got.gapFetchInFlight["chat-2"] {
		t.Fatal("a free slot should start the next queued chat")
	}
}

func TestGapFetchesAreCapped(t *testing.T) {
	app := NewApp()
	oldAt := time.Now().Add(-10 * time.Minute)
	newAt := time.Now()
	var chats []Chat
	m := NewModel(app, "client", "user")
	m.focused = true
	m.app.SelectedIndex = -1
	for i := 0; i < maxConcurrentGapFetches+2; i++ {
		id := "chat-" + string(rune('a'+i))
		chats = append(chats, chatWithPreview(id, "old-"+id, oldAt))
		m.stableChatOrder = append(m.stableChatOrder, id)
		m.lastMsgID[id] = "old-" + id
		m.lastMsgTime[id] = oldAt
	}
	m.app.Chats = chats
	var fresh []Chat
	for i := range chats {
		id := chats[i].ID
		fresh = append(fresh, chatWithPreview(id, "new-"+id, newAt))
	}

	updated, _ := m.Update(MsgChatsLoaded{Chats: fresh})
	got := updated.(Model)

	if len(got.gapFetchInFlight) != maxConcurrentGapFetches {
		t.Fatalf("in flight = %d, want %d", len(got.gapFetchInFlight), maxConcurrentGapFetches)
	}
	if len(got.gapFetchQueue) != 2 {
		t.Fatalf("queue = %v, want 2 waiting", got.gapFetchQueue)
	}
}

func TestOpenSkipsFetchWhileGapIsInFlight(t *testing.T) {
	app := NewApp()
	app.ChatMessagesLoadedOnce["chat-1"] = true
	app.ChatCacheDirty["chat-1"] = true
	app.CachedMessages["chat-1"] = []Message{{ID: "old", CreatedDateTime: time.Now().Add(-time.Hour).Format(time.RFC3339Nano)}}
	m := NewModel(app, "client", "user")
	m.gapFetchInFlight["chat-1"] = true
	m.gapFetchQueue = []string{"chat-1", "chat-2"}

	got, cmd := m.loadChatMessages("chat-1")
	if cmd != nil {
		t.Fatal("opening a chat with a gap fetch in flight should not start another request")
	}
	if len(got.app.Messages) != 1 || got.app.Messages[0].ID != "old" {
		t.Fatalf("open should show the current cache, got %+v", got.app.Messages)
	}
	if len(got.gapFetchQueue) != 1 || got.gapFetchQueue[0] != "chat-2" {
		t.Fatalf("opening should drop this chat from the queue and leave the rest, got %v", got.gapFetchQueue)
	}
}

func burstMsg(id, sender, text string, when time.Time) Message {
	return Message{
		ID:              id,
		CreatedDateTime: when.Format(time.RFC3339Nano),
		MessageType:     "message",
		From:            &MessageFrom{User: &MessageUser{DisplayName: strp(sender)}},
		Body:            &MessageBody{Content: strp("<p>" + text + "</p>")},
	}
}

func TestBurstNotificationListsNewMessages(t *testing.T) {
	app := NewApp()
	app.NotificationShowPreview = true
	app.NotificationPreviewLen = 80
	app.Chats = []Chat{{ID: "chat-1", CachedDisplayName: strp("Ada")}}
	m := NewModel(app, "client", "user")

	base := time.Now().Add(-time.Minute)
	old := burstMsg("old", "Ada", "earlier", base)
	page := []Message{
		burstMsg("m4", "Dee", "four", base.Add(4*time.Second)),
		burstMsg("m2", "Bo", "two", base.Add(2*time.Second)),
		old,
		burstMsg("m1", "Ann", "one", base.Add(1*time.Second)),
		burstMsg("m3", "Cy", "three", base.Add(3*time.Second)),
	}
	fresh := newerMessages(page, "old", base)
	if len(fresh) != 4 {
		t.Fatalf("newer messages = %d, want 4", len(fresh))
	}
	title, body := m.formatBurst("chat-1", fresh)
	if title != "Ada (4)" {
		t.Fatalf("title = %q", title)
	}
	want := "Ann: one\nBo: two\nCy: three\nand 1 more"
	if body != want {
		t.Fatalf("body = %q", body)
	}

	oneTitle, oneBody := m.formatBurst("chat-1", fresh[:1])
	if oneTitle != "Ann" || oneBody != "one" {
		t.Fatalf("single notification = %q / %q", oneTitle, oneBody)
	}
}

func TestBurstNotifyClearsAfterPage(t *testing.T) {
	app := NewApp()
	app.Chats = []Chat{{ID: "chat-1", ChatType: "oneOnOne", CachedDisplayName: strp("Ada")}}
	app.SelectedIndex = -1
	m := NewModel(app, "client", "user")
	m.gapFetchInFlight["chat-1"] = true
	m.pendingBurstNotify["chat-1"] = burstNotify{
		PrevID:    "old",
		NotBefore: time.Now().Add(-time.Hour),
		Preview:   burstMsg("newest", "Ada", "last", time.Now()),
	}
	base := time.Now().Add(-time.Minute)
	updated, _ := m.Update(MsgGapMessagesLoaded{
		ChatID: "chat-1",
		Messages: []Message{
			burstMsg("m1", "Ann", "one", base),
			burstMsg("m2", "Bo", "two", base.Add(time.Second)),
		},
		OK: true,
	})
	got := updated.(Model)
	if _, ok := got.pendingBurstNotify["chat-1"]; ok {
		t.Fatal("the page should consume the pending notification")
	}
}
