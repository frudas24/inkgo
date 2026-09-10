// Package scheduler exposes the shared animation clock. It consolidates many
// UI animations and intervals into a single wake-up source.
package scheduler

import (
	"time"

	tui "github.com/frudas24/inkgo"
)

type Clock = tui.Clock

func New(interval time.Duration) *Clock { return tui.NewClock(interval) }
