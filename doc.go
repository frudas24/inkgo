// Package ink is a native Go rewrite of a customized Ink terminal-UI core.
//
// It deliberately does not embed Node.js or emulate React. The host tree is a
// concrete Go Node tree that callers can mutate through stable references.
// Layout, terminal-cell rendering, incremental diffs, input parsing, focus,
// mouse interaction, scroll boxes, selection and ANSI handling are native Go.
package inkgo
