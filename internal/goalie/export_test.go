package goalie

import (
	"testing"
	"time"
)

// SetSlowCall sets the time after which goalie serve logs a call, which users can't change.
func SetSlowCall(t *testing.T, limit time.Duration) {
	old := slowCall
	slowCall = limit
	t.Cleanup(func() { slowCall = old })
}
