package engine

import (
	"time"

	sched "github.com/frudas24/inkgo/scheduler"
)

// Clock is the shared animation/interval clock implementation owned by the
// scheduler domain. The root alias preserves the round-1/2 import surface.
type Clock = sched.Clock

// NewClock creates an idle shared clock. Prefer scheduler.New in new code.
func NewClock(interval time.Duration) *Clock { return sched.New(interval) }
