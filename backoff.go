package main

import (
	"sync"
	"time"
)

// Global pause for SD image fetches.
// In-memory only (resets on container restart).
var (
	imageFetchPauseMu    sync.RWMutex
	imageFetchPauseUntil time.Time // UTC instant when pause ends; zero => no pause
)

// shouldBlockGlobal reports whether a global pause is active, and the remaining duration.
func shouldBlockGlobal() (bool, time.Duration) {
	imageFetchPauseMu.RLock()
	defer imageFetchPauseMu.RUnlock()
	if imageFetchPauseUntil.IsZero() {
		return false, 0
	}
	now := time.Now().UTC()
	if now.Before(imageFetchPauseUntil) {
		return true, time.Until(imageFetchPauseUntil)
	}
	return false, 0
}

// setGlobalPauseUntil sets a global pause until the given UTC instant.
func setGlobalPauseUntil(until time.Time, reason string) {
	if until.IsZero() {
		return
	}
	imageFetchPauseMu.Lock()
	defer imageFetchPauseMu.Unlock()
	// Only extend, never shorten, to avoid flapping under load.
	if until.After(imageFetchPauseUntil) {
		imageFetchPauseUntil = until
		logger.Warn("Proxy: global image fetch paused", "until_utc", until, "reason", reason)
	}
}

// clearGlobalPause clears any global pause (useful for debugging/admin endpoints).
func clearGlobalPause() {
	imageFetchPauseMu.Lock()
	imageFetchPauseUntil = time.Time{}
	imageFetchPauseMu.Unlock()
	logger.Info("Proxy: global image fetch pause cleared")
}

// nextUTCMidnightPlus returns the next UTC midnight after 'ref' plus 'mins' minutes.
func nextUTCMidnightPlus(ref time.Time, mins int) time.Time {
	ref = ref.UTC()
	next := time.Date(ref.Year(), ref.Month(), ref.Day()+1, 0, 0, 0, 0, time.UTC)
	return next.Add(time.Duration(mins) * time.Minute)
}
