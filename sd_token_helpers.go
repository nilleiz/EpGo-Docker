package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Global token state shared across the process.
// Ensures we log in at most ~once per 24h and avoid "too many logins".
var (
	sdToken           string
	sdTokenExpiry     time.Time
	sdTokenMu         sync.RWMutex // guards reads of sdToken/sdTokenExpiry
	sdRefreshMutex    sync.Mutex   // serializes refresh so we don't stampede SD
	forcedRefreshMu   sync.Mutex
	lastForcedRefresh time.Time
	bootTime          = time.Now().UTC()
)

const forcedRefreshCooldown = 5 * time.Minute

type persistedToken struct {
	Token       string    `json:"token"`
	TokenExpiry time.Time `json:"token_expiry_utc"`
}

func tokenFilePath() string {
	// Persist token next to the cache file as a sidecar JSON.
	p := Config.Files.Cache
	if p == "" {
		// default inside container
		return "/app/config_cache.sdtoken.json"
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	return base + ".sdtoken.json"
}

func loadTokenFromDisk() {
	path := tokenFilePath()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	var pt persistedToken
	if json.Unmarshal(data, &pt) == nil {
		// Accept only future-expiring tokens
		if pt.Token != "" && time.Now().UTC().Before(pt.TokenExpiry) {
			sdToken = pt.Token
			sdTokenExpiry = pt.TokenExpiry
			if logger != nil {
				logger.Info("SD token: loaded from disk",
					"expires_utc", sdTokenExpiry)
			}
		}
	}
}

func deleteTokenFromDisk() {
	path := tokenFilePath()
	_ = os.Remove(path)
	if logger != nil {
		logger.Warn("SD token: deleted persisted token", "path", path)
	}
}

func saveTokenToDisk(tok string, exp time.Time) {
	path := tokenFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	blob, _ := json.MarshalIndent(persistedToken{
		Token:       tok,
		TokenExpiry: exp.UTC(),
	}, "", "  ")
	_ = os.WriteFile(path, blob, 0644)
	if logger != nil {
		logger.Info("SD token: saved to disk", "expires_utc", exp.UTC())
	}
}

// ---- helpers to interpret SD error payloads ----

type sdLoginErr struct {
	Response   string `json:"response"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
	DateTime   string `json:"datetime"`
	ServerID   string `json:"serverID"`
	Token      string `json:"token"`
	ServerTime int64  `json:"serverTime"`
}

func parseSDLoginErr(b []byte) (sdLoginErr, bool) {
	var e sdLoginErr
	if err := json.Unmarshal(b, &e); err != nil {
		return e, false
	}
	// consider valid if either "response" or "code" is populated
	if e.Response == "" && e.Code == 0 && e.Message == "" {
		return e, false
	}
	return e, true
}

func refTimeFromErr(e sdLoginErr) time.Time {
	// Prefer serverTime epoch, then RFC3339 datetime, else now UTC
	if e.ServerTime > 0 {
		return time.Unix(e.ServerTime, 0).UTC()
	}
	if e.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, e.DateTime); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// getSDToken returns a valid Schedules Direct token.
// Behavior:
//   - On first call, tries to load a persisted token from disk.
//   - Reuses token until near expiry (10 min margin).
//   - Serializes refresh so only one goroutine logs in.
//   - Enforces a minimum spacing at boot to avoid multiple logins.
//   - If SD responds with TOO_MANY_LOGINS (4009), set a global pause until next UTC midnight +5m.
func getSDToken() (string, error) {
	return getSDTokenWithOptions(false)
}

func getSDTokenWithOptions(skipDiskLoad bool) (string, error) {
	// Initial lazy load from disk
	sdTokenMu.RLock()
	tokLoaded := sdToken != ""
	sdTokenMu.RUnlock()
	if !tokLoaded && !skipDiskLoad {
		sdTokenMu.Lock()
		if sdToken == "" {
			loadTokenFromDisk()
		}
		sdTokenMu.Unlock()
	}

	// Fast path
	sdTokenMu.RLock()
	token := sdToken
	exp := sdTokenExpiry
	sdTokenMu.RUnlock()

	const refreshMargin = 10 * time.Minute
	now := time.Now().UTC()
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

	// Extra safety at boot: if we already have a token and it's still valid,
	// avoid relogin storms within the first few minutes.
	if token != "" && now.Sub(bootTime) < 5*time.Minute && now.Before(exp) {
		if logger != nil {
			logger.Info("SD token: reusing boot-time token (no login)",
				"expires_utc", exp)
		}
		return token, nil
	}

	// Do a real login once
	if logger != nil {
		logger.Warn("SD token: performing LOGIN to Schedules Direct (serialized)")
	}

	var sd SD
	if err := sd.Init(); err != nil {
		return "", err
	}
	// sd.Login() populates sd.Resp.Login.Body even on non-200
	if err := sd.Login(); err != nil {
		// Inspect body for TOO_MANY_LOGINS (code 4009)
		if e, ok := parseSDLoginErr(sd.Resp.Body); ok {
			if strings.EqualFold(e.Response, "TOO_MANY_LOGINS") || e.Code == 4009 {
				// Pause globally until next UTC midnight + 5 minutes
				ref := refTimeFromErr(e)
				until := nextUTCMidnightPlus(ref, 5)
				setGlobalPauseUntil(until, "TOO_MANY_LOGINS (403) — login disabled by SD")
				if logger != nil {
					logger.Error("SD token: login disabled due to TOO_MANY_LOGINS; global pause set",
						"retry_at_utc", until, "message", e.Message)
				}
			}
		}
		if logger != nil {
			logger.Error("SD token: LOGIN failed", "error", err)
		}
		return "", err
	}

	newToken := sd.Token
	newExp := time.Unix(sd.Resp.Login.TokenExpires, 0).UTC()

	// Persist and publish
	sdTokenMu.Lock()
	sdToken = newToken
	sdTokenExpiry = newExp
	sdTokenMu.Unlock()
	saveTokenToDisk(newToken, newExp)

	if logger != nil {
		logger.Info("SD token: LOGIN succeeded", "expires_utc", newExp)
	}

	return newToken, nil
}

// forceRefreshToken clears the cached token and performs a refresh once.
// Used to retry once on 401 Unauthorized.
func forceRefreshToken() (string, error) {
	if logger != nil {
		logger.Warn("SD token: forced refresh requested (clearing token)")
	}
	sdTokenMu.Lock()
	sdToken = ""
	sdTokenExpiry = time.Time{}
	sdTokenMu.Unlock()
	deleteTokenFromDisk()
	return getSDTokenWithOptions(true)
}

// forceRefreshTokenLimited enforces a cooldown between forced refreshes to prevent
// tight retry loops from spamming the Schedules Direct login endpoint.
// Returns the new token, a boolean indicating whether a refresh was attempted,
// and any error from the refresh attempt. If overrideCooldown is true, the
// cooldown check is bypassed (but the timestamp is still updated) so callers
// can recover immediately from scenarios like SD invalidating tokens when the
// client IP changes.
func forceRefreshTokenLimited(overrideCooldown bool) (string, bool, error) {
	forcedRefreshMu.Lock()
	defer forcedRefreshMu.Unlock()

	now := time.Now().UTC()
	if !lastForcedRefresh.IsZero() && now.Sub(lastForcedRefresh) < forcedRefreshCooldown {
		// We refreshed very recently. Reuse the current token instead of logging in again
		// to avoid spamming SD when multiple requests fail simultaneously.
		tok, _ := getSDTokenWithOptions(false)

		retryAt := lastForcedRefresh.Add(forcedRefreshCooldown)
		if logger != nil {
			logger.Warn("SD token: forced refresh suppressed due to cooldown", "retry_at_utc", retryAt)
		}

		if tok != "" {
			// Treat as a successful refresh so callers retry with the latest token.
			return tok, true, nil
		}

		if !overrideCooldown {
			return "", false, nil
		}
	}

	lastForcedRefresh = now

	tok, err := forceRefreshToken()
	if err != nil {
		return "", true, err
	}

	return tok, true, nil
}
