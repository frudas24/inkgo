package engine

import (
	"io"
	"strings"
	"testing"
)

func TestHyperlinksRejectTerminalControls(t *testing.T) {
	urls := []string{
		"https://example.com\x07\x1b]0;injected-title\x07",
		"https://example.com\x1b\\\x1b[2J",
		"https://example.com\u009c\u009b2J",
		"https://example.com\x9c\x1b[2J",
		"https://example.com\nline",
		"https://example.com\x00tail",
		"https://example.com\x7ftail",
	}
	for _, url := range urls {
		if got := Hyperlink(url, "click"); got != "click" {
			t.Errorf("unsafe helper output for %q: %q", url, got)
		}
		screen := NewScreen(1, 1)
		screen.SetCell(0, 0, "x", 1, TextStyle{}, url)
		patches := []string{DiffScreens(nil, screen), DiffScreensRelative(nil, screen), DiffScreensRelative(NewScreen(1, 1), screen)}
		for _, fullscreen := range []bool{false, true} {
			root := Root(Link(url, Text("click")))
			patches = append(patches, NewRenderer(RenderOptions{Width: 20, Height: 4, Fullscreen: fullscreen}).Render(root).Patch)
		}
		for _, patch := range patches {
			// Rejected URLs emit neither a hyperlink nor fragments of its payload.
			if strings.Contains(patch, "\x1b]8;") || strings.Contains(patch, "example.com") || strings.Contains(patch, "injected-title") || strings.ContainsAny(patch, "\u009b\u009c") {
				t.Errorf("unsafe patch for %q: %q", url, patch)
			}
		}
	}
}

func TestHyperlinksPreserveValidUnicodeURLs(t *testing.T) {
	url := "https://example.com/日本語?q=a%20b#片"
	expected := OSC(8, "", url)
	if got := Hyperlink(url, "click"); got != expected+"click"+OSC(8, "", "") {
		t.Fatalf("valid helper output = %q", got)
	}
	screen := NewScreen(1, 1)
	screen.SetCell(0, 0, "x", 1, TextStyle{}, url)
	for _, patch := range []string{DiffScreens(nil, screen), DiffScreensRelative(nil, screen), DiffScreensRelative(NewScreen(1, 1), screen)} {
		if !strings.Contains(patch, expected) {
			t.Errorf("valid URL lost: %q", patch)
		}
	}
}

func TestRuntimeStopCancelsRemainingInputBatch(t *testing.T) {
	for _, first := range []string{"q", PasteStart + "quit" + PasteEnd} {
		root := Root(Text("x"))
		rt := NewRuntime(root, strings.NewReader(first+"a\x1b"), io.Discard, RenderOptions{})
		var calls []string
		root.Handlers.OnKeyDown = func(e *KeyboardEvent) { calls = append(calls, e.Key.Name); rt.Stop() }
		rt.OnPaste = func(text string) { calls = append(calls, text); rt.Stop() }
		if err := rt.Run(); err != nil {
			t.Fatal(err)
		}
		rt.HandleInput([]byte("b"))
		rt.flushIncompleteInput()
		if len(calls) != 1 {
			t.Fatalf("callbacks after Stop: %v", calls)
		}
		if rt.incompleteTimer != nil {
			t.Fatal("stopped runtime armed a timer")
		}
	}
}

func TestTerminalQueryResponsesRespectBarriers(t *testing.T) {
	q := NewTerminalQuerier(io.Discard)
	defer q.Close()
	first := q.Send(QueryXTVERSION())
	barrier := q.Flush()
	later := q.Send(QueryDA1())
	secondBarrier := q.Flush()
	q.OnResponse(TerminalResponse{Type: "da1"})
	select {
	case <-barrier:
	default:
		t.Fatal("first barrier not closed")
	}
	select {
	case response := <-first:
		if response != nil {
			t.Fatal("unanswered query resolved with response")
		}
	default:
		t.Fatal("earlier query unresolved")
	}
	select {
	case <-later:
		t.Fatal("sentinel answered a query after its barrier")
	default:
	}
	q.OnResponse(TerminalResponse{Type: "da1"})
	select {
	case response := <-later:
		if response == nil || response.Type != "da1" {
			t.Fatal("later DA1 query missing response")
		}
	default:
		t.Fatal("later DA1 query unresolved")
	}
	select {
	case <-secondBarrier:
		t.Fatal("query reply closed its following barrier")
	default:
	}
	q.OnResponse(TerminalResponse{Type: "da1"})
	select {
	case <-secondBarrier:
	default:
		t.Fatal("second barrier not closed")
	}
}

func TestTerminalQueriesDoNotMatchAcrossPendingBarrier(t *testing.T) {
	q := NewTerminalQuerier(io.Discard)
	defer q.Close()
	barrier := q.Flush()
	later := q.Send(QueryXTVERSION())
	q.OnResponse(TerminalResponse{Type: "xtversion", Name: "early"})
	select {
	case <-later:
		t.Fatal("matched response beyond uncompleted barrier")
	default:
	}
	q.OnResponse(TerminalResponse{Type: "da1"})
	select {
	case <-barrier:
	default:
		t.Fatal("barrier not closed")
	}
	q.OnResponse(TerminalResponse{Type: "xtversion", Name: "current"})
	select {
	case response := <-later:
		if response == nil || response.Name != "current" {
			t.Fatal("wrong batch response")
		}
	default:
		t.Fatal("query unresolved")
	}
}
