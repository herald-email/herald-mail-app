package app

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type themeColorPickerMode int

const (
	themeColorPickerModeXterm themeColorPickerMode = iota
	themeColorPickerModeRGB

	themePickerVisibleRows = 6
)

type themeColorPickerField struct {
	key       string
	title     string
	accessor  huh.Accessor[string]
	focused   bool
	width     int
	height    int
	mode      themeColorPickerMode
	xterm     int
	rgb       [3]int
	channel   int
	display   string
	theme     huh.Theme
	hasDarkBg bool

	prev    key.Binding
	next    key.Binding
	submit  key.Binding
	up      key.Binding
	down    key.Binding
	left    key.Binding
	right   key.Binding
	modeKey key.Binding
	inherit key.Binding
}

func newThemeColorPickerField(title string, value *string) *themeColorPickerField {
	f := &themeColorPickerField{
		title:    title,
		accessor: huh.NewPointerAccessor(value),
		width:    40,
		height:   8,
		xterm:    25,
		prev:     key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "back")),
		next:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next")),
		submit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
		up:       key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("up/k", "up")),
		down:     key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("down/j", "down")),
		left:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("left/h", "left")),
		right:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("right/l", "right")),
		modeKey:  key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mode")),
		inherit:  key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "inherit")),
	}
	f.syncFromValue()
	return f
}

func (f *themeColorPickerField) Key(key string) *themeColorPickerField {
	f.key = key
	return f
}

func (f *themeColorPickerField) Init() tea.Cmd { return nil }

func (f *themeColorPickerField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		f.hasDarkBg = msg.IsDark()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, f.prev):
			return f, huh.PrevField
		case key.Matches(msg, f.next, f.submit):
			return f, huh.NextField
		case key.Matches(msg, f.modeKey):
			if f.mode == themeColorPickerModeXterm {
				f.mode = themeColorPickerModeRGB
			} else {
				f.mode = themeColorPickerModeXterm
				f.xterm = nearestXterm256(f.rgb)
			}
			f.writeValue()
		case key.Matches(msg, f.inherit):
			f.display = "inherit"
			f.accessor.Set("inherit")
		case key.Matches(msg, f.up):
			f.move(-16, 1)
		case key.Matches(msg, f.down):
			f.move(16, -1)
		case key.Matches(msg, f.left):
			if f.mode == themeColorPickerModeRGB {
				f.channel = max(f.channel-1, 0)
			} else {
				f.move(-1, 0)
			}
		case key.Matches(msg, f.right):
			if f.mode == themeColorPickerModeRGB {
				f.channel = min(f.channel+1, 2)
			} else {
				f.move(1, 0)
			}
		}
	}
	return f, nil
}

func (f *themeColorPickerField) move(xtermDelta, rgbDelta int) {
	if f.mode == themeColorPickerModeRGB {
		f.rgb[f.channel] = clampThemePicker(f.rgb[f.channel]+rgbDelta, 0, 255)
	} else {
		f.xterm = clampThemePicker(f.xterm+xtermDelta, 0, 255)
	}
	f.writeValue()
}

func (f *themeColorPickerField) syncFromValue() {
	value := strings.TrimSpace(f.accessor.Get())
	if value == "" {
		value = "inherit"
	}
	f.display = value
	if strings.EqualFold(value, "inherit") {
		return
	}
	if rgb, ok := parseThemePickerHex(value); ok {
		f.rgb = rgb
		f.xterm = nearestXterm256(rgb)
		return
	}
	if n, ok := parseThemePickerIndex(value); ok {
		f.xterm = n
		f.rgb = xterm256ToRGB(n)
	}
}

func (f *themeColorPickerField) writeValue() {
	if f.mode == themeColorPickerModeRGB {
		f.display = fmt.Sprintf("#%02x%02x%02x", f.rgb[0], f.rgb[1], f.rgb[2])
		f.accessor.Set(f.display)
		return
	}
	f.display = fmt.Sprintf("xterm:%d", f.xterm)
	f.accessor.Set(f.display)
}

func (f *themeColorPickerField) View() string {
	if !f.focused {
		f.syncFromValue()
	}
	styles := f.activeStyles()
	lines := []string{styles.Title.Render(f.title)}
	value := strings.TrimSpace(f.display)
	if value == "" {
		value = "inherit"
	}
	lines = append(lines, themeColorSwatch("value", value))
	if f.focused {
		if f.mode == themeColorPickerModeRGB {
			lines = append(lines, f.rgbView()...)
		} else {
			lines = append(lines, f.xtermGridView()...)
		}
		lines = append(lines, styles.Description.Render("m mode  i inherit  arrows/hjkl adjust  tab next"))
	} else {
		lines = append(lines, styles.Description.Render("Focus to use xterm grid or RGB picker."))
	}
	return styles.Card.Width(f.width).Render(strings.Join(lines, "\n"))
}

func (f *themeColorPickerField) xtermGridView() []string {
	selectedRow := f.xterm / 16
	startRow := clampThemePicker(selectedRow-2, 0, 16-themePickerVisibleRows)
	rows := make([]string, 0, themePickerVisibleRows+1)
	for row := startRow; row < startRow+themePickerVisibleRows; row++ {
		var b strings.Builder
		for col := 0; col < 16; col++ {
			idx := row*16 + col
			b.WriteString(f.xtermGridCell(idx))
		}
		rows = append(rows, b.String())
	}
	rows = append(rows, fmt.Sprintf("xterm:%d  rows %d-%d/15", f.xterm, startRow, startRow+themePickerVisibleRows-1))
	return rows
}

func (f *themeColorPickerField) xtermGridCell(idx int) string {
	style := lipgloss.NewStyle().Background(themeXtermColor(idx))
	if idx == f.xterm {
		return style.Foreground(themeXtermContrastColor(idx)).Bold(true).Render("[]")
	}
	return style.Render("  ")
}

func (f *themeColorPickerField) rgbView() []string {
	names := []string{"R", "G", "B"}
	lines := make([]string, 0, 4)
	for i, name := range names {
		label := fmt.Sprintf("%s %3d", name, f.rgb[i])
		if i == f.channel {
			label = "> " + label
		} else {
			label = "  " + label
		}
		lines = append(lines, label+"  "+rgbBar(f.rgb[i]))
	}
	lines = append(lines, fmt.Sprintf("#%02x%02x%02x", f.rgb[0], f.rgb[1], f.rgb[2]))
	return lines
}

func rgbBar(value int) string {
	filled := clampThemePicker(value/16, 0, 16)
	return strings.Repeat("#", filled) + strings.Repeat("-", 16-filled)
}

func (f *themeColorPickerField) Focus() tea.Cmd {
	f.focused = true
	f.syncFromValue()
	return nil
}

func (f *themeColorPickerField) Blur() tea.Cmd {
	f.focused = false
	return nil
}

func (f *themeColorPickerField) Error() error { return nil }
func (f *themeColorPickerField) Skip() bool   { return false }
func (f *themeColorPickerField) Zoom() bool   { return false }

func (f *themeColorPickerField) KeyBinds() []key.Binding {
	return []key.Binding{f.prev, f.submit, f.next, f.up, f.down, f.left, f.right, f.modeKey, f.inherit}
}

func (f *themeColorPickerField) Run() error { return huh.Run(f) }

func (f *themeColorPickerField) RunAccessible(w io.Writer, _ io.Reader) error {
	_, _ = fmt.Fprintf(w, "%s: %s\n", f.title, f.accessor.Get())
	return nil
}

func (f *themeColorPickerField) WithTheme(theme huh.Theme) huh.Field {
	if f.theme == nil {
		f.theme = theme
	}
	return f
}

func (f *themeColorPickerField) WithKeyMap(_ *huh.KeyMap) huh.Field { return f }

func (f *themeColorPickerField) WithWidth(width int) huh.Field {
	f.width = width
	return f
}

func (f *themeColorPickerField) WithHeight(height int) huh.Field {
	f.height = height
	return f
}

func (f *themeColorPickerField) WithPosition(p huh.FieldPosition) huh.Field {
	f.prev.SetEnabled(!p.IsFirst())
	f.next.SetEnabled(!p.IsLast())
	f.submit.SetEnabled(!p.IsLast())
	return f
}

func (f *themeColorPickerField) GetKey() string { return f.key }
func (f *themeColorPickerField) GetValue() any  { return f.accessor.Get() }
func (f *themeColorPickerField) activeStyles() *huh.FieldStyles {
	theme := f.theme
	if theme == nil {
		theme = huh.ThemeFunc(huh.ThemeCharm)
	}
	if f.focused {
		return &theme.Theme(f.hasDarkBg).Focused
	}
	return &theme.Theme(f.hasDarkBg).Blurred
}

func parseThemePickerIndex(value string) (int, bool) {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"xterm:", "ansi:"} {
		if strings.HasPrefix(lower, prefix) {
			n, err := strconv.Atoi(strings.TrimPrefix(lower, prefix))
			return clampThemePicker(n, 0, 255), err == nil
		}
	}
	return 0, false
}

func parseThemePickerHex(value string) ([3]int, bool) {
	var rgb [3]int
	v := strings.TrimSpace(value)
	if len(v) != 7 || !strings.HasPrefix(v, "#") {
		return rgb, false
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.ParseInt(v[1+i*2:3+i*2], 16, 0)
		if err != nil {
			return rgb, false
		}
		rgb[i] = int(n)
	}
	return rgb, true
}

func xterm256ToRGB(n int) [3]int {
	n = clampThemePicker(n, 0, 255)
	base := [16][3]int{
		{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
		{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
		{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
		{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	if n < 16 {
		return base[n]
	}
	if n >= 232 {
		level := 8 + (n-232)*10
		return [3]int{level, level, level}
	}
	levels := [6]int{0, 95, 135, 175, 215, 255}
	idx := n - 16
	return [3]int{levels[idx/36], levels[(idx/6)%6], levels[idx%6]}
}

func nearestXterm256(rgb [3]int) int {
	best := 0
	bestDistance := math.MaxFloat64
	for i := 0; i < 256; i++ {
		candidate := xterm256ToRGB(i)
		dr := float64(rgb[0] - candidate[0])
		dg := float64(rgb[1] - candidate[1])
		db := float64(rgb[2] - candidate[2])
		distance := dr*dr + dg*dg + db*db
		if distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func clampThemePicker(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
