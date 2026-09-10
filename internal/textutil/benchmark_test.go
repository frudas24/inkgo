package textutil

import "testing"

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
