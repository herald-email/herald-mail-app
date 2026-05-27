package app

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func isUnsetColor(c color.Color) bool {
	return c == nil || reflect.DeepEqual(c, lipgloss.NoColor{})
}

func TestDefaultThemeUsesAdaptiveTerminalRoles(t *testing.T) {
	if defaultTheme.Name != "inherited" {
		t.Fatalf("default theme should be inherited, got %q", defaultTheme.Name)
	}

	if defaultTheme.Text.Primary.Foreground != nil {
		t.Fatalf("primary text should inherit the terminal foreground, got %#v", defaultTheme.Text.Primary.Foreground)
	}
	if defaultTheme.Text.Primary.Background != nil {
		t.Fatalf("primary text should not force a background, got %#v", defaultTheme.Text.Primary.Background)
	}
	if !defaultTheme.Focus.SelectionActive.Reverse || !defaultTheme.Focus.SelectionActive.Bold {
		t.Fatalf("active selections should use reverse-video plus bold for terminal theme compatibility")
	}
	if defaultTheme.Focus.SelectionActive.Background != nil {
		t.Fatalf("active adaptive selection should not force a background, got %#v", defaultTheme.Focus.SelectionActive.Background)
	}
	if defaultTheme.Metadata.Subject.Foreground != nil {
		t.Fatalf("ordinary preview subjects should inherit primary text instead of warning color, got %#v", defaultTheme.Metadata.Subject.Foreground)
	}
	if !reflect.DeepEqual(defaultTheme.Severity.Destructive.Background, lipgloss.Color("1")) {
		t.Fatalf("destructive prompts should use ANSI red background, got %#v", defaultTheme.Severity.Destructive.Background)
	}
}

func TestAdaptiveChromeInactiveTabsAndHintsUseDefaultForeground(t *testing.T) {
	if defaultTheme.Chrome.TabInactive.Foreground != nil {
		t.Fatalf("inactive tabs should inherit terminal foreground, got %#v", defaultTheme.Chrome.TabInactive.Foreground)
	}
	if defaultTheme.Chrome.TabInactive.Background != nil {
		t.Fatalf("inactive tabs should not force a background, got %#v", defaultTheme.Chrome.TabInactive.Background)
	}
	if defaultTheme.Chrome.TabInactive.Faint {
		t.Fatalf("inactive tabs should not use faint styling")
	}
	if defaultTheme.Chrome.HintBar.Foreground != nil {
		t.Fatalf("key hints should inherit terminal foreground, got %#v", defaultTheme.Chrome.HintBar.Foreground)
	}
	if defaultTheme.Chrome.HintBar.Background != nil {
		t.Fatalf("key hints should not force a background, got %#v", defaultTheme.Chrome.HintBar.Background)
	}
	if defaultTheme.Chrome.HintBar.Faint {
		t.Fatalf("key hints should not use faint styling")
	}
}

func TestAdaptiveLowContrastTextRolesUseDefaultForeground(t *testing.T) {
	roles := map[string]ThemeStyle{
		"text muted":     defaultTheme.Text.Muted,
		"text dim":       defaultTheme.Text.Dim,
		"text disabled":  defaultTheme.Text.Disabled,
		"metadata label": defaultTheme.Metadata.Label,
		"metadata date":  defaultTheme.Metadata.Date,
	}
	for name, role := range roles {
		if role.Foreground != nil {
			t.Fatalf("%s should inherit terminal foreground, got %#v", name, role.Foreground)
		}
		if role.Faint {
			t.Fatalf("%s should not use faint styling", name)
		}
	}
}

func TestHeraldHuhThemeDimsFormHelp(t *testing.T) {
	styles := heraldHuhTheme(true)
	helpStyles := map[string]lipgloss.Style{
		"ellipsis":        styles.Help.Ellipsis,
		"short key":       styles.Help.ShortKey,
		"short desc":      styles.Help.ShortDesc,
		"short separator": styles.Help.ShortSeparator,
		"full key":        styles.Help.FullKey,
		"full desc":       styles.Help.FullDesc,
		"full separator":  styles.Help.FullSeparator,
	}
	for name, style := range helpStyles {
		if !reflect.DeepEqual(style.GetForeground(), lipgloss.Color("8")) {
			t.Fatalf("%s help should use dim gray, got %#v", name, style.GetForeground())
		}
		if !style.GetFaint() {
			t.Fatalf("%s help should use faint styling", name)
		}
	}
}

func TestLegacyDarkThemeKeepsCurrentXtermPalette(t *testing.T) {
	theme := ThemeByName("legacy-dark")
	if theme.Name != "herald-dark" {
		t.Fatalf("expected legacy-dark alias to resolve to herald-dark, got %q", theme.Name)
	}
	if !reflect.DeepEqual(theme.Chrome.TabActive.Background, lipgloss.Color("57")) {
		t.Fatalf("legacy active tab background should keep xterm purple, got %#v", theme.Chrome.TabActive.Background)
	}
	if !reflect.DeepEqual(theme.Focus.PanelBorderFocused.Foreground, lipgloss.Color("39")) {
		t.Fatalf("legacy focused border should keep xterm cyan, got %#v", theme.Focus.PanelBorderFocused.Foreground)
	}
	if !reflect.DeepEqual(theme.Focus.SelectionActive.Background, lipgloss.Color("57")) {
		t.Fatalf("legacy active selection should keep xterm purple, got %#v", theme.Focus.SelectionActive.Background)
	}
	if theme.Focus.SelectionActive.Reverse {
		t.Fatalf("legacy active selection should not use reverse-video")
	}
}

func TestThemeBuildersExposeSemanticStyles(t *testing.T) {
	activeTables := defaultTheme.TableStyles(true)
	if !activeTables.Selected.GetReverse() {
		t.Fatalf("adaptive active table selection should use reverse-video")
	}
	if !activeTables.Selected.GetBold() {
		t.Fatalf("adaptive active table selection should be bold")
	}

	inactiveTables := defaultTheme.TableStyles(false)
	if !inactiveTables.Selected.GetUnderline() {
		t.Fatalf("inactive table selection should remain underlined")
	}

	header := newPreviewHeaderStyles(true)
	if !isUnsetColor(header.subj.GetForeground()) {
		t.Fatalf("preview subject should inherit primary text, got %#v", header.subj.GetForeground())
	}
	if !header.subj.GetBold() {
		t.Fatalf("preview subject should remain bold")
	}
}

func TestCentralizedSurfaceRolesPreserveExistingHues(t *testing.T) {
	roleChecks := map[string]struct {
		got  color.Color
		want color.Color
	}{
		"compact overlay border": {got: defaultTheme.Overlay.CompactBorder.Foreground, want: lipgloss.Color("62")},
		"demo key foreground":    {got: defaultTheme.Overlay.DemoKeyBadge.Foreground, want: lipgloss.Color("230")},
		"demo key background":    {got: defaultTheme.Overlay.DemoKeyBadge.Background, want: lipgloss.Color("238")},
		"setup title":            {got: defaultTheme.Setup.Title.Foreground, want: lipgloss.Color("205")},
		"setup spinner":          {got: defaultTheme.Setup.Spinner.Foreground, want: lipgloss.Color("205")},
		"compose accent":         {got: defaultTheme.Compose.Accent.Foreground, want: lipgloss.Color("86")},
		"compose attachment":     {got: defaultTheme.Compose.Attachment.Foreground, want: lipgloss.Color("111")},
		"diff delete foreground": {got: defaultTheme.Diff.Delete.Foreground, want: lipgloss.Color("196")},
		"diff delete background": {got: defaultTheme.Diff.Delete.Background, want: lipgloss.Color("52")},
		"contacts keyword":       {got: defaultTheme.Contacts.KeywordSearch.Foreground, want: lipgloss.Color("33")},
		"rules selected bg":      {got: defaultTheme.Rules.Selected.Background, want: lipgloss.Color("57")},
	}
	for name, check := range roleChecks {
		if !reflect.DeepEqual(check.got, check.want) {
			t.Fatalf("%s role should preserve existing hue %#v, got %#v", name, check.want, check.got)
		}
	}
	if !defaultTheme.Diff.Delete.Strikethrough {
		t.Fatalf("diff delete role should preserve strikethrough styling")
	}
}

func TestRenderStatusBarAdaptiveEnvelopeOwnsNestedFragments(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.statusMessage = "Demo data loaded"
	m.demoMode = true

	rendered := m.renderStatusBar()
	if !strings.HasPrefix(rendered, "\x1b[7m") {
		t.Fatalf("adaptive status bar should start with reverse-video envelope, got %q", rendered)
	}
	if strings.Contains(rendered, "\x1b[36mDemo data loaded") {
		t.Fatalf("status message should not reset the adaptive status bar envelope, got %q", rendered)
	}
}
