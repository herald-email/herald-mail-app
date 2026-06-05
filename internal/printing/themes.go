package printing

type Theme string

const (
	ThemeOriginal   Theme = "original"
	ThemeSwiss      Theme = "swiss"
	ThemeGitHub     Theme = "github"
	ThemeManuscript Theme = "manuscript"
	ThemeAcademic   Theme = "academic"
)

type ThemeOption struct {
	Key  string
	ID   Theme
	Name string
}

var markdownThemeOptions = []ThemeOption{
	{Key: "2", ID: ThemeSwiss, Name: "Markdown Swiss"},
	{Key: "3", ID: ThemeGitHub, Name: "Markdown GitHub"},
	{Key: "4", ID: ThemeManuscript, Name: "Markdown Manuscript"},
	{Key: "5", ID: ThemeAcademic, Name: "Markdown Academic"},
}

func MarkdownThemes() []ThemeOption {
	out := make([]ThemeOption, len(markdownThemeOptions))
	copy(out, markdownThemeOptions)
	return out
}

func MarkdownThemeByKey(key string) (ThemeOption, bool) {
	for _, option := range markdownThemeOptions {
		if option.Key == key {
			return option, true
		}
	}
	return ThemeOption{}, false
}

func normalizeTheme(theme Theme) Theme {
	switch theme {
	case ThemeSwiss, ThemeGitHub, ThemeManuscript, ThemeAcademic:
		return theme
	default:
		return ThemeSwiss
	}
}

func printThemeForRequest(req Request) Theme {
	if normalizeMode(req.Mode) != ModeRenderedMarkdown {
		return ThemeOriginal
	}
	return normalizeTheme(req.Theme)
}
