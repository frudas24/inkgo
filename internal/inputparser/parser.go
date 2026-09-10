package inputparser

import (
	core "github.com/frudas24/inkgo/internal/core"
	escscan "github.com/frudas24/inkgo/internal/escscan"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type Key = core.Key

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
	st         = "\x1b\\"
	focusIn    = "\x1b[I"
	focusOut   = "\x1b[O"
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
	Value  string // compatibility alias for Data/Name

	Mode, Status int
	Flags        int
	Row, Col     int
	Code         int
	Data, Name   string
}

type InputParser struct {
	mu      sync.Mutex
	buffer  string
	inPaste bool
	paste   strings.Builder
}

func NewInputParser() *InputParser { return &InputParser{} }
func (p *InputParser) Buffer() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buffer
}
func (p *InputParser) InPaste() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inPaste
}
func (p *InputParser) Pending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inPaste || p.buffer != ""
}

var sgrMouseRE = regexp.MustCompile(`^\x1b\[<(\d+);(\d+);(\d+)([Mm])`)
var csiURE = regexp.MustCompile(`^\x1b\[(\d+)(?:;(\d+))?u`)
var modifyOtherRE = regexp.MustCompile(`^\x1b\[27;(\d+);(\d+)~`)

func (p *InputParser) Feed(data []byte) []ParsedInput {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buffer += string(data)
	return p.consume(false)
}
func (p *InputParser) Flush() []ParsedInput {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.consume(true)
}

func (p *InputParser) consume(flush bool) []ParsedInput {
	var out []ParsedInput
	for len(p.buffer) > 0 {
		if p.inPaste {
			if i := strings.Index(p.buffer, pasteEnd); i >= 0 {
				p.paste.WriteString(p.buffer[:i])
				out = append(out, ParsedInput{Kind: InputPaste, Paste: p.paste.String(), Sequence: p.paste.String()})
				p.paste.Reset()
				p.inPaste = false
				p.buffer = p.buffer[i+len(pasteEnd):]
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
			keep := len(pasteEnd) - 1
			if len(p.buffer) > keep {
				p.paste.WriteString(p.buffer[:len(p.buffer)-keep])
				p.buffer = p.buffer[len(p.buffer)-keep:]
			}
			break
		}
		if strings.HasPrefix(p.buffer, pasteStart) {
			p.inPaste = true
			p.buffer = p.buffer[len(pasteStart):]
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
		seq, complete := NextEscapeSequence(p.buffer)
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
		if seq == focusIn {
			out = append(out, ParsedInput{Kind: InputResponse, Response: TerminalResponse{Type: "focus-in"}, Sequence: seq})
			continue
		}
		if seq == focusOut {
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

func NextEscapeSequence(s string) (string, bool) { return escscan.NextSequence(s) }

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
	if m := sgrMouseRE.FindStringSubmatch(s); m != nil {
		btn, _ := strconv.Atoi(m[1])
		col, _ := strconv.Atoi(m[2])
		row, _ := strconv.Atoi(m[3])
		a := "press"
		if m[4] == "m" {
			a = "release"
		}
		return &ParsedMouse{Button: btn, Action: a, Col: col, Row: row}
	}
	// X10: ESC [ M followed by encoded button/x/y bytes, each biased by 32.
	if len(s) == 6 && strings.HasPrefix(s, "\x1b[M") {
		btn := int(s[3]) - 32
		col := int(s[4]) - 32
		row := int(s[5]) - 32
		if btn < 0 || col <= 0 || row <= 0 {
			return nil
		}
		action := "press"
		if btn&3 == 3 && btn&0x40 == 0 {
			action = "release"
		}
		return &ParsedMouse{Button: btn, Action: action, Col: col, Row: row}
	}
	return nil
}

func parseResponse(s string) *TerminalResponse {
	if strings.HasPrefix(s, "\x1b[?") && strings.HasSuffix(s, "$y") {
		body := strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[?"), "$y")
		parts := parseInts(body)
		if len(parts) >= 2 {
			return &TerminalResponse{Type: "decrpm", Params: parts, Mode: parts[0], Status: parts[1]}
		}
	}
	if strings.HasPrefix(s, "\x1b[?") && strings.HasSuffix(s, "c") {
		params := parseInts(strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[?"), "c"))
		return &TerminalResponse{Type: "da1", Params: params}
	}
	if strings.HasPrefix(s, "\x1b[>") && strings.HasSuffix(s, "c") {
		params := parseInts(strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[>"), "c"))
		return &TerminalResponse{Type: "da2", Params: params}
	}
	if strings.HasPrefix(s, "\x1b[?") && strings.HasSuffix(s, "u") {
		body := strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[?"), "u")
		if n, err := strconv.Atoi(body); err == nil {
			return &TerminalResponse{Type: "kittyKeyboard", Flags: n, Params: []int{n}}
		}
	}
	if strings.HasPrefix(s, "\x1b[?") && strings.HasSuffix(s, "R") {
		body := strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[?"), "R")
		parts := parseInts(body)
		if len(parts) >= 2 {
			return &TerminalResponse{Type: "cursorPosition", Row: parts[0], Col: parts[1], Params: parts}
		}
	}
	if strings.HasPrefix(s, "\x1b]") {
		body := strings.TrimPrefix(s, "\x1b]")
		body = strings.TrimSuffix(body, st)
		body = strings.TrimSuffix(body, "\x07")
		if semi := strings.IndexByte(body, ';'); semi > 0 {
			if code, err := strconv.Atoi(body[:semi]); err == nil {
				data := body[semi+1:]
				return &TerminalResponse{Type: "osc", Code: code, Data: data, Value: data, Params: []int{code}}
			}
		}
	}
	if strings.HasPrefix(s, "\x1bP>|") {
		name := strings.TrimSuffix(strings.TrimPrefix(s, "\x1bP>|"), st)
		name = strings.TrimSuffix(name, "\x07")
		return &TerminalResponse{Type: "xtversion", Name: name, Data: name, Value: name}
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
