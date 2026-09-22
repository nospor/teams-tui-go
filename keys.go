package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbletea"
)

// KeyMap is the resolved set of bindings for every mode. Defaults live in
// DefaultKeyMap. Config keybindings replace listed actions; omitted actions
// keep their defaults.
type KeyMap struct {
	Normal            NormalKeys
	Compose           ComposeKeys
	Mention           MentionKeys
	Message           MessageKeys
	MessageView       MessageViewKeys
	Reaction          ReactionKeys
	DeleteConfirm     DeleteConfirmKeys
	URLList           URLListKeys
	SearchInput       SearchInputKeys
	SearchResults     SearchResultsKeys
	ChatSearchInput   ChatSearchInputKeys
	ChatSearchResults ChatSearchResultsKeys
	Help              HelpKeys
	Presence          PresenceKeys
	Profile           ProfileKeys
	FilePicker        FilePickerKeys
}

type NormalKeys struct {
	Next, Prev, Section, PageDown, PageUp                   key.Binding
	Notifications, Help, Compose, ChatSearch, Search        key.Binding
	Sleep, Messages, Favourite, Presence, ChannelHide, Quit key.Binding
}

type ComposeKeys struct {
	Cancel, Send, Newline, Editor, Attach, PasteImage key.Binding
}

type MentionKeys struct {
	Cancel, Prev, Next, Confirm key.Binding
}

type MessageKeys struct {
	Close, Next, Prev, React, Yank, Delete, Editor key.Binding
	Edit, Reply, YankURL, OpenURL, View            key.Binding
	Presence, Profile                              key.Binding
}

type MessageViewKeys struct {
	Close, Editor, Confirm, Download, Attachments key.Binding
	Next, Prev, PageDown, PageUp                  key.Binding
}

type ReactionKeys struct {
	Close, Like, Heart, Laugh, Surprised, Sad, Angry key.Binding
}

type DeleteConfirmKeys struct {
	Yes, No key.Binding
}

type URLListKeys struct {
	Close, Next, Prev, Confirm, Yank, OpenURL key.Binding
}

type SearchInputKeys struct {
	Cancel, Submit key.Binding
}

type SearchResultsKeys struct {
	Close, Next, Prev, EditQuery, Goto key.Binding
	Yank, Expand, YankURL, OpenURL     key.Binding
}

type ChatSearchInputKeys struct {
	Cancel, FocusResults, Submit key.Binding
}

type ChatSearchResultsKeys struct {
	Close, Next, Prev, EditQuery, Open key.Binding
}

type HelpKeys struct {
	Close, Next, Prev key.Binding
}

type PresenceKeys struct {
	Close, Next, Prev key.Binding
}

type ProfileKeys struct {
	Close key.Binding
}

type FilePickerKeys struct {
	Next, Prev, PageDown, PageUp    key.Binding
	Top, Bottom, Back, Open, Select key.Binding
	Sort, SortOrder, Hidden, Close  key.Binding
}

func bind(keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}

// DefaultKeyMap returns the built-in vim-style bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Normal: NormalKeys{
			Next:          bind("j", "down"),
			Prev:          bind("k", "up"),
			Section:       bind("tab"),
			PageDown:      bind("J", "pgdown"),
			PageUp:        bind("K", "pgup"),
			Notifications: bind("n"),
			Help:          bind("?"),
			Compose:       bind("i"),
			ChatSearch:    bind("c"),
			Search:        bind("/"),
			Sleep:         bind("esc"),
			Messages:      bind("m"),
			Favourite:     bind("f"),
			Presence:      bind("p"),
			ChannelHide:   bind("h"),
			Quit:          bind("q"),
		},
		Compose: ComposeKeys{
			Cancel:     bind("esc"),
			Send:       bind("enter"),
			Newline:    bind("alt+enter", "shift+enter", "ctrl+enter"),
			Editor:     bind("ctrl+g"),
			Attach:     bind("ctrl+f"),
			PasteImage: bind("ctrl+v", "ctrl+shift+v", "ctrl+V"),
		},
		Mention: MentionKeys{
			Cancel:  bind("esc"),
			Prev:    bind("up", "shift+tab"),
			Next:    bind("down", "tab"),
			Confirm: bind("enter"),
		},
		Message: MessageKeys{
			Close:    bind("esc", "m"),
			Next:     bind("j", "down"),
			Prev:     bind("k", "up"),
			React:    bind("r"),
			Yank:     bind("y"),
			Delete:   bind("d"),
			Editor:   bind("ctrl+g"),
			Edit:     bind("e"),
			Reply:    bind("a"),
			YankURL:  bind("u"),
			OpenURL:  bind("o"),
			View:     bind("v"),
			Presence: bind("p"),
			Profile:  bind("i"),
		},
		MessageView: MessageViewKeys{
			Close:       bind("esc", "q", "v"),
			Editor:      bind("ctrl+g"),
			Confirm:     bind("enter"),
			Download:    bind("d"),
			Attachments: bind("tab"),
			Next:        bind("j", "down"),
			Prev:        bind("k", "up"),
			PageDown:    bind("J", "shift+down", "pgdown"),
			PageUp:      bind("K", "shift+up", "pgup"),
		},
		Reaction: ReactionKeys{
			Close:     bind("esc", "r"),
			Like:      bind("1"),
			Heart:     bind("2"),
			Laugh:     bind("3"),
			Surprised: bind("4"),
			Sad:       bind("5"),
			Angry:     bind("6"),
		},
		DeleteConfirm: DeleteConfirmKeys{
			Yes: bind("y", "Y"),
			No:  bind("n", "N", "esc"),
		},
		URLList: URLListKeys{
			Close:   bind("esc", "q"),
			Next:    bind("j", "down"),
			Prev:    bind("k", "up"),
			Confirm: bind("enter"),
			Yank:    bind("y"),
			OpenURL: bind("o"),
		},
		SearchInput: SearchInputKeys{
			Cancel: bind("esc"),
			Submit: bind("enter"),
		},
		SearchResults: SearchResultsKeys{
			Close:     bind("esc", "q"),
			Next:      bind("j", "down"),
			Prev:      bind("k", "up"),
			EditQuery: bind("/"),
			Goto:      bind("g"),
			Yank:      bind("y"),
			Expand:    bind("enter"),
			YankURL:   bind("u"),
			OpenURL:   bind("o"),
		},
		ChatSearchInput: ChatSearchInputKeys{
			Cancel:       bind("esc"),
			FocusResults: bind("down", "up", "tab"),
			Submit:       bind("enter"),
		},
		ChatSearchResults: ChatSearchResultsKeys{
			Close:     bind("esc", "q"),
			Next:      bind("j", "down"),
			Prev:      bind("k", "up"),
			EditQuery: bind("/"),
			Open:      bind("enter"),
		},
		Help: HelpKeys{
			Close: bind("esc", "q", "?", "enter"),
			Next:  bind("j", "down"),
			Prev:  bind("k", "up"),
		},
		Presence: PresenceKeys{
			Close: bind("esc", "q", "p", "enter"),
			Next:  bind("j", "down"),
			Prev:  bind("k", "up"),
		},
		Profile: ProfileKeys{
			Close: bind("esc", "q", "i", "enter"),
		},
		FilePicker: FilePickerKeys{
			Next:      bind("j", "down", "ctrl+n"),
			Prev:      bind("k", "up", "ctrl+p"),
			PageDown:  bind("J", "pgdown"),
			PageUp:    bind("K", "pgup"),
			Top:       bind("g"),
			Bottom:    bind("G"),
			Back:      bind("h", "backspace", "left", "esc"),
			Open:      bind("l", "right", "enter"),
			Select:    bind("enter"),
			Sort:      bind("s", "ctrl+s"),
			SortOrder: bind("o", "ctrl+o"),
			Hidden:    bind("."),
			Close:     bind("esc", "q"),
		},
	}
}

// sharedAction is a key name that, when set at the top of keybindings,
// replaces that action in every mode which has it.
const (
	sharedNext     = "next"
	sharedPrev     = "prev"
	sharedPageDown = "page_down"
	sharedPageUp   = "page_up"
	sharedClose    = "close"
	sharedQuit     = "quit"
	sharedYank     = "yank"
	sharedYankURL  = "yank_url"
	sharedOpenURL  = "open_url"
	sharedConfirm  = "confirm"
	sharedCancel   = "cancel"
)

type explicitLevel int

const (
	expNone explicitLevel = iota
	expShared
	expMode
)

type keySlot struct {
	action string
	shared string
	bind   *key.Binding
}

func (k *KeyMap) slots(mode string) []keySlot {
	switch mode {
	case "normal":
		n := &k.Normal
		return []keySlot{
			{"quit", sharedQuit, &n.Quit},
			{"next", sharedNext, &n.Next},
			{"prev", sharedPrev, &n.Prev},
			{"section", "", &n.Section},
			{"notifications", "", &n.Notifications},
			{"help", "", &n.Help},
			{"compose", "", &n.Compose},
			{"chat_search", "", &n.ChatSearch},
			{"search", "", &n.Search},
			{"sleep", "", &n.Sleep},
			{"page_up", sharedPageUp, &n.PageUp},
			{"page_down", sharedPageDown, &n.PageDown},
			{"messages", "", &n.Messages},
			{"favourite", "", &n.Favourite},
			{"presence", "", &n.Presence},
			{"channel_hide", "", &n.ChannelHide},
		}
	case "compose":
		c := &k.Compose
		return []keySlot{
			{"cancel", sharedCancel, &c.Cancel},
			{"send", sharedConfirm, &c.Send},
			{"newline", "", &c.Newline},
			{"editor", "", &c.Editor},
			{"attach", "", &c.Attach},
			{"paste_image", "", &c.PasteImage},
		}
	case "mention":
		c := &k.Mention
		return []keySlot{
			{"cancel", sharedCancel, &c.Cancel},
			{"prev", sharedPrev, &c.Prev},
			{"next", sharedNext, &c.Next},
			{"confirm", sharedConfirm, &c.Confirm},
		}
	case "message":
		c := &k.Message
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
			{"react", "", &c.React},
			{"yank", sharedYank, &c.Yank},
			{"delete", "", &c.Delete},
			{"editor", "", &c.Editor},
			{"edit", "", &c.Edit},
			{"reply", "", &c.Reply},
			{"yank_url", sharedYankURL, &c.YankURL},
			{"open_url", sharedOpenURL, &c.OpenURL},
			{"view", "", &c.View},
			{"presence", "", &c.Presence},
			{"profile", "", &c.Profile},
		}
	case "message_view":
		c := &k.MessageView
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"editor", "", &c.Editor},
			{"confirm", sharedConfirm, &c.Confirm},
			{"download", "", &c.Download},
			{"attachments", "", &c.Attachments},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
			{"page_down", sharedPageDown, &c.PageDown},
			{"page_up", sharedPageUp, &c.PageUp},
		}
	case "reaction":
		c := &k.Reaction
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"like", "", &c.Like},
			{"heart", "", &c.Heart},
			{"laugh", "", &c.Laugh},
			{"surprised", "", &c.Surprised},
			{"sad", "", &c.Sad},
			{"angry", "", &c.Angry},
		}
	case "delete_confirm":
		c := &k.DeleteConfirm
		return []keySlot{
			{"yes", "", &c.Yes},
			{"no", "", &c.No},
		}
	case "url_list":
		c := &k.URLList
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
			{"confirm", sharedConfirm, &c.Confirm},
			{"yank", sharedYank, &c.Yank},
			{"open_url", sharedOpenURL, &c.OpenURL},
		}
	case "search_input":
		c := &k.SearchInput
		return []keySlot{
			{"cancel", sharedCancel, &c.Cancel},
			{"submit", sharedConfirm, &c.Submit},
		}
	case "search_results":
		c := &k.SearchResults
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
			{"edit_query", "", &c.EditQuery},
			{"goto", "", &c.Goto},
			{"yank", sharedYank, &c.Yank},
			{"expand", sharedConfirm, &c.Expand},
			{"yank_url", sharedYankURL, &c.YankURL},
			{"open_url", sharedOpenURL, &c.OpenURL},
		}
	case "chat_search_input":
		c := &k.ChatSearchInput
		return []keySlot{
			{"cancel", sharedCancel, &c.Cancel},
			{"focus_results", "", &c.FocusResults},
			{"submit", sharedConfirm, &c.Submit},
		}
	case "chat_search_results":
		c := &k.ChatSearchResults
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
			{"edit_query", "", &c.EditQuery},
			{"open", sharedConfirm, &c.Open},
		}
	case "help":
		c := &k.Help
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
		}
	case "presence":
		c := &k.Presence
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
		}
	case "profile":
		c := &k.Profile
		return []keySlot{
			{"close", sharedClose, &c.Close},
		}
	case "filepicker":
		c := &k.FilePicker
		return []keySlot{
			{"close", sharedClose, &c.Close},
			{"next", sharedNext, &c.Next},
			{"prev", sharedPrev, &c.Prev},
			{"page_down", sharedPageDown, &c.PageDown},
			{"page_up", sharedPageUp, &c.PageUp},
			{"top", "", &c.Top},
			{"bottom", "", &c.Bottom},
			{"back", "", &c.Back},
			{"open", "", &c.Open},
			{"select", sharedConfirm, &c.Select},
			{"sort", "", &c.Sort},
			{"sort_order", "", &c.SortOrder},
			{"hidden", "", &c.Hidden},
		}
	default:
		return nil
	}
}

var modeNames = []string{
	"normal", "compose", "mention", "message", "message_view", "reaction",
	"delete_confirm", "url_list", "search_input", "search_results",
	"chat_search_input", "chat_search_results", "help", "presence", "profile",
	"filepicker",
}

var sharedNames = []string{
	sharedNext, sharedPrev, sharedPageDown, sharedPageUp, sharedClose,
	sharedQuit, sharedYank, sharedYankURL, sharedOpenURL, sharedConfirm, sharedCancel,
}

func isModeName(name string) bool {
	for _, m := range modeNames {
		if m == name {
			return true
		}
	}
	return false
}

func isSharedName(name string) bool {
	for _, s := range sharedNames {
		if s == name {
			return true
		}
	}
	return false
}

// ResolveKeyMap starts from the defaults and applies a keybindings JSON
// object. An empty payload returns the defaults. Warnings describe unknown
// names and keys that had to be removed because two actions in one mode
// claimed them.
func ResolveKeyMap(raw json.RawMessage) (KeyMap, []string) {
	km := DefaultKeyMap()
	if len(raw) == 0 || string(raw) == "null" {
		return km, nil
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return km, []string{"keybindings: expected an object, using defaults"}
	}

	var warnings []string
	levels := map[string]map[string]explicitLevel{}

	var unknown []string
	for name := range doc {
		if !isModeName(name) && !isSharedName(name) {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		warnings = append(warnings, fmt.Sprintf("keybindings: unknown key %q ignored", name))
	}

	for _, shared := range sharedNames {
		rawKeys, ok := doc[shared]
		if !ok {
			continue
		}
		keys, err := parseKeyList(rawKeys)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("keybindings: %s: %s", shared, err))
			continue
		}
		for _, mode := range modeNames {
			for _, slot := range km.slots(mode) {
				if slot.shared != shared {
					continue
				}
				setBindingKeys(slot.bind, keys)
				setLevel(levels, mode, slot.action, expShared)
			}
		}
	}

	for _, mode := range modeNames {
		rawMode, ok := doc[mode]
		if !ok {
			continue
		}
		var actions map[string]json.RawMessage
		if err := json.Unmarshal(rawMode, &actions); err != nil {
			warnings = append(warnings, fmt.Sprintf("keybindings: %s: expected an object of actions", mode))
			continue
		}
		slots := km.slots(mode)
		var unknownActions []string
		for name := range actions {
			if findSlot(slots, name) == nil {
				unknownActions = append(unknownActions, name)
			}
		}
		sort.Strings(unknownActions)
		for _, name := range unknownActions {
			warnings = append(warnings, fmt.Sprintf("keybindings: %s.%s: unknown action ignored", mode, name))
		}
		for _, slot := range slots {
			rawKeys, ok := actions[slot.action]
			if !ok {
				continue
			}
			keys, err := parseKeyList(rawKeys)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("keybindings: %s.%s: %s", mode, slot.action, err))
				continue
			}
			setBindingKeys(slot.bind, keys)
			setLevel(levels, mode, slot.action, expMode)
		}
	}

	for _, mode := range modeNames {
		warnings = append(warnings, stripConflicts(mode, km.slots(mode), levels[mode])...)
	}
	return km, warnings
}

func setLevel(levels map[string]map[string]explicitLevel, mode, action string, level explicitLevel) {
	if levels[mode] == nil {
		levels[mode] = map[string]explicitLevel{}
	}
	levels[mode][action] = level
}

func findSlot(slots []keySlot, action string) *keySlot {
	for i := range slots {
		if slots[i].action == action {
			return &slots[i]
		}
	}
	return nil
}

func parseKeyList(raw json.RawMessage) ([]string, error) {
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("expected an array of key strings")
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		n := normalizeKey(k)
		if n == "" {
			return nil, fmt.Errorf("empty key string")
		}
		out = append(out, n)
	}
	return out, nil
}

func setBindingKeys(b *key.Binding, keys []string) {
	if len(keys) == 0 {
		b.Unbind()
		return
	}
	b.SetKeys(keys...)
}

func stripKey(b *key.Binding, drop string) {
	var keep []string
	for _, k := range b.Keys() {
		if k != drop {
			keep = append(keep, k)
		}
	}
	setBindingKeys(b, keep)
}

// stripConflicts removes a key from every action except the one that should
// keep it. Defaults may share a key (the file picker uses enter for both
// open and select). A key the user assigned explicitly wins over a default;
// if two explicit actions share it, the earlier action in the mode keeps it.
func stripConflicts(mode string, slots []keySlot, levels map[string]explicitLevel) []string {
	type owner struct {
		action string
		level  explicitLevel
		index  int
	}
	owners := map[string]owner{}
	var warnings []string
	for i, slot := range slots {
		level := expNone
		if levels != nil {
			level = levels[slot.action]
		}
		for _, keyName := range append([]string(nil), slot.bind.Keys()...) {
			prev, seen := owners[keyName]
			if !seen {
				owners[keyName] = owner{slot.action, level, i}
				continue
			}
			if level == expNone && prev.level == expNone {
				continue
			}
			keepPrev := prev.level >= level
			if keepPrev {
				stripKey(slot.bind, keyName)
				warnings = append(warnings, fmt.Sprintf("keybindings: %s: %q kept on %s, removed from %s", mode, keyName, prev.action, slot.action))
				continue
			}
			stripKey(slots[prev.index].bind, keyName)
			owners[keyName] = owner{slot.action, level, i}
			warnings = append(warnings, fmt.Sprintf("keybindings: %s: %q kept on %s, removed from %s", mode, keyName, slot.action, prev.action))
		}
	}
	return warnings
}

// namedKeys maps friendly config spellings onto Bubble Tea key names.
var namedKeys = map[string]string{
	"up": "up", "down": "down", "left": "left", "right": "right",
	"arrowup": "up", "arrowdown": "down", "arrowleft": "left", "arrowright": "right",
	"pgup": "pgup", "pageup": "pgup",
	"pgdown": "pgdown", "pgdn": "pgdown", "pagedown": "pgdown",
	"esc": "esc", "escape": "esc",
	"enter": "enter", "return": "enter",
	"tab": "tab", "backspace": "backspace", "space": "space",
	"delete": "delete", "del": "delete",
	"home": "home", "end": "end",
}

func normalizeKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	if strings.Contains(k, "+") {
		parts := strings.Split(k, "+")
		for i, p := range parts {
			p = strings.ToLower(strings.TrimSpace(p))
			if i == len(parts)-1 {
				if canon, ok := namedKeys[p]; ok {
					p = canon
				}
			}
			parts[i] = p
		}
		return strings.Join(parts, "+")
	}
	if canon, ok := namedKeys[strings.ToLower(k)]; ok {
		return canon
	}
	return k
}

var keyLabels = map[string]string{
	"up": "↑", "down": "↓", "left": "←", "right": "→",
	"pgup": "PgUp", "pgdown": "PgDn",
	"esc": "Esc", "enter": "Enter", "tab": "Tab",
	"backspace": "Backspace", "space": "Space", "delete": "Del",
	"home": "Home", "end": "End",
	"shift+tab": "Shift+Tab", "shift+up": "Shift+↑", "shift+down": "Shift+↓",
	"alt+enter": "Alt+Enter", "shift+enter": "Shift+Enter", "ctrl+enter": "Ctrl+Enter",
	"ctrl+c": "Ctrl+C", "ctrl+g": "Ctrl+G", "ctrl+f": "Ctrl+F",
	"ctrl+v": "Ctrl+V", "ctrl+shift+v": "Ctrl+Shift+V",
	"ctrl+n": "Ctrl+N", "ctrl+p": "Ctrl+P", "ctrl+s": "Ctrl+S", "ctrl+o": "Ctrl+O",
}

func keyLabel(k string) string {
	if label, ok := keyLabels[k]; ok {
		return label
	}
	if strings.Contains(k, "+") {
		parts := strings.Split(k, "+")
		for i, p := range parts {
			switch p {
			case "ctrl", "alt", "shift":
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			default:
				if len(p) == 1 {
					parts[i] = strings.ToUpper(p)
				}
			}
		}
		return strings.Join(parts, "+")
	}
	return k
}

// FormatKeys renders a binding for help text. sep is " / " in the help popup
// and "/" in compact bars. An unbound action renders as an em dash.
func FormatKeys(b key.Binding, sep string) string {
	keys := b.Keys()
	if len(keys) == 0 {
		return "—"
	}
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = keyLabel(k)
	}
	return strings.Join(labels, sep)
}

// slashKeys joins every key of the given bindings with slashes, for compact
// status titles such as "j/↓/k/↑".
func slashKeys(binds ...key.Binding) string {
	var parts []string
	for _, b := range binds {
		if len(b.Keys()) == 0 {
			continue
		}
		parts = append(parts, FormatKeys(b, "/"))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "/")
}

func pressed(msg tea.KeyMsg, b key.Binding) bool {
	return key.Matches(msg, b)
}
