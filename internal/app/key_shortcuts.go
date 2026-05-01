package app

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

var shiftedPhysicalShortcutKeys = map[rune]rune{
	'`':  '~',
	'1':  '!',
	'2':  '@',
	'3':  '#',
	'4':  '$',
	'5':  '%',
	'6':  '^',
	'7':  '&',
	'8':  '*',
	'9':  '(',
	'0':  ')',
	'-':  '_',
	'=':  '+',
	'[':  '{',
	']':  '}',
	'\\': '|',
	';':  ':',
	'\'': '"',
	',':  '<',
	'.':  '>',
	'/':  '?',
}

// Some terminals do not report BaseCode. Keep printable-layout fallbacks for
// those sessions, but prefer Bubble Tea v2 physical-key data whenever present.
// This only helps layouts that emit a committed character per keypress. IME
// pre-edit text, such as Japanese romaji-to-hiragana composition, is held by
// the terminal/input method until committed and must rely on BaseCode support.
var printablePhysicalShortcutAliases = map[rune]rune{
	// Cyrillic ЙЦУКЕН-family layouts.
	'й': 'q', 'Й': 'Q',
	'ц': 'w', 'Ц': 'W',
	'у': 'e', 'У': 'E',
	'к': 'r', 'К': 'R',
	'е': 't', 'Е': 'T',
	'н': 'y', 'Н': 'Y',
	'г': 'u', 'Г': 'U',
	'ш': 'i', 'Ш': 'I',
	'щ': 'o', 'Щ': 'O',
	'з': 'p', 'З': 'P',
	'х': '[', 'Х': '{',
	'ъ': ']', 'Ъ': '}',
	'ї': ']', 'Ї': '}',
	'ф': 'a', 'Ф': 'A',
	'ы': 's', 'Ы': 'S',
	'і': 's', 'І': 'S',
	'в': 'd', 'В': 'D',
	'а': 'f', 'А': 'F',
	'п': 'g', 'П': 'G',
	'р': 'h', 'Р': 'H',
	'о': 'j', 'О': 'J',
	'л': 'k', 'Л': 'K',
	'д': 'l', 'Д': 'L',
	'ж': ';', 'Ж': ':',
	'э': '\'', 'Э': '"',
	'є': '\'', 'Є': '"',
	'я': 'z', 'Я': 'Z',
	'ч': 'x', 'Ч': 'X',
	'с': 'c', 'С': 'C',
	'м': 'v', 'М': 'V',
	'и': 'b', 'И': 'B',
	'т': 'n', 'Т': 'N',
	'ь': 'm', 'Ь': 'M',
	'б': ',', 'Б': '<',
	'ю': '.', 'Ю': '>',
	'.': '/', ',': '?',

	// Japanese direct-kana layout. This does not cover romaji IME pre-edit,
	// which often emits no key message until a kana sequence is committed.
	'た': 'q', 'タ': 'q',
	'て': 'w', 'テ': 'w',
	'い': 'e', 'イ': 'e',
	'す': 'r', 'ス': 'r',
	'か': 't', 'カ': 't',
	'ん': 'y', 'ン': 'y',
	'な': 'u', 'ナ': 'u',
	'に': 'i', 'ニ': 'i',
	'ら': 'o', 'ラ': 'o',
	'せ': 'p', 'セ': 'p',
	'ち': 'a', 'チ': 'a',
	'と': 's', 'ト': 's',
	'し': 'd', 'シ': 'd',
	'は': 'f', 'ハ': 'f',
	'き': 'g', 'キ': 'g',
	'く': 'h', 'ク': 'h',
	'ま': 'j', 'マ': 'j',
	'の': 'k', 'ノ': 'k',
	'り': 'l', 'リ': 'l',
	'れ': ';', 'レ': ';',
	'け': '\'', 'ケ': '\'',
	'つ': 'z', 'ツ': 'z',
	'っ': 'z', 'ッ': 'z',
	'さ': 'x', 'サ': 'x',
	'そ': 'c', 'ソ': 'c',
	'ひ': 'v', 'ヒ': 'v',
	'こ': 'b', 'コ': 'b',
	'み': 'n', 'ミ': 'n',
	'も': 'm', 'モ': 'm',
	'ね': ',', 'ネ': ',',
	'る': '.', 'ル': '.',
	'め': '/', 'メ': '/',
	'ほ': '-', 'ホ': '-',
	'へ': '=', 'ヘ': '=',
	'む': ']', 'ム': ']',
}

func shortcutKey(msg tea.KeyPressMsg) string {
	if key, ok := physicalShortcutKey(msg); ok {
		return key
	}
	key := msg.Key()
	base, addModifiers := fallbackShortcutBase(key, msg.String())
	base = normalizeShortcutKeyString(base)
	if !addModifiers {
		return base
	}
	parts := shortcutModifierParts(key.Mod)
	parts = append(parts, base)
	return strings.Join(parts, "+")
}

func fallbackShortcutBase(key tea.Key, rendered string) (string, bool) {
	if key.Text != "" {
		return key.Text, true
	}
	if key.Code >= 0x20 && key.Code <= 0x7e {
		return string(key.Code), true
	}
	return rendered, !strings.Contains(rendered, "+")
}

func physicalShortcutKey(msg tea.KeyPressMsg) (string, bool) {
	key := msg.Key()
	if key.BaseCode == 0 {
		return "", false
	}
	code := key.BaseCode
	if key.Mod.Contains(tea.ModShift) {
		if shifted, ok := shiftedPhysicalShortcutKeys[code]; ok {
			code = shifted
		} else {
			code = []rune(strings.ToUpper(string(code)))[0]
		}
	}
	base := string(code)
	parts := shortcutModifierParts(key.Mod)
	parts = append(parts, base)
	return strings.Join(parts, "+"), true
}

func shortcutModifierParts(mod tea.KeyMod) []string {
	var parts []string
	if mod.Contains(tea.ModCtrl) {
		parts = append(parts, "ctrl")
	}
	if mod.Contains(tea.ModAlt) {
		parts = append(parts, "alt")
	}
	if mod.Contains(tea.ModMeta) {
		parts = append(parts, "meta")
	}
	if mod.Contains(tea.ModHyper) {
		parts = append(parts, "hyper")
	}
	if mod.Contains(tea.ModSuper) {
		parts = append(parts, "super")
	}
	return parts
}

func normalizeShortcutKeyString(raw string) string {
	parts := strings.Split(raw, "+")
	if len(parts) == 0 {
		return raw
	}
	last := parts[len(parts)-1]
	if mapped, ok := printablePhysicalShortcutAlias(last); ok {
		parts[len(parts)-1] = mapped
		return strings.Join(parts, "+")
	}
	return raw
}

func printablePhysicalShortcutAlias(value string) (string, bool) {
	r, size := utf8.DecodeRuneInString(value)
	if r == utf8.RuneError || size != len(value) {
		return "", false
	}
	mapped, ok := printablePhysicalShortcutAliases[r]
	if !ok {
		return "", false
	}
	return string(mapped), true
}
