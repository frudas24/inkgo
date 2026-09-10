package core

// Key is a normalized terminal key event. Sequence preserves the original
// bytes while Name/Text and modifiers provide terminal-independent semantics.
type Key struct {
	Name     string
	Text     string
	Sequence string
	Ctrl     bool
	Alt      bool
	Shift    bool
	Meta     bool
	Super    bool
	Hyper    bool
	Release  bool
	Repeat   bool
}
