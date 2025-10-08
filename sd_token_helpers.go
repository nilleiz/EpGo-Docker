package main

import (
	"sync"
	"time"
)

// Global token state shared across the process.
// Ensures we log in at most ~once per 24h and avoid "too many logins".
var (
	sdToken        string
	sdTokenExpiry  time.Time
	sdTokenMu      sync.RWMutex // guards reads of sdToken/sdTokenExpiry
	sdRefreshMutex sync.Mutex   // serializes refresh so we don't stampede SD
)

// getSDToken returns a valid Schedules Direct token.
// - Reuses the cached token until near expiry (10 min margin)
// - Serializes refresh so only one goroutine logs in
func getSDToken() (string, error) {
	// Fast path: read lock
	sdTokenMu.RLock()
	token := sdToken
	exp := sdTokenExpiry
	sdTokenMu.RUnlock()

	const refreshMargin = 10 * time.Minute
	now := time.Now()
	if token != "" && now.Before(exp.Add(-refreshMargin)) {
		return token, nil
	}

	// Slow path: serialize refresh
	sdRefreshMutex.Lock()
	defer sdRefreshMutex.Unlock()

	// Re-check after acquiring the lock
	sdTokenMu.RLock()
	token = sdToken
	exp = sdTokenExpiry
	sdTokenMu.RUnlock()
	if token != "" && now.Before(exp.Add(-refreshMargin)) {
		return token, nil
	}

	// Do a real login once
	var sd SD
	if err := sd.Init(); err != nil {
		return "", err
	}
	if err := sd.Login(); err != nil {
		return "", err
	}

	// sd.Login sets sd.Token and sd.Resp.Login.TokenExpires (unix seconds)
	newToken := sd.Token
	newExp := time.Unix(sd.Resp.Login.TokenExpires, 0)

	sdTokenMu.Lock()
	sdToken = newToken
	sdTokenExpiry = newExp
	sdTokenMu.Unlock()

	return newToken, nil
}

// forceRefreshToken clears the cached token and performs a refresh once.
// Used to retry once on 401 Unauthorized.
func forceRefreshToken() (string, error) {
	sdTokenMu.Lock()
	sdToken = ""
	sdTokenExpiry = time.Time{}
	sdTokenMu.Unlock()
	return getSDToken()
}
