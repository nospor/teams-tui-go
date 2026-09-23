package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseSearchQuerySupportsQuotesFieldsAndNegation(t *testing.T) {
	query := parseSearchQuery(`"quarterly plan" from:"Alice Smith" is:unread -has:file`)
	if len(query.Terms) != 4 {
		t.Fatalf("terms = %#v", query.Terms)
	}
	if query.Terms[0].Field != "" || query.Terms[0].Value != "quarterly plan" {
		t.Fatalf("quoted phrase parsed incorrectly: %#v", query.Terms[0])
	}
	if query.Terms[1].Field != "from" || query.Terms[1].Value != "Alice Smith" {
		t.Fatalf("quoted field parsed incorrectly: %#v", query.Terms[1])
	}
	if !query.Terms[3].Negated || query.Terms[3].Field != "has" {
		t.Fatalf("negated field parsed incorrectly: %#v", query.Terms[3])
	}
}

func TestComponentTextScoreUsesLiteralOrRegexpComponents(t *testing.T) {
	if _, matched := componentTextScore("Quarterly planning", "qp"); matched {
		t.Fatal("loose character subsequence unexpectedly matched")
	}
	if _, matched := componentTextScore("Quarterly planning", `q.*p`); !matched {
		t.Fatal("regexp component did not match")
	}
	if _, matched := componentTextScore("Quarterly planning", "plan"); !matched {
		t.Fatal("literal component did not match")
	}
	if _, matched := componentTextScore("Quarterly planning", "["); matched {
		t.Fatal("invalid regexp unexpectedly matched")
	}
	if _, matched := parseSearchQuery("plan q").Match(searchTarget{Text: []string{"Quarterly planning"}}); !matched {
		t.Fatal("order-independent literal components did not match")
	}
}

func TestParsedSearchQueryCompilesEachPatternOnce(t *testing.T) {
	query := parseSearchQuery(`planning q.*p [`)
	if len(query.Terms) != 3 {
		t.Fatalf("parsed terms = %#v", query.Terms)
	}
	if query.Terms[0].Pattern == nil || query.Terms[1].Pattern == nil {
		t.Fatal("valid search components were not precompiled")
	}
	if query.Terms[2].Pattern != nil {
		t.Fatal("invalid regexp component unexpectedly compiled")
	}
	if _, matched := query.Match(searchTarget{Text: []string{"planning q-to-p ["}}); !matched {
		t.Fatal("precompiled query changed literal-first matching")
	}
}

func TestStructuredMessageQuery(t *testing.T) {
	target := searchTarget{
		Text:         []string{"Quarterly roadmap approved"},
		Sender:       []string{"Alice Smith"},
		Conversation: []string{"Product planning"},
		Kind:         "message",
		CreatedAt:    time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local),
		Unread:       true,
		Favorite:     true,
		HasFile:      true,
	}
	query := parseSearchQuery(`quarter roadmap from:alice in:product is:unread is:favorite has:file after:2026-08-01 -type:event`)
	if _, matched := query.Match(target); !matched {
		t.Fatal("structured component query did not match target")
	}
	if _, matched := parseSearchQuery(`before:2026-08-01`).Match(target); matched {
		t.Fatal("date exclusion did not apply")
	}
}

func TestGlobalSearchUsesHiddenChatsAndAlreadyLoadedMessages(t *testing.T) {
	app := NewApp()
	visibleName := "Visible"
	hiddenName := "Planning archive"
	app.SetChats([]Chat{{ID: "visible", CachedDisplayName: &visibleName}})
	model := NewModel(app, "client", "user")
	hidden := Chat{ID: "hidden", CachedDisplayName: &hiddenName}
	model.latestChats = []Chat{app.Chats[0], hidden}
	model.stableChatOrder = []string{"visible", "hidden"}
	body := "The quarterly roadmap is ready"
	app.CachedMessages[hidden.ID] = []Message{{
		ID:              "message-1",
		CreatedDateTime: "2026-08-05T12:00:00Z",
		Body:            &MessageBody{Content: &body},
	}}

	app.UserSearchQuery = "quarter roadmap"
	model.updateUserSearchLocalResults()
	if len(app.UserSearchMessageResults) != 1 || app.UserSearchMessageResults[0].ChatID != hidden.ID {
		t.Fatalf("hidden loaded message missing from search: %#v", app.UserSearchMessageResults)
	}
	if len(app.CachedMessages[hidden.ID]) != 1 {
		t.Fatal("search mutated the existing message cache")
	}
}

func TestGlobalSearchRanksNameThenParticipantThenMessageSections(t *testing.T) {
	app := NewApp()
	nameTitle := "Roadmap Team"
	memberName := "Alice Example"
	memberEmail := "alice@example.com"
	body := "budget approval is ready"
	chat := Chat{
		ID:                "chat-1",
		CachedDisplayName: &nameTitle,
		Members: []ChatMember{{
			DisplayName: &memberName,
			Email:       &memberEmail,
		}},
		LastMessagePreview: &Message{ID: "message-1", Body: &MessageBody{Content: &body}},
	}
	app.SetChats([]Chat{chat})
	model := NewModel(app, "client", "user")
	model.latestChats = []Chat{chat}
	model.stableChatOrder = []string{chat.ID}

	app.UserSearchQuery = "roadmap"
	model.updateUserSearchLocalResults()
	if len(app.UserSearchLocalResults) != 1 || len(app.UserSearchMemberResults) != 0 {
		t.Fatalf("name search sections = names:%#v members:%#v", app.UserSearchLocalResults, app.UserSearchMemberResults)
	}

	app.UserSearchQuery = "alice"
	model.updateUserSearchLocalResults()
	if len(app.UserSearchLocalResults) != 0 || len(app.UserSearchMemberResults) != 1 {
		t.Fatalf("participant search sections = names:%#v members:%#v", app.UserSearchLocalResults, app.UserSearchMemberResults)
	}

	app.UserSearchQuery = "budget"
	model.updateUserSearchLocalResults()
	items := model.getUserSearchItems()
	if len(app.UserSearchLocalResults) != 0 || len(app.UserSearchMemberResults) != 0 || len(items) != 1 || items[0].Type != UserSearchItemMessage {
		t.Fatalf("message body leaked into chat sections: names=%#v members=%#v items=%#v", app.UserSearchLocalResults, app.UserSearchMemberResults, items)
	}
}

func TestTransientSearchInventoryDoesNotExpandSidebarUntilOpened(t *testing.T) {
	app := NewApp()
	visibleName := "Visible chat"
	oldName := "Archived planning"
	visible := Chat{ID: "visible", CachedDisplayName: &visibleName}
	old := Chat{ID: "old", CachedDisplayName: &oldName}
	app.SetChats([]Chat{visible})
	model := NewModel(app, "client", "user")
	model.latestChats = []Chat{visible}
	model.stableChatOrder = []string{visible.ID}
	model.searchChatInventory = []Chat{visible, old}
	model.searchChatInventoryLoaded = true
	app.UserSearchPopupMode = true
	app.UserSearchQuery = "archived"
	model.updateUserSearchLocalResults()

	if len(app.Chats) != 1 || app.Chats[0].ID != visible.ID {
		t.Fatalf("transient inventory changed sidebar before open: %#v", app.Chats)
	}
	if len(app.UserSearchLocalResults) != 1 || app.UserSearchLocalResults[0].ID != old.ID {
		t.Fatalf("old chat was not searchable: %#v", app.UserSearchLocalResults)
	}

	model, _ = model.handleUserSearchNavigationKey(tea.KeyMsg{Type: tea.KeyEnter})
	if selected := app.GetSelectedChat(); selected == nil || selected.ID != old.ID {
		t.Fatalf("opening old result selected %#v", selected)
	}
	found := false
	for _, chat := range model.latestChats {
		if chat.ID == old.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("opened old result was not hydrated into the chat list")
	}
}

func TestAsyncSearchInventoryPreservesHighlightedResultIdentity(t *testing.T) {
	app := NewApp()
	firstName := "Planning Alpha"
	secondName := "Planning Beta"
	olderName := "Planning Archive"
	first := Chat{ID: "first", CachedDisplayName: &firstName}
	second := Chat{ID: "second", CachedDisplayName: &secondName}
	older := Chat{ID: "older", CachedDisplayName: &olderName}
	app.SetChats([]Chat{first, second})
	app.UserSearchPopupMode = true
	app.UserSearchQuery = "planning"
	model := NewModel(app, "client", "user")
	model.latestChats = []Chat{first, second}
	model.stableChatOrder = []string{first.ID, second.ID}
	model.updateUserSearchLocalResults()
	app.UserSearchSelectedIndex = 1
	want := model.selectedUserSearchItemKey()

	model, _ = model.updateInternal(MsgSearchChatInventoryLoaded{Chats: []Chat{older, first, second}})
	if got := model.selectedUserSearchItemKey(); got != want {
		t.Fatalf("async inventory moved selection from %q to %q", want, got)
	}
}

func TestMessageMatchesUsesComponentQueryGrammar(t *testing.T) {
	body := "Hello world from Go"
	att := "report.pdf"
	msg := Message{
		ID: "1",
		From: &MessageFrom{
			User: &MessageUser{DisplayName: strp("Alice Smith")},
		},
		Body:        &MessageBody{Content: &body},
		Attachments: []MessageAttachment{{Name: &att}},
	}
	model := Model{}
	if !model.messageMatches(&msg, "from:Alice") {
		t.Fatal("from: field should match the sender")
	}
	if model.messageMatches(&msg, "from:Bob") {
		t.Fatal("from: field matched the wrong sender")
	}
	if !model.messageMatches(&msg, `h.*Go`) {
		t.Fatal("regexp component should match the body")
	}
}
