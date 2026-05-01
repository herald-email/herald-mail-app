package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestLayoutIndependentBrowseKeysFollowPhysicalQWERTYShortcuts(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(keyPhysical("ξ", 'j')) // Greek layout on physical j
	updated := model.(*Model)
	if got := updated.timelineTable.Cursor(); got != 1 {
		t.Fatalf("physical j cursor=%d, want 1", got)
	}

	model, _ = updated.handleKeyMsg(keyPhysical("κ", 'k')) // Greek layout on physical k
	updated = model.(*Model)
	if got := updated.timelineTable.Cursor(); got != 0 {
		t.Fatalf("physical k cursor=%d, want 0", got)
	}

	model, _ = updated.handleKeyMsg(keyPhysical("λ", 'l')) // Greek layout on physical l
	updated = model.(*Model)
	if !updated.showLogs {
		t.Fatal("physical l should toggle logs")
	}
}

func TestJapaneseKanaFallbackBrowseKeysFollowPhysicalQWERTYShortcuts(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(keyRunes("ま")) // Japanese kana layout on physical j
	updated := model.(*Model)
	if got := updated.timelineTable.Cursor(); got != 1 {
		t.Fatalf("Japanese kana ま cursor=%d, want 1", got)
	}

	model, _ = updated.handleKeyMsg(keyRunes("の")) // Japanese kana layout on physical k
	updated = model.(*Model)
	if got := updated.timelineTable.Cursor(); got != 0 {
		t.Fatalf("Japanese kana の cursor=%d, want 0", got)
	}

	model, _ = updated.handleKeyMsg(keyRunes("り")) // Japanese kana layout on physical l
	updated = model.(*Model)
	if !updated.showLogs {
		t.Fatal("Japanese kana り should toggle logs")
	}
}

func TestShortcutKeyPrefersBaseCodeAndPreservesModifiersAndShift(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{name: "basecode lowercase", msg: keyPhysical("ξ", 'j'), want: "j"},
		{name: "basecode uppercase", msg: tea.KeyPressMsg{Text: "Ж", Code: 'ж', BaseCode: 'c', Mod: tea.ModShift}, want: "C"},
		{name: "basecode alt modifier", msg: tea.KeyPressMsg{Text: "λ", Code: 'λ', BaseCode: 'l', Mod: tea.ModAlt}, want: "alt+l"},
		{name: "basecode shifted punctuation", msg: tea.KeyPressMsg{Text: "؟", Code: '؟', BaseCode: '/', Mod: tea.ModShift}, want: "?"},
		{name: "fallback lowercase", msg: keyRunes("о"), want: "j"},
		{name: "fallback uppercase", msg: keyRunes("С"), want: "C"},
		{name: "fallback alt modifier", msg: altKey('д'), want: "alt+l"},
		{name: "fallback ukrainian layout", msg: keyRunes("ї"), want: "]"},
		{name: "fallback physical slash", msg: keyRunes("."), want: "/"},
		{name: "fallback physical question mark", msg: keyRunes(","), want: "?"},
		{name: "fallback japanese kana j", msg: keyRunes("ま"), want: "j"},
		{name: "fallback japanese kana k", msg: keyRunes("の"), want: "k"},
		{name: "fallback japanese kana l", msg: keyRunes("り"), want: "l"},
		{name: "fallback japanese kana c", msg: keyRunes("そ"), want: "c"},
		{name: "fallback japanese kana slash", msg: keyRunes("め"), want: "/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortcutKey(tc.msg); got != tc.want {
				t.Fatalf("shortcutKey=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestCyrillicPunctuationOpensTimelineSearchAndPreservesQueryText(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(keyRunes(".")) // Russian layout physical /
	updated := model.(*Model)
	if !updated.timeline.searchMode {
		t.Fatal("Cyrillic-layout physical slash should open Timeline search")
	}

	model, _ = updated.handleKeyMsg(keyRunes("привет"))
	updated = model.(*Model)
	if got := updated.timeline.searchInput.Value(); got != "привет" {
		t.Fatalf("search input value=%q, want Cyrillic query preserved", got)
	}
}

func TestCyrillicUppercaseShortcutOpensTimelineCompose(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)

	model, cmd := m.handleKeyMsg(keyRunes("С")) // Russian layout physical Shift+C
	updated := model.(*Model)
	if cmd != nil {
		t.Fatalf("expected Cyrillic С to open blank Compose synchronously, got command %T", cmd)
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab=%d, want Compose", updated.activeTab)
	}
}

func TestCyrillicQuestionMarkAliasOpensHelpInBrowseContext(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(keyRunes(",")) // Russian layout physical Shift+/
	updated := model.(*Model)
	if !updated.showHelp {
		t.Fatal("Cyrillic-layout physical question mark should open shortcut help in browse contexts")
	}
}

func TestCyrillicContactsBrowseKeysDoNotRequireLatinLayout(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabContacts
	m.contactsList = []models.ContactData{
		{Email: "alice@example.com", DisplayName: "Alice"},
		{Email: "boris@example.com", DisplayName: "Boris"},
	}
	m.contactsFiltered = m.contactsList
	m.contactsIdx = 0
	m.contactFocusPanel = 0

	model, _ := m.handleKeyMsg(keyRunes("о")) // Russian layout physical j
	updated := model.(*Model)
	if got := updated.contactsIdx; got != 1 {
		t.Fatalf("Cyrillic о contactsIdx=%d, want 1", got)
	}

	model, _ = updated.handleKeyMsg(keyRunes("л")) // Russian layout physical k
	updated = model.(*Model)
	if got := updated.contactsIdx; got != 0 {
		t.Fatalf("Cyrillic л contactsIdx=%d, want 0", got)
	}
}

func TestCyrillicTextInputStillTypesNativeCharacters(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeBody.Focus()

	model, cmd := m.handleKeyMsg(keyRunes("сообщение"))
	updated := model.(*Model)
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("Cyrillic text in Compose should not trigger global quit")
		}
	}
	if got := updated.composeBody.Value(); got != "сообщение" {
		t.Fatalf("compose body value=%q, want Cyrillic text preserved", got)
	}
}

func TestJapaneseKanaTextInputStillTypesNativeCharacters(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeBody.Focus()

	model, cmd := m.handleKeyMsg(keyRunes("まのり"))
	updated := model.(*Model)
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("Japanese kana text in Compose should not trigger global quit")
		}
	}
	if got := updated.composeBody.Value(); got != "まのり" {
		t.Fatalf("compose body value=%q, want Japanese kana text preserved", got)
	}
}

func TestCyrillicPunctuationAliasDoesNotStealComposeComma(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeBody.Focus()

	model, _ := m.handleKeyMsg(keyRunes(","))
	updated := model.(*Model)
	if updated.showHelp {
		t.Fatal("comma in Compose should type into the body, not open shortcut help")
	}
	if got := updated.composeBody.Value(); got != "," {
		t.Fatalf("compose body value=%q, want comma preserved", got)
	}
}
