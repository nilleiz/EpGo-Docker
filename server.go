package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ensureProgramMetadata fetches SD metadata for a single ProgramID and stores it in Cache.
// Returns true if metadata is present afterwards.
func ensureProgramMetadata(programID string) bool {
	if _, ok := Cache.Metadata[programID]; ok {
		return true
	}

	logger.Info("Proxy: metadata missing, fetching", "programID", programID)

	var sd SD
	if err := sd.Init(); err != nil {
		logger.Error("Proxy: SD init failed", "programID", programID, "error", err)
		return false
	}
	tok, err := getSDToken()
	if err != nil {
		logger.Error("Proxy: token fetch failed (metadata)", "programID", programID, "error", err)
		return false
	}
	sd.Token = tok

	sd.Req.URL = fmt.Sprintf("%smetadata/programs/", sd.BaseURL)
	sd.Req.Type = "POST"
	sd.Req.Call = "metadata"
	// IMPORTANT: AddMetadata expects gzipped body → request compressed response
	sd.Req.Compression = true

	body, err := json.Marshal([]string{programID})
	if err != nil {
		logger.Error("Proxy: marshal metadata request failed", "programID", programID, "error", err)
		return false
	}
	sd.Req.Data = body

	if err := sd.Connect(); err != nil {
		logger.Warn("Proxy: SD metadata connect failed, will try refresh", "programID", programID, "error", err)
		if tok2, err2 := forceRefreshToken(); err2 == nil {
			sd.Token = tok2
			if err3 := sd.Connect(); err3 != nil {
				logger.Error("Proxy: SD metadata connect failed after refresh", "programID", programID, "error", err3)
				return false
			}
		} else {
			logger.Error("Proxy: token refresh failed (metadata)", "programID", programID, "error", err2)
			return false
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	Cache.AddMetadata(&sd.Resp.Body, &wg)
	wg.Wait()

	if err := Cache.Save(); err != nil {
		logger.Warn("Proxy: cache save after metadata fetch failed", "programID", programID, "error", err)
	}

	if _, ok := Cache.Metadata[programID]; ok {
		logger.Info("Proxy: metadata stored", "programID", programID)
		return true
	}

	logger.Warn("Proxy: metadata fetch returned no entry", "programID", programID)
	return false
}

// sdImageIDFromURI extracts the SD image ID from a URI or path-like string.
func sdImageIDFromURI(uri string) string {
	if uri == "" {
		return ""
	}
	// If it's a URL, parse the path; else treat as path/ID
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		if u, err := url.Parse(uri); err == nil {
			last := filepath.Base(u.Path)
			return strings.TrimSuffix(last, ".jpg")
		}
	}
	// trim trailing .jpg if present
	return strings.TrimSuffix(filepath.Base(uri), ".jpg")
}

// lookupImageMeta finds Category/Aspect/Width/Height of an image by programID+imageID.
// Selection rules mirror resolveSDImageForProgram, but scoped to a single imageID:
// - Allowed categories only
// - Enforce poster aspect when configured (so banner/box art with other aspects are ignored)
// - Prefer show > season > episode via Tier
// - Then prefer higher-ranked categories (e.g., Banner-L1 over Banner-L2)
// - Ties by width
func lookupImageMeta(programID, imageID string) (category, aspect string, width, height int, ok bool) {
	m, ok := Cache.Metadata[programID]
	if !ok {
		return "", "", 0, 0, false
	}

	desired := strings.TrimSpace(Config.Options.Images.PosterAspect)

	bestScore := 1 << 30
	bestWidth := -1
	var best Data

	for _, d := range m.Data {
		if sdImageIDFromURI(d.URI) != imageID {
			continue
		}

		catRank, allowed := allowedCategoryRank(d.Category)
		if !allowed {
			continue
		}

		if desired != "" && !strings.EqualFold(desired, "all") && !strings.EqualFold(d.Aspect, desired) {
			continue
		}

		aspectRankVal := 0
		if desired == "" || strings.EqualFold(desired, "all") {
			aspectRankVal = aspectRank(d.Aspect)
		}

		score := tierRank(d.Tier)*100 + catRank*10 + aspectRankVal

		if score < bestScore || (score == bestScore && d.Width > bestWidth) {
			bestScore = score
			bestWidth = d.Width
			best = d
		}
	}

	if best.URI == "" {
		return "", "", 0, 0, false
	}

	return best.Category, best.Aspect, best.Width, best.Height, true
}

// sdErrorTime extracts a reference time from an SD JSON error body.
// Understands "serverTime" (unix seconds) or "datetime" (RFC3339). Returns UTC (or zero on failure).
func sdErrorTime(buf []byte) time.Time {
	type sdErr struct {
		DateTime   string `json:"datetime"`
		ServerTime int64  `json:"serverTime"`
	}
	var e sdErr
	if err := json.Unmarshal(buf, &e); err != nil {
		return time.Time{}
	}
	if e.ServerTime > 0 {
		return time.Unix(e.ServerTime, 0).UTC()
	}
	if e.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, e.DateTime); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// StartServer starts a local HTTP server: static files + SD image proxy (pinned + legacy).
func StartServer(dir string, port string) {
	// Ensure cached programme metadata is available even if the last EPG refresh failed
	if err := Cache.Open(); err != nil {
		logger.Warn("Proxy: unable to open cache; override resolution may be limited", "error", err)
	}

	// Load ProgramID → imageID index
	indexInit()

	cacheDays := Config.Options.Images.MaxCacheAgeDays
	folderImage := Config.Options.Images.Path
	if folderImage == "" {
		folderImage = "images"
	}

	if Config.Options.Images.PurgeStale && cacheDays > 0 {
		if removed, err := purgeStalePosterFiles(folderImage, cacheDays); err != nil {
			logger.Warn("Proxy: purge of stale cached posters failed", "path", folderImage, "error", err)
		} else if removed > 0 {
			logger.Info("Proxy: purged stale cached posters on startup", "removed", removed, "purge_after_days", cacheDays*2, "path", folderImage)
		}
	}

	if cacheDays > 0 {
		logger.Info("Proxy: configured max cache age", "max_cache_days", cacheDays)
	} else {
		logger.Info("Proxy: configured for indefinite cache age", "max_cache_days", cacheDays)
	}

	mux := http.NewServeMux()

	// /proxy/sd/{programID}[/<imageID>]
	mux.HandleFunc("/proxy/sd/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/proxy/sd/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "missing programID", http.StatusBadRequest)
			return
		}
		programID := strings.TrimSuffix(parts[0], ".jpg")
		imageID := ""
		if len(parts) >= 2 && parts[1] != "" {
			imageID = strings.TrimSuffix(parts[1], ".jpg")
		}

		if overrideID, ok := overrideImageForProgram(programID); ok {
			imageID = overrideID
		}

		blockGlobal, blockRemain := shouldBlockGlobal()

		// Ensure image folder exists
		folderImage := Config.Options.Images.Path
		if folderImage == "" {
			folderImage = "images"
		}
		if err := os.MkdirAll(folderImage, 0755); err != nil {
			http.Error(w, "failed to prepare image folder", http.StatusInternalServerError)
			return
		}

		// --- PINNED MODE: /proxy/sd/{programID}/{imageID} ---
		if imageID != "" {
			filePath := filepath.Join(folderImage, imageID+".jpg")

			logWithMeta := func(prefix string, allowMetaFetch bool) {
				cat, asp, wpx, hpx, ok := lookupImageMeta(programID, imageID)
				if ok {
					logger.Info(prefix,
						"programID", programID, "imageID", imageID,
						"category", cat, "aspect", asp, "w", wpx, "h", hpx, "path", filePath)
				} else {
					if allowMetaFetch && ensureProgramMetadata(programID) {
						if cat2, asp2, w2, h2, ok2 := lookupImageMeta(programID, imageID); ok2 {
							logger.Info(prefix,
								"programID", programID, "imageID", imageID,
								"category", cat2, "aspect", asp2, "w", w2, "h", h2, "path", filePath)
							return
						}
					}
					logger.Info(prefix+" (no meta)",
						"programID", programID, "imageID", imageID, "path", filePath)
				}
			}

			// 1) Serve from disk if present
			if fi, err := os.Stat(filePath); err == nil && !fi.IsDir() {
				logWithMeta("Proxy: serve pinned from cache", !blockGlobal)
				_ = indexSet(programID, imageID)
				serveFileCached(w, r, filePath)
				return
			}

			if blockGlobal {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", blockRemain.Seconds()))
				http.Error(w, "image downloads paused due to upstream limits", http.StatusTooManyRequests)
				logger.Warn("Proxy: global pause in effect; denying image download", "programID", programID, "remaining", blockRemain)
				return
			}

			// 2) Download pinned asset directly (no resolver)
			token, err := getSDToken()
			if err != nil {
				logger.Error("Proxy: token error before pinned fetch", "programID", programID, "imageID", imageID, "error", err)
				http.Error(w, "token error", http.StatusBadGateway)
				return
			}
			imageURL := fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s.jpg?token=%s", imageID, token)
			logger.Info("Proxy: downloading pinned image", "programID", programID, "imageID", imageID, "url", imageURL)

			client := &http.Client{Timeout: 20 * time.Second}
			fetch := func(url string) (*http.Response, error) {
				req, _ := http.NewRequest("GET", url, nil)
				req.Header.Set("User-Agent", userAgent())
				return client.Do(req)
			}

			resp, err := fetch(imageURL)
			if err != nil {
				logger.Error("Proxy: pinned fetch failed", "programID", programID, "imageID", imageID, "error", err)
				http.Error(w, "fetch failed", http.StatusBadGateway)
				return
			}
			if resp.StatusCode == http.StatusUnauthorized {
				logger.Warn("Proxy: SD token unauthorized for pinned fetch, refreshing", "programID", programID, "imageID", imageID)
				resp.Body.Close()
				if token2, err2 := forceRefreshToken(); err2 == nil {
					imageURL = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s.jpg?token=%s", imageID, token2)
					resp, err = fetch(imageURL)
					if err != nil {
						logger.Error("Proxy: pinned fetch retry failed", "programID", programID, "imageID", imageID, "error", err)
						http.Error(w, "fetch retry failed", http.StatusBadGateway)
						return
					}
				} else {
					logger.Error("Proxy: token refresh failed (pinned)", "programID", programID, "imageID", imageID, "error", err2)
					http.Error(w, "token refresh failed", http.StatusBadGateway)
					return
				}
			}
			defer resp.Body.Close()

			buf, rerr := io.ReadAll(resp.Body)
			if rerr != nil {
				logger.Error("Proxy: read pinned body failed", "programID", programID, "imageID", imageID, "error", rerr)
				http.Error(w, "read failed", http.StatusBadGateway)
				return
			}
			if resp.StatusCode != http.StatusOK {
				logger.Warn("Proxy: SD returned non-200 for pinned", "programID", programID, "imageID", imageID, "status", resp.Status, "body", truncate(string(buf), 256))
				http.Error(w, resp.Status, resp.StatusCode)
				return
			}

			// Validate payload is an image
			ct := resp.Header.Get("Content-Type")
			if ct == "" {
				ct = http.DetectContentType(buf)
			}
			isImage := strings.HasPrefix(strings.ToLower(ct), "image/") && looksLikeImage(buf)
			if !isImage {
				bodyText := string(buf)
				if strings.Contains(bodyText, "Counter resets at 00:00Z.") {
					ref := sdErrorTime(buf)
					if ref.IsZero() {
						ref = time.Now().UTC()
					}
					until := nextUTCMidnightPlus(ref, 5)
					setGlobalPauseUntil(until, "SD quota message: Counter resets at 00:00Z.")
					retryAfter := time.Until(until)
					logger.Warn("Proxy: SD quota message during pinned fetch; pausing",
						"programID", programID, "imageID", imageID, "retry_after", retryAfter.String(), "until_utc", until, "body", truncate(bodyText, 256))
					w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
					http.Error(w, "image downloads paused until next UTC midnight window", http.StatusTooManyRequests)
					return
				}
				logger.Warn("Proxy: SD returned non-image payload for pinned; not caching",
					"programID", programID, "imageID", imageID, "content_type", ct, "body", truncate(bodyText, 256))
				http.Error(w, "Schedules Direct returned a non-image payload", http.StatusBadGateway)
				return
			}

			filePath = filepath.Join(folderImage, imageID+".jpg")
			if err := os.WriteFile(filePath, buf, 0644); err != nil {
				logger.Error("Proxy: save failed (pinned write)", "programID", programID, "imageID", imageID, "path", filePath, "error", err)
				http.Error(w, "save failed", http.StatusInternalServerError)
				return
			}
			_ = indexSet(programID, imageID)
			// Always report category (fetch metadata if missing)
			logWithMeta("Proxy: serve freshly cached (pinned)", true)
			serveFileCached(w, r, filePath)
			return
		}

		// --- LEGACY MODE: /proxy/sd/{programID} (resolver path) ---
		maxAge := time.Duration(0)
		if days := Config.Options.Images.MaxCacheAgeDays; days > 0 {
			maxAge = time.Duration(days) * 24 * time.Hour
		}
		purgeEnabled := Config.Options.Images.PurgeStale && maxAge > 0
		purgeThreshold := maxAge * 2
		purgeAfterDays := Config.Options.Images.MaxCacheAgeDays * 2
		now := time.Now()

		indexImageID := ""
		indexImagePath := ""
		indexImageExpired := false

		// 1) Try ProgramID → imageID index
		if entry, ok := indexGetEntry(programID); ok && entry.ImageID != "" {
			imgID := entry.ImageID
			indexImageID = imgID
			indexImagePath = filepath.Join(folderImage, imgID+".jpg")
			if fi, err := os.Stat(indexImagePath); err == nil && !fi.IsDir() {
				lastTouch := entry.lastRequest()
				if lastTouch.IsZero() {
					lastTouch = fi.ModTime()
				}
				purged := false
				if purgeEnabled && now.Sub(lastTouch) > purgeThreshold {
					logger.Info("Proxy: purging stale cached image (index hit)",
						"programID", programID, "imageID", imgID, "path", indexImagePath,
						"last_request_utc", lastTouch.UTC(), "purge_after_days", purgeAfterDays)
					if err := os.Remove(indexImagePath); err != nil {
						logger.Warn("Proxy: failed to remove stale cached image",
							"programID", programID, "imageID", imgID, "path", indexImagePath, "error", err)
					} else {
						if err := indexDeleteImageIDs([]string{imgID}); err != nil {
							logger.Warn("Proxy: failed to prune index for stale cached image",
								"imageID", imgID, "error", err)
						}
					}
					indexImageExpired = true
					purged = true
					indexImageID = ""
					indexImagePath = ""
				}
				if !purged {
					expired := false
					if maxAge > 0 && now.Sub(lastTouch) > maxAge {
						expired = true
						indexImageExpired = true
					}
					if !expired {
						if cat, asp, wpx, hpx, ok := lookupImageMeta(programID, imgID); ok {
							logger.Info("Proxy: serve from cache (index hit)",
								"programID", programID, "imageID", imgID, "category", cat, "aspect", asp, "w", wpx, "h", hpx, "path", indexImagePath)
						} else {
							logger.Info("Proxy: serve from cache (index hit, no meta)",
								"programID", programID, "imageID", imgID, "path", indexImagePath)
						}
						_ = indexSet(programID, imgID)
						serveFileCached(w, r, indexImagePath)
						return
					}

					// Log expiration before refreshing
					if blockGlobal {
						if cat, asp, wpx, hpx, ok := lookupImageMeta(programID, imgID); ok {
							logger.Info("Proxy: serve expired cache during global pause",
								"programID", programID, "imageID", imgID, "category", cat, "aspect", asp, "w", wpx, "h", hpx, "path", indexImagePath, "max_cache_days", Config.Options.Images.MaxCacheAgeDays)
						} else {
							logger.Info("Proxy: serve expired cache during global pause",
								"programID", programID, "imageID", imgID, "path", indexImagePath, "max_cache_days", Config.Options.Images.MaxCacheAgeDays)
						}
						_ = indexSet(programID, imgID)
						serveFileCached(w, r, indexImagePath)
						return
					}
					if _, _, _, _, ok := lookupImageMeta(programID, imgID); !ok && !blockGlobal {
						_ = ensureProgramMetadata(programID)
					}
					if cat, asp, wpx, hpx, ok := lookupImageMeta(programID, imgID); ok {
						logger.Info("Proxy: cached image expired; refreshing",
							"programID", programID, "imageID", imgID, "category", cat, "aspect", asp, "w", wpx, "h", hpx, "path", indexImagePath, "max_cache_days", Config.Options.Images.MaxCacheAgeDays)
					} else {
						logger.Info("Proxy: cached image expired; refreshing",
							"programID", programID, "imageID", imgID, "path", indexImagePath, "max_cache_days", Config.Options.Images.MaxCacheAgeDays)
					}
				}
			} else {
				indexImageExpired = true
				logger.Warn("Proxy: index stale, removing mapping", "programID", programID, "imageID", imgID)
				_ = indexDelete(programID)
			}
		}

		// 2) Resolve via metadata (or fetch-on-miss)
		chosen, ok := Cache.resolveSDImageForProgram(programID)
		if !ok || chosen.URI == "" {
			if !blockGlobal && ensureProgramMetadata(programID) {
				if ch2, ok2 := Cache.resolveSDImageForProgram(programID); ok2 && ch2.URI != "" {
					chosen = ch2
					ok = true
				}
			}
		}

		useResolved := ok && chosen.URI != ""
		if useResolved {
			logger.Info("Proxy: resolved image candidate",
				"programID", programID,
				"imageID", sdImageIDFromURI(chosen.URI),
				"category", chosen.Category, "aspect", chosen.Aspect, "w", chosen.Width, "h", chosen.Height,
				"uri", chosen.URI)
			imageID = sdImageIDFromURI(chosen.URI)
		} else {
			imageID = indexImageID
			if imageID == "" {
				if blockGlobal {
					w.Header().Set("Retry-After", fmt.Sprintf("%.0f", blockRemain.Seconds()))
					http.Error(w, "image downloads paused due to upstream limits", http.StatusTooManyRequests)
					logger.Warn("Proxy: global pause in effect; no cached image available", "programID", programID, "remaining", blockRemain)
				} else {
					logger.Warn("Proxy: no suitable image in metadata", "programID", programID)
					http.NotFound(w, r)
				}
				return
			}
			logger.Info("Proxy: using cached image id (no new metadata)", "programID", programID, "imageID", imageID)
		}

		// During a global pause, still serve from cache if the resolved image is already on disk
		// even when the programme→image index lacks an entry.
		if blockGlobal && imageID != "" {
			filePath := filepath.Join(folderImage, imageID+".jpg")
			if fi, err := os.Stat(filePath); err == nil && !fi.IsDir() {
				lastTouch := indexLastRequestForImage(imageID)
				if lastTouch.IsZero() {
					lastTouch = fi.ModTime()
				}
				if maxAge > 0 && now.Sub(lastTouch) > maxAge {
					logger.Info("Proxy: serve expired cache during global pause (resolved)",
						"programID", programID, "imageID", imageID, "path", filePath,
						"max_cache_days", Config.Options.Images.MaxCacheAgeDays)
				} else {
					logger.Info("Proxy: serve from cache during global pause (resolved)",
						"programID", programID, "imageID", imageID, "path", filePath)
				}
				_ = indexSet(programID, imageID)
				serveFileCached(w, r, filePath)
				return
			}
		}

		if blockGlobal {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", blockRemain.Seconds()))
			http.Error(w, "image downloads paused due to upstream limits", http.StatusTooManyRequests)
			logger.Warn("Proxy: global pause in effect; denying image download", "programID", programID, "remaining", blockRemain)
			return
		}

		// 3) Serve from disk if present (and update index) provided it hasn't expired
		filePath := filepath.Join(folderImage, imageID+".jpg")
		if fi, err := os.Stat(filePath); err == nil && !fi.IsDir() {
			lastTouch := indexLastRequestForImage(imageID)
			if lastTouch.IsZero() {
				lastTouch = fi.ModTime()
			}
			purged := false
			if purgeEnabled && now.Sub(lastTouch) > purgeThreshold {
				logger.Info("Proxy: purging stale cached image",
					"programID", programID, "imageID", imageID, "path", filePath,
					"last_request_utc", lastTouch.UTC(), "purge_after_days", purgeAfterDays)
				if err := os.Remove(filePath); err != nil {
					logger.Warn("Proxy: failed to remove stale cached image",
						"programID", programID, "imageID", imageID, "path", filePath, "error", err)
				} else {
					if err := indexDeleteImageIDs([]string{imageID}); err != nil {
						logger.Warn("Proxy: failed to prune index for stale cached image", "imageID", imageID, "error", err)
					}
				}
				purged = true
			}
			if !purged {
				expired := false
				if maxAge > 0 && now.Sub(lastTouch) > maxAge {
					expired = true
				}
				if !expired {
					if cat, asp, wpx, hpx, ok := lookupImageMeta(programID, imageID); ok {
						logger.Info("Proxy: serve from cache (by imageID)",
							"programID", programID, "imageID", imageID, "category", cat, "aspect", asp, "w", wpx, "h", hpx, "path", filePath)
					} else {
						logger.Info("Proxy: serve from cache (by imageID, no meta)",
							"programID", programID, "imageID", imageID, "path", filePath)
					}
					_ = indexSet(programID, imageID)
					serveFileCached(w, r, filePath)
					return
				}
				if !(indexImageExpired && indexImageID == imageID) {
					logger.Info("Proxy: cached candidate expired; refreshing", "programID", programID, "imageID", imageID, "path", filePath)
				}
			}
		}

		// Token (only when a download is required)
		token, err := getSDToken()
		if err != nil {
			logger.Error("Proxy: token error before fetch", "programID", programID, "error", err)
			http.Error(w, "token error", http.StatusBadGateway)
			return
		}

		// IMPORTANT: do NOT redeclare imageID; reuse existing variable to avoid shadowing
		var imageURL string
		imageURL = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s.jpg?token=%s", imageID, token)

		// 4) Download (retry once on 401)
		logger.Info("Proxy: downloading image from SD", "programID", programID, "imageID", imageID, "url", imageURL)

		client := &http.Client{Timeout: 20 * time.Second}
		fetch := func(url string) (*http.Response, error) {
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("User-Agent", userAgent())
			return client.Do(req)
		}

		resp, err := fetch(imageURL)
		if err != nil {
			logger.Error("Proxy: fetch failed", "programID", programID, "imageID", imageID, "error", err)
			http.Error(w, "fetch failed", http.StatusBadGateway)
			return
		}
		if resp.StatusCode == http.StatusUnauthorized {
			logger.Warn("Proxy: SD token unauthorized, refreshing", "programID", programID)
			resp.Body.Close()
			if token2, err2 := forceRefreshToken(); err2 == nil {
				imageURL = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s.jpg?token=%s", imageID, token2)
				resp, err = fetch(imageURL)
				if err != nil {
					logger.Error("Proxy: fetch retry failed", "programID", programID, "imageID", imageID, "error", err)
					http.Error(w, "fetch retry failed", http.StatusBadGateway)
					return
				}
			} else {
				logger.Error("Proxy: token refresh failed", "programID", programID, "error", err2)
				http.Error(w, "token refresh failed", http.StatusBadGateway)
				return
			}
		}
		defer resp.Body.Close()

		// Read entire body for validation
		buf, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			logger.Error("Proxy: read body failed", "programID", programID, "imageID", imageID, "error", rerr)
			http.Error(w, "read failed", http.StatusBadGateway)
			return
		}

		// Non-200 -> do not save
		if resp.StatusCode != http.StatusOK {
			logger.Warn("Proxy: SD returned non-200", "programID", programID, "imageID", imageID, "status", resp.Status, "body", truncate(string(buf), 256))
			http.Error(w, resp.Status, resp.StatusCode)
			return
		}

		// Validate payload is an image; SD can return JSON with 200 OK.
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = http.DetectContentType(buf)
		}
		isImage := strings.HasPrefix(strings.ToLower(ct), "image/") && looksLikeImage(buf)
		if !isImage {
			// If body contains quota message, pause globally until next UTC midnight + 5 minutes.
			bodyText := string(buf)
			if strings.Contains(bodyText, "Counter resets at 00:00Z.") {
				ref := sdErrorTime(buf)
				if ref.IsZero() {
					ref = time.Now().UTC()
				}
				until := nextUTCMidnightPlus(ref, 5)
				setGlobalPauseUntil(until, "SD quota message: Counter resets at 00:00Z.")
				retryAfter := time.Until(until)

				logger.Warn("Proxy: SD returned quota message; pausing all image downloads",
					"programID", programID, "imageID", imageID,
					"retry_after", retryAfter.String(), "until_utc", until, "body", truncate(bodyText, 256))

				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
				http.Error(w, "image downloads paused until next UTC midnight window", http.StatusTooManyRequests)
				return
			}

			// Generic non-image payload — do not save; return 502
			logger.Warn("Proxy: SD returned non-image payload; not caching",
				"programID", programID, "imageID", imageID, "content_type", ct, "body", truncate(bodyText, 256))
			http.Error(w, "Schedules Direct returned a non-image payload", http.StatusBadGateway)
			return
		}

		// Save to disk
		if err := os.WriteFile(filePath, buf, 0644); err != nil {
			logger.Error("Proxy: save failed (write)", "programID", programID, "imageID", imageID, "path", filePath, "error", err)
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		logger.Info("Proxy: saved image", "programID", programID, "imageID", imageID, "path", filePath)

		// Update index and serve (log with category if possible)
		_ = indexSet(programID, imageID)
		if _, _, _, _, ok := lookupImageMeta(programID, imageID); !ok {
			_ = ensureProgramMetadata(programID)
		}
		if cat, asp, wpx, hpx, ok := lookupImageMeta(programID, imageID); ok {
			logger.Info("Proxy: serve freshly cached",
				"programID", programID, "imageID", imageID, "category", cat, "aspect", asp, "w", wpx, "h", hpx, "path", filePath)
		} else {
			logger.Info("Proxy: serve freshly cached (no meta)",
				"programID", programID, "imageID", imageID, "path", filePath)
		}
		serveFileCached(w, r, filePath)
	})

	// Static server
	fs := http.FileServer(http.Dir(dir))
	mux.Handle("/", fs)

	logger.Info("Starting server", "address", "http://"+Config.Server.Address+":"+port, "serving", filepath.Clean(dir))
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("Server failed to start", "error", err)
	}
}

func purgeStalePosterFiles(dir string, cacheDays int) (int, error) {
	if cacheDays <= 0 {
		return 0, nil
	}

	maxAge := time.Duration(cacheDays) * 24 * time.Hour
	purgeThreshold := maxAge * 2
	if purgeThreshold <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-purgeThreshold)
	purgeAfterDays := cacheDays * 2
	removedIDs := make([]string, 0)
	removedCount := 0
	var firstErr error

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jpg") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		imageID := strings.TrimSuffix(name, filepath.Ext(name))
		if isOverrideImageID(imageID) {
			continue
		}
		lastTouch := indexLastRequestForImage(imageID)
		if lastTouch.IsZero() {
			lastTouch = info.ModTime()
		}
		if !lastTouch.Before(cutoff) {
			continue
		}

		fullPath := filepath.Join(dir, name)
		if err := os.Remove(fullPath); err != nil {
			logger.Warn("Proxy: failed to remove stale cached poster during startup purge", "path", fullPath, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		removedCount++
		if imageID != "" {
			removedIDs = append(removedIDs, imageID)
		}
		logger.Info("Proxy: purged stale cached poster",
			"path", fullPath, "last_request_utc", lastTouch.UTC(), "purge_after_days", purgeAfterDays)
	}

	if len(removedIDs) > 0 {
		if err := indexDeleteImageIDs(removedIDs); err != nil {
			logger.Warn("Proxy: failed to prune index for purged posters", "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return removedCount, firstErr
}

// Serve a local file with strong cache headers
func serveFileCached(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	modTime := fi.ModTime()
	now := time.Now()
	_ = os.Chtimes(path, now, modTime)

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))
	http.ServeContent(w, r, fi.Name(), modTime, f)
}

// looksLikeImage does a minimal magic check so we don't save JSON/HTML as .jpg
func looksLikeImage(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	// JPEG: FF D8 FF
	if b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return true
	}
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(b) >= 8 && string(b[:8]) == string(png) {
		return true
	}
	// WebP: "RIFF"...."WEBP"
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return true
	}
	// Fallback: sniff
	typ := http.DetectContentType(b)
	return strings.HasPrefix(strings.ToLower(typ), "image/")
}

// truncate utility for logging
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
