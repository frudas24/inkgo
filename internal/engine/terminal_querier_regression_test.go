package engine

import (
	"errors"
	"testing"
)

type queryFailWriter struct {
	writes int
	failAt int
	short  bool
}

func (w *queryFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		if w.short && len(p) > 0 {
			return len(p) - 1, nil
		}
		return 0, errors.New("query output failed")
	}
	return len(p), nil
}

func TestTerminalQuerierSendWriteFailureResolvesImmediately(t *testing.T) {
	w := &queryFailWriter{failAt: 1}
	q := NewTerminalQuerier(w)
	result := q.Send(QueryDA2())
	if len(q.pending) != 0 {
		t.Fatalf("failed query write left impossible waiter pending: %d", len(q.pending))
	}
	select {
	case got, ok := <-result:
		if !ok || got != nil {
			t.Fatalf("failed query write resolved %#v, open=%v; want one nil result", got, ok)
		}
	default:
		t.Fatal("failed query write left result channel blocked")
	}
}

func TestTerminalQuerierFlushWriteFailureResolvesCurrentWaiters(t *testing.T) {
	w := &queryFailWriter{failAt: 2}
	q := NewTerminalQuerier(w)
	result := q.Send(QueryDA2()) // first write succeeds
	done := q.Flush()            // DA1 sentinel write fails
	if len(q.pending) != 0 {
		t.Fatalf("failed flush write left impossible waiters pending: %d", len(q.pending))
	}
	select {
	case got, ok := <-result:
		if !ok || got != nil {
			t.Fatalf("failed flush resolved query as %#v, open=%v; want one nil result", got, ok)
		}
	default:
		t.Fatal("failed flush left query result blocked")
	}
	select {
	case <-done:
	default:
		t.Fatal("failed flush left sentinel waiter blocked")
	}
}

func TestTerminalQuerierShortWriteIsFailure(t *testing.T) {
	w := &queryFailWriter{failAt: 1, short: true}
	q := NewTerminalQuerier(w)
	result := q.Send(QueryDA2())
	select {
	case got := <-result:
		if got != nil {
			t.Fatalf("short query write resolved %#v; want nil", got)
		}
	default:
		t.Fatal("short query write left result blocked")
	}
}
