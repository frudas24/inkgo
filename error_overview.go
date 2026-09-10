package inkgo

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorOverview returns a small, dependency-free error view suitable for
// replacing the application tree after a fatal UI error. Go errors do not
// universally carry JavaScript-style source stacks, so this renders the error
// chain instead of guessing file excerpts.
func ErrorOverview(err error) *Node {
	if err == nil {
		return Box(Style{})
	}
	label := Text(" ERROR ", TextStyle{Bold: true, Color: ANSIColor(15), BackgroundColor: ANSIColor(1)})
	message := Text(err.Error(), TextStyle{Color: ANSIColor(1)})
	children := []*Node{label, message}

	var chain []string
	for cause := errors.Unwrap(err); cause != nil; cause = errors.Unwrap(cause) {
		chain = append(chain, cause.Error())
	}
	if len(chain) > 0 {
		children = append(children,
			Text("Caused by:", TextStyle{Bold: true}),
			Text(strings.Join(chain, "\n")),
		)
	}
	return Box(Style{
		FlexDirection: Column,
		PaddingX:      I(1),
		PaddingY:      I(1),
		Gap:           I(1),
	}, children...)
}

// ErrorfOverview is a convenience for callers that are converting a recovered
// panic into a visible error tree.
func ErrorfOverview(format string, args ...any) *Node {
	return ErrorOverview(fmt.Errorf(format, args...))
}
