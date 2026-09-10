package engine

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ESC = "\x1b"
	BEL = "\x07"
	ST  = "\x1b\\"
)

func CSI(parts ...any) string {
	var b strings.Builder
	b.WriteString(ESC)
	b.WriteByte('[')
	for _, p := range parts {
		b.WriteString(fmt.Sprint(p))
	}
	return b.String()
}

func OSC(parts ...any) string {
	var b strings.Builder
	b.WriteString(ESC)
	b.WriteByte(']')
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(fmt.Sprint(p))
	}
	b.WriteString(BEL)
	return b.String()
}

func CursorUp(n int) string {
	if n <= 0 {
		n = 1
	}
	return CSI(n, "A")
}
func CursorDown(n int) string {
	if n <= 0 {
		n = 1
	}
	return CSI(n, "B")
}
func CursorForward(n int) string {
	if n <= 0 {
		n = 1
	}
	return CSI(n, "C")
}
func CursorBack(n int) string {
	if n <= 0 {
		n = 1
	}
	return CSI(n, "D")
}
func CursorTo(col int) string            { return CSI(max(1, col+1), "G") }
func CursorPosition(row, col int) string { return CSI(max(1, row+1), ";", max(1, col+1), "H") }
func CursorMove(x, y int) string {
	var b strings.Builder
	if y < 0 {
		b.WriteString(CursorUp(-y))
	} else if y > 0 {
		b.WriteString(CursorDown(y))
	}
	if x < 0 {
		b.WriteString(CursorBack(-x))
	} else if x > 0 {
		b.WriteString(CursorForward(x))
	}
	return b.String()
}
func ScrollUp(n int) string {
	if n <= 0 {
		n = 1
	}
	return CSI(n, "S")
}
func ScrollDown(n int) string {
	if n <= 0 {
		n = 1
	}
	return CSI(n, "T")
}
func SetScrollRegion(top, bottom int) string { return CSI(top+1, ";", bottom+1, "r") }

const (
	CursorHome             = "\x1b[H"
	CursorSave             = "\x1b[s"
	CursorRestore          = "\x1b[u"
	EraseLine              = "\x1b[2K"
	EraseScreen            = "\x1b[2J"
	EraseScrollback        = "\x1b[3J"
	ResetScrollRegion      = "\x1b[r"
	PasteStart             = "\x1b[200~"
	PasteEnd               = "\x1b[201~"
	FocusIn                = "\x1b[I"
	FocusOut               = "\x1b[O"
	BeginSyncUpdate        = "\x1b[?2026h"
	EndSyncUpdate          = "\x1b[?2026l"
	EnableBracketPaste     = "\x1b[?2004h"
	DisableBracketPaste    = "\x1b[?2004l"
	EnableFocusEvents      = "\x1b[?1004h"
	DisableFocusEvents     = "\x1b[?1004l"
	ShowCursor             = "\x1b[?25h"
	HideCursor             = "\x1b[?25l"
	EnterAltScreen         = "\x1b[?1049h"
	ExitAltScreen          = "\x1b[?1049l"
	EnableMouseTracking    = "\x1b[?1000h\x1b[?1002h\x1b[?1006h"
	DisableMouseTracking   = "\x1b[?1006l\x1b[?1002l\x1b[?1000l"
	EnableKittyKeyboard    = "\x1b[>1u"
	DisableKittyKeyboard   = "\x1b[<u"
	EnableModifyOtherKeys  = "\x1b[>4;2m"
	DisableModifyOtherKeys = "\x1b[>4m"
)

func EraseToEndOfLine() string     { return CSI("K") }
func EraseToStartOfLine() string   { return CSI(1, "K") }
func EraseToEndOfScreen() string   { return CSI("J") }
func EraseToStartOfScreen() string { return CSI(1, "J") }

func Hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return OSC(8, "", url) + text + OSC(8, "", "")
}

// StyledGrapheme is the render-ready result of parsing ANSI text.
type StyledGrapheme struct {
	Value     string
	Width     int
	Style     TextStyle
	Hyperlink string
}

var namedANSI = map[int]int{
	30: 0, 31: 1, 32: 2, 33: 3, 34: 4, 35: 5, 36: 6, 37: 7,
	90: 8, 91: 9, 92: 10, 93: 11, 94: 12, 95: 13, 96: 14, 97: 15,
}

func applySGR(params string, style *TextStyle) {
	if params == "" {
		params = "0"
	}
	parts := strings.Split(params, ";")
	vals := make([]int, len(parts))
	for i, p := range parts {
		if p == "" {
			vals[i] = 0
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			n = -1
		}
		vals[i] = n
	}
	for i := 0; i < len(vals); i++ {
		v := vals[i]
		switch v {
		case 0:
			*style = TextStyle{}
		case 1:
			style.Bold = true
		case 2:
			style.Dim = true
		case 3:
			style.Italic = true
		case 4:
			style.Underline = true
		case 7:
			style.Inverse = true
		case 9:
			style.Strikethrough = true
		case 22:
			style.Bold, style.Dim = false, false
		case 23:
			style.Italic = false
		case 24:
			style.Underline = false
		case 27:
			style.Inverse = false
		case 29:
			style.Strikethrough = false
		case 39:
			style.Color = Color{}
		case 49:
			style.BackgroundColor = Color{}
		default:
			if idx, ok := namedANSI[v]; ok {
				style.Color = ANSIColor(idx)
			} else if v >= 40 && v <= 47 {
				style.BackgroundColor = ANSIColor(v - 40)
			} else if v >= 100 && v <= 107 {
				style.BackgroundColor = ANSIColor(v - 100 + 8)
			} else if (v == 38 || v == 48) && i+2 < len(vals) && vals[i+1] == 5 {
				c := ANSI256(vals[i+2])
				if v == 38 {
					style.Color = c
				} else {
					style.BackgroundColor = c
				}
				i += 2
			} else if (v == 38 || v == 48) && i+4 < len(vals) && vals[i+1] == 2 {
				r, g, b := vals[i+2], vals[i+3], vals[i+4]
				if r >= 0 && r <= 255 && g >= 0 && g <= 255 && b >= 0 && b <= 255 {
					c := RGB(uint8(r), uint8(g), uint8(b))
					if v == 38 {
						style.Color = c
					} else {
						style.BackgroundColor = c
					}
				}
				i += 4
			}
		}
	}
}

func mergeStyle(base, overlay TextStyle) TextStyle {
	out := base
	if overlay.Color.Kind != ColorUnset {
		out.Color = overlay.Color
	}
	if overlay.BackgroundColor.Kind != ColorUnset {
		out.BackgroundColor = overlay.BackgroundColor
	}
	out.Dim = out.Dim || overlay.Dim
	out.Bold = out.Bold || overlay.Bold
	out.Italic = out.Italic || overlay.Italic
	out.Underline = out.Underline || overlay.Underline
	out.Strikethrough = out.Strikethrough || overlay.Strikethrough
	out.Inverse = out.Inverse || overlay.Inverse
	return out
}

// ParseANSI handles SGR styling and OSC-8 hyperlinks. Unknown terminal
// control sequences are consumed instead of becoming visible text.
func ParseANSI(input string, base TextStyle) []StyledGrapheme {
	var out []StyledGrapheme
	style := base
	href := ""
	plainStart := 0
	flush := func(end int) {
		if end <= plainStart {
			return
		}
		for _, g := range Graphemes(input[plainStart:end]) {
			if g.Text == "\r" {
				continue
			}
			w := g.Width
			out = append(out, StyledGrapheme{Value: g.Text, Width: w, Style: style, Hyperlink: href})
		}
	}
	for i := 0; i < len(input); {
		if input[i] != 0x1b {
			i++
			continue
		}
		flush(i)
		if i+1 >= len(input) {
			plainStart = i + 1
			break
		}
		switch input[i+1] {
		case '[':
			j := i + 2
			for j < len(input) && (input[j] < 0x40 || input[j] > 0x7e) {
				j++
			}
			if j >= len(input) {
				i = len(input)
				plainStart = i
				break
			}
			if input[j] == 'm' {
				applySGR(input[i+2:j], &style)
			}
			i = j + 1
			plainStart = i
		case ']':
			j := i + 2
			for j < len(input) && input[j] != 0x07 && !(input[j] == 0x1b && j+1 < len(input) && input[j+1] == '\\') {
				j++
			}
			content := input[i+2 : j]
			if strings.HasPrefix(content, "8;") {
				parts := strings.SplitN(content, ";", 3)
				if len(parts) == 3 {
					href = parts[2]
				}
			}
			if j < len(input) && input[j] == 0x1b {
				j += 2
			} else if j < len(input) {
				j++
			}
			i = j
			plainStart = i
		case 'P', '_', '^': // DCS/APC/PM until ST
			j := strings.Index(input[i+2:], ST)
			if j < 0 {
				i = len(input)
			} else {
				i += 2 + j + len(ST)
			}
			plainStart = i
		default:
			i += 2
			plainStart = i
		}
	}
	flush(len(input))
	return out
}

func colorSGR(c Color, foreground bool) string {
	if c.Kind == ColorUnset {
		return ""
	}
	prefix := 38
	if !foreground {
		prefix = 48
	}
	switch c.Kind {
	case ColorANSI:
		idx := c.ANSI
		if idx < 8 {
			if foreground {
				return strconv.Itoa(30 + idx)
			}
			return strconv.Itoa(40 + idx)
		}
		if idx < 16 {
			if foreground {
				return strconv.Itoa(90 + idx - 8)
			}
			return strconv.Itoa(100 + idx - 8)
		}
		return fmt.Sprintf("%d;5;%d", prefix, idx)
	case ColorANSI256:
		return fmt.Sprintf("%d;5;%d", prefix, c.ANSI)
	case ColorRGB:
		return fmt.Sprintf("%d;2;%d;%d;%d", prefix, c.R, c.G, c.B)
	}
	return ""
}

func StyleSequence(s TextStyle) string {
	parts := []string{"0"}
	if s.Bold {
		parts = append(parts, "1")
	}
	if s.Dim {
		parts = append(parts, "2")
	}
	if s.Italic {
		parts = append(parts, "3")
	}
	if s.Underline {
		parts = append(parts, "4")
	}
	if s.Inverse {
		parts = append(parts, "7")
	}
	if s.Strikethrough {
		parts = append(parts, "9")
	}
	if c := colorSGR(s.Color, true); c != "" {
		parts = append(parts, c)
	}
	if c := colorSGR(s.BackgroundColor, false); c != "" {
		parts = append(parts, c)
	}
	return CSI(strings.Join(parts, ";"), "m")
}
