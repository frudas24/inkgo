package engine

import (
	"io"
	"sync"
)

// TerminalQuery pairs an outbound escape sequence with a matcher for the
// expected inbound terminal response.
type TerminalQuery struct {
	Request string
	Match   func(TerminalResponse) bool
}

func QueryDECRQM(mode int) TerminalQuery {
	return TerminalQuery{Request: CSI("?", mode, "$p"), Match: func(r TerminalResponse) bool { return r.Type == "decrpm" && r.Mode == mode }}
}

func QueryDA1() TerminalQuery {
	return TerminalQuery{Request: CSI("c"), Match: func(r TerminalResponse) bool { return r.Type == "da1" }}
}

func QueryDA2() TerminalQuery {
	return TerminalQuery{Request: CSI(">c"), Match: func(r TerminalResponse) bool { return r.Type == "da2" }}
}

func QueryKittyKeyboard() TerminalQuery {
	return TerminalQuery{Request: CSI("?u"), Match: func(r TerminalResponse) bool { return r.Type == "kittyKeyboard" }}
}

func QueryCursorPosition() TerminalQuery {
	return TerminalQuery{Request: CSI("?6n"), Match: func(r TerminalResponse) bool { return r.Type == "cursorPosition" }}
}

func QueryOSCColor(code int) TerminalQuery {
	return TerminalQuery{Request: OSC(code, "?"), Match: func(r TerminalResponse) bool { return r.Type == "osc" && r.Code == code }}
}

func QueryXTVERSION() TerminalQuery {
	return TerminalQuery{Request: CSI(">0q"), Match: func(r TerminalResponse) bool { return r.Type == "xtversion" }}
}

type pendingTerminalQuery struct {
	query    *TerminalQuery
	result   chan *TerminalResponse
	sentinel chan struct{}
}

// TerminalQuerier is a timeout-free terminal capability query manager. Query
// batches are closed by a DA1 sentinel, exactly like the TypeScript fork:
// responses arriving before the sentinel resolve their matching request;
// requests still pending when the sentinel arrives resolve as unsupported.
type TerminalQuerier struct {
	mu      sync.Mutex
	out     io.Writer
	pending []*pendingTerminalQuery
}

func NewTerminalQuerier(out io.Writer) *TerminalQuerier {
	return &TerminalQuerier{out: out}
}

// Send writes a terminal query and returns a receive-only channel that yields
// a response pointer or nil if a later Flush sentinel proves it unsupported.
func (q *TerminalQuerier) Send(query TerminalQuery) <-chan *TerminalResponse {
	out := make(chan *TerminalResponse, 1)
	p := &pendingTerminalQuery{query: &query, result: out}
	q.mu.Lock()
	q.pending = append(q.pending, p)
	if q.out != nil {
		_, _ = io.WriteString(q.out, query.Request)
	}
	q.mu.Unlock()
	return out
}

// Flush writes the DA1 sentinel and returns a channel closed when its response
// is observed. Pending queries queued before this barrier resolve nil first.
func (q *TerminalQuerier) Flush() <-chan struct{} {
	done := make(chan struct{})
	q.mu.Lock()
	q.pending = append(q.pending, &pendingTerminalQuery{sentinel: done})
	if q.out != nil {
		_, _ = io.WriteString(q.out, CSI("c"))
	}
	q.mu.Unlock()
	return done
}

// OnResponse routes one parsed response into the first matching pending query,
// otherwise a DA1 response completes the first sentinel barrier.
func (q *TerminalQuerier) OnResponse(r TerminalResponse) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, p := range q.pending {
		if p.query != nil && p.query.Match != nil && p.query.Match(r) {
			copy(q.pending[i:], q.pending[i+1:])
			q.pending[len(q.pending)-1] = nil
			q.pending = q.pending[:len(q.pending)-1]
			rc := r
			p.result <- &rc
			close(p.result)
			return
		}
	}
	if r.Type != "da1" {
		return
	}
	barrier := -1
	for i, p := range q.pending {
		if p.sentinel != nil {
			barrier = i
			break
		}
	}
	if barrier < 0 {
		return
	}
	items := append([]*pendingTerminalQuery(nil), q.pending[:barrier+1]...)
	copy(q.pending, q.pending[barrier+1:])
	for i := len(q.pending) - barrier - 1; i < len(q.pending); i++ {
		if i >= 0 {
			q.pending[i] = nil
		}
	}
	q.pending = q.pending[:len(q.pending)-barrier-1]
	for _, p := range items {
		if p.query != nil {
			p.result <- nil
			close(p.result)
		} else if p.sentinel != nil {
			close(p.sentinel)
		}
	}
}

// Close resolves all outstanding requests as unsupported and closes any
// sentinel waiters. It is safe to call during runtime shutdown.
func (q *TerminalQuerier) Close() {
	q.mu.Lock()
	items := q.pending
	q.pending = nil
	q.mu.Unlock()
	for _, p := range items {
		if p.query != nil {
			p.result <- nil
			close(p.result)
		} else if p.sentinel != nil {
			close(p.sentinel)
		}
	}
}
