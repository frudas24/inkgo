package textutil

import (
	"strings"
	"testing"

	core "github.com/frudas24/inkgo/internal/core"
)

// benchmarkTranscriptLine is the shape the TUI audit measured: one 1024-byte
// printable-ASCII entry body of the kind a chat transcript holds per turn.
func benchmarkTranscriptLine() string {
	unit := "alpha bravo charlie delta echo foxtrot golf hotel "
	var sb strings.Builder
	for sb.Len() < 1024 {
		sb.WriteString(unit)
	}
	return sb.String()[:1024]
}

func BenchmarkStringWidthASCII(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog 0123456789"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StringWidth(text)
	}
}

func BenchmarkStringWidthANSIASCII(b *testing.B) {
	text := "\x1b[32mThe quick brown fox jumps over the lazy dog 0123456789\x1b[0m"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StringWidth(text)
	}
}

func BenchmarkStringWidthEmoji(b *testing.B) {
	text := "✅ ⚠️ ©️ ™️ 👨‍👩‍👧‍👦 🇨🇴 🫍"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StringWidth(text)
	}
}

func BenchmarkWrapPlainASCII(b *testing.B) {
	text := benchmarkTranscriptLine()
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = WrapText(text, 100, core.TextWrapWrap)
	}
}

func BenchmarkMeasureTextPlainASCII(b *testing.B) {
	text := benchmarkTranscriptLine()
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = MeasureText(text, 100, core.TextWrapWrap)
	}
}

// BenchmarkMeasureTextTranscript measures a layout pass over a whole window of
// transcript entries, which is what a width change re-runs per text node.
func BenchmarkMeasureTextTranscript(b *testing.B) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = benchmarkTranscriptLine()
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			_ = MeasureText(line, 100, core.TextWrapWrap)
		}
	}
}

// BenchmarkWrapStyledASCII is the control: dropping visual state across rows and
// splitting words still has to happen when the line carries escapes.
func BenchmarkWrapStyledASCII(b *testing.B) {
	text := "\x1b[32m" + benchmarkTranscriptLine() + "\x1b[0m"
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = WrapText(text, 100, core.TextWrapWrap)
	}
}
