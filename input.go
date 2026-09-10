package ink

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type InputKind uint8

const (
	InputKey InputKind = iota
	InputMouse
	InputPaste
	InputResponse
)

type ParsedInput struct {
	Kind     InputKind
	Key      Key
	Mouse    ParsedMouse
	Paste    string
	Response TerminalResponse
	Sequence string
}

type ParsedMouse struct {
	Button   int
	Action   string
	Col, Row int
}
type TerminalResponse struct {
	Type   string
	Params []int
	Value  string
}

type InputParser struct {
	buffer  string
	inPaste bool
	paste   strings.Builder
}

func NewInputParser() *InputParser    { return &InputParser{} }
func (p *InputParser) Buffer() string { return p.buffer }

var sgrMouseRE = regexp.MustCompile(`^\x1b\[<(\d+);(\d+);(\d+)([Mm])`)
var csiURE = regexp.MustCompile(`^\x1b\[(\d+)(?:;(\d+))?u`)
var modifyOtherRE = regexp.MustCompile(`^\x1b\[27;(\d+);(\d+)~`)

func (p *InputParser) Feed(data []byte) []ParsedInput {
	p.buffer += string(data)
	return p.consume(false)
}
func (p *InputParser) Flush() []ParsedInput { return p.consume(true) }

func (p *InputParser) consume(flush bool) []ParsedInput {
	var out []ParsedInput
	for len(p.buffer) > 0 {
		if p.inPaste {
			if i := strings.Index(p.buffer, PasteEnd); i >= 0 {
				p.paste.WriteString(p.buffer[:i])
				out = append(out, ParsedInput{Kind: InputPaste, Paste: p.paste.String(), Sequence: p.paste.String()})
				p.paste.Reset()
				p.inPaste = false
				p.buffer = p.buffer[i+len(PasteEnd):]
				continue
			}
			if flush {
				p.paste.WriteString(p.buffer)
				p.buffer = ""
				if p.paste.Len() > 0 {
					out = append(out, ParsedInput{Kind: InputPaste, Paste: p.paste.String()})
					p.paste.Reset()
				}
				p.inPaste = false
				break
			}
			keep := len(PasteEnd) - 1
			if len(p.buffer) > keep {
				p.paste.WriteString(p.buffer[:len(p.buffer)-keep])
				p.buffer = p.buffer[len(p.buffer)-keep:]
			}
			break
		}
		if strings.HasPrefix(p.buffer, PasteStart) {
			p.inPaste = true
			p.buffer = p.buffer[len(PasteStart):]
			continue
		}
		if p.buffer[0] != 0x1b {
			r, n := utf8.DecodeRuneInString(p.buffer)
			if r == utf8.RuneError && n == 1 && !flush {
				break
			}
			seq := p.buffer[:n]
			p.buffer = p.buffer[n:]
			out = append(out, ParsedInput{Kind: InputKey, Key: keyFromText(seq), Sequence: seq})
			continue
		}
		seq, complete := nextEscapeSequence(p.buffer)
		if !complete {
			if flush {
				seq = p.buffer
				p.buffer = ""
			} else {
				break
			}
		} else {
			p.buffer = p.buffer[len(seq):]
		}
		if seq == "" {
			break
		}
		if seq == FocusIn {
			out = append(out, ParsedInput{Kind: InputResponse, Response: TerminalResponse{Type: "focus-in"}, Sequence: seq})
			continue
		}
		if seq == FocusOut {
			out = append(out, ParsedInput{Kind: InputResponse, Response: TerminalResponse{Type: "focus-out"}, Sequence: seq})
			continue
		}
		if m := parseMouse(seq); m != nil {
			if m.Button&0x40 != 0 {
				name := "wheelup"
				if m.Button&1 != 0 {
					name = "wheeldown"
				}
				mods := mouseMods(m.Button)
				out = append(out, ParsedInput{Kind: InputKey, Key: Key{Name: name, Sequence: seq, Shift: mods.Shift, Alt: mods.Alt, Ctrl: mods.Ctrl}, Sequence: seq})
			} else {
				out = append(out, ParsedInput{Kind: InputMouse, Mouse: *m, Sequence: seq})
			}
			continue
		}
		if resp := parseResponse(seq); resp != nil {
			out = append(out, ParsedInput{Kind: InputResponse, Response: *resp, Sequence: seq})
			continue
		}
		out = append(out, ParsedInput{Kind: InputKey, Key: parseKeySequence(seq), Sequence: seq})
	}
	return out
}

func nextEscapeSequence(s string) (string, bool) {
	if len(s) < 2 {
		return "", false
	}
	if s[1] == '[' {
		for i := 2; i < len(s); i++ {
			b := s[i]
			if b >= 0x40 && b <= 0x7e {
				return s[:i+1], true
			}
		}
		return "", false
	}
	if s[1] == ']' {
		if i := strings.IndexByte(s[2:], 0x07); i >= 0 {
			return s[:2+i+1], true
		}
		if i := strings.Index(s[2:], ST); i >= 0 {
			return s[:2+i+len(ST)], true
		}
		return "", false
	}
	if s[1] == 'P' {
		if i := strings.Index(s[2:], ST); i >= 0 {
			return s[:2+i+len(ST)], true
		}
		return "", false
	}
	// ESC + UTF-8 rune (Alt/meta key) or SS3 sequence.
	if s[1] == 'O' {
		if len(s) < 3 {
			return "", false
		}
		return s[:3], true
	}
	_, n := utf8.DecodeRuneInString(s[1:])
	if n == 0 {
		return "", false
	}
	if n == 1 && s[1] >= 0x80 {
		return "", false
	}
	return s[:1+n], true
}

type modifierFlags struct{ Shift, Alt, Ctrl, Super bool }

func decodeModifier(v int) modifierFlags {
	m := v - 1
	if m < 0 {
		m = 0
	}
	return modifierFlags{Shift: m&1 != 0, Alt: m&2 != 0, Ctrl: m&4 != 0, Super: m&8 != 0}
}
func mouseMods(v int) modifierFlags {
	return modifierFlags{Shift: v&4 != 0, Alt: v&8 != 0, Ctrl: v&16 != 0}
}
func keycodeName(cp int) string {
	switch cp {
	case 9:
		return "tab"
	case 13:
		return "return"
	case 27:
		return "escape"
	case 32:
		return "space"
	case 127:
		return "backspace"
	case 57399, 57400, 57401, 57402, 57403, 57404, 57405, 57406, 57407, 57408:
		return strconv.Itoa(cp - 57399)
	case 57409:
		return "."
	case 57410:
		return "/"
	case 57411:
		return "*"
	case 57412:
		return "-"
	case 57413:
		return "+"
	case 57414:
		return "return"
	case 57415:
		return "="
	}
	if cp >= 32 && cp <= 126 {
		return strings.ToLower(string(rune(cp)))
	}
	return ""
}

var legacyKeys = map[string]string{
	"\x1bOP": "f1", "\x1bOQ": "f2", "\x1bOR": "f3", "\x1bOS": "f4", "\x1b[15~": "f5", "\x1b[17~": "f6", "\x1b[18~": "f7", "\x1b[19~": "f8", "\x1b[20~": "f9", "\x1b[21~": "f10", "\x1b[23~": "f11", "\x1b[24~": "f12",
	"\x1b[A": "up", "\x1b[B": "down", "\x1b[C": "right", "\x1b[D": "left", "\x1b[F": "end", "\x1b[H": "home", "\x1bOA": "up", "\x1bOB": "down", "\x1bOC": "right", "\x1bOD": "left", "\x1bOF": "end", "\x1bOH": "home",
	"\x1b[1~": "home", "\x1b[2~": "insert", "\x1b[3~": "delete", "\x1b[4~": "end", "\x1b[5~": "pageup", "\x1b[6~": "pagedown", "\x1b[7~": "home", "\x1b[8~": "end", "\x1b[Z": "tab",
}

func keyFromText(s string) Key {
	if s == "\r" || s == "\n" {
		return Key{Name: "return", Sequence: s}
	}
	if s == "\t" {
		return Key{Name: "tab", Text: "\t", Sequence: s}
	}
	if s == "\x7f" || s == "\b" {
		return Key{Name: "backspace", Sequence: s}
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r > 0 && r < 32 {
		return Key{Name: strings.ToLower(string(r + 'a' - 1)), Ctrl: true, Sequence: s}
	}
	return Key{Name: strings.ToLower(s), Text: s, Sequence: s}
}

func parseKeySequence(s string) Key {
	if s == "\x1b" {
		return Key{Name: "escape", Sequence: s}
	}
	if m := csiURE.FindStringSubmatch(s); m != nil {
		cp, _ := strconv.Atoi(m[1])
		mod := 1
		if m[2] != "" {
			mod, _ = strconv.Atoi(m[2])
		}
		f := decodeModifier(mod)
		name := keycodeName(cp)
		text := ""
		if cp >= 32 && cp <= 0x10ffff && name != "" && len([]rune(name)) == 1 {
			text = string(rune(cp))
		}
		return Key{Name: name, Text: text, Sequence: s, Ctrl: f.Ctrl, Alt: f.Alt, Meta: f.Alt, Shift: f.Shift, Super: f.Super}
	}
	if m := modifyOtherRE.FindStringSubmatch(s); m != nil {
		mod, _ := strconv.Atoi(m[1])
		cp, _ := strconv.Atoi(m[2])
		f := decodeModifier(mod)
		return Key{Name: keycodeName(cp), Sequence: s, Ctrl: f.Ctrl, Alt: f.Alt, Meta: f.Alt, Shift: f.Shift, Super: f.Super}
	}
	if name, ok := legacyKeys[s]; ok {
		shift := s == "\x1b[Z"
		return Key{Name: name, Sequence: s, Shift: shift}
	}
	// xterm CSI 1;modifier A/B/C/D/H/F
	if strings.HasPrefix(s, "\x1b[") && len(s) > 3 {
		final := s[len(s)-1]
		body := s[2 : len(s)-1]
		parts := strings.Split(body, ";")
		if len(parts) >= 2 {
			mod, _ := strconv.Atoi(parts[len(parts)-1])
			f := decodeModifier(mod)
			names := map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end"}
			if name := names[final]; name != "" {
				return Key{Name: name, Sequence: s, Ctrl: f.Ctrl, Alt: f.Alt, Meta: f.Alt, Shift: f.Shift, Super: f.Super}
			}
		}
	}
	if strings.HasPrefix(s, "\x1b") && len(s) > 1 {
		k := keyFromText(s[1:])
		k.Alt = true
		k.Meta = true
		k.Sequence = s
		return k
	}
	return Key{Sequence: s}
}

func parseMouse(s string) *ParsedMouse {
	m := sgrMouseRE.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	btn, _ := strconv.Atoi(m[1])
	col, _ := strconv.Atoi(m[2])
	row, _ := strconv.Atoi(m[3])
	a := "press"
	if m[4] == "m" {
		a = "release"
	}
	return &ParsedMouse{Button: btn, Action: a, Col: col, Row: row}
}
func parseResponse(s string) *TerminalResponse {
	if strings.HasPrefix(s, "\x1b[?") && strings.HasSuffix(s, "$y") {
		body := strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[?"), "$y")
		return &TerminalResponse{Type: "decrpm", Params: parseInts(body)}
	}
	if strings.HasPrefix(s, "\x1b[?") && strings.HasSuffix(s, "c") {
		return &TerminalResponse{Type: "da1", Params: parseInts(strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[?"), "c"))}
	}
	if strings.HasPrefix(s, "\x1b[>") && strings.HasSuffix(s, "c") {
		return &TerminalResponse{Type: "da2", Params: parseInts(strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[>"), "c"))}
	}
	if strings.HasPrefix(s, "\x1b]") {
		return &TerminalResponse{Type: "osc", Value: s}
	}
	if strings.HasPrefix(s, "\x1bP>|") {
		return &TerminalResponse{Type: "xtversion", Value: strings.TrimSuffix(strings.TrimPrefix(s, "\x1bP>|"), ST)}
	}
	return nil
}
func parseInts(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, e := strconv.Atoi(p); e == nil {
			out = append(out, n)
		}
	}
	return out
}
