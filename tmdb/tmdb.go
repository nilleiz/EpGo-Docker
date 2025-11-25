package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	tmdbURL           = "https://api.themoviedb.org/3/%s"
	tmdbImageBase     = "https://image.tmdb.org/t/p"
	defaultPosterSize = "w500" // sharper default for Plex posters
	httpTimeout       = 8 * time.Second
	userAgent         = "EpGo-Docker (+https://github.com/nilleiz/EpGo-Docker)"
)

var (
	// fetchLogOnce ensures we only log the long-running TMDb fetch notice once.
	fetchLogOnce sync.Once

	httpClient = &http.Client{Timeout: httpTimeout}

	// caches holds in-memory copies of TMDb cache files keyed by filename.
	caches sync.Map
)

// isV4Token detects whether the provided TMDb credential looks like a v4 read
// access token (JWT). v3 keys are short (32 chars) and should be sent as a
// query parameter, while v4 tokens are long JWT strings that belong in the
// Authorization header.
func isV4Token(key string) bool {
	if key == "" {
		return false
	}

	// JWTs start with eyJ... and contain dots; any substantially long key should
	// also be treated as v4-style to avoid misusing short v3 API keys as Bearer
	// tokens (which TMDb rejects by closing the connection).
	return strings.HasPrefix(key, "eyJ") || strings.Contains(key, ".") || len(key) > 50
}

// posterURL builds a full TMDb image URL from a path and size.
// If size is empty, defaultPosterSize is used.
func posterURL(path, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = defaultPosterSize
	}
	path = strings.TrimPrefix(path, "/")
	return fmt.Sprintf("%s/%s/%s", tmdbImageBase, size, path)
}

type searchResponse struct {
	Results []struct {
		PosterPath string `json:"poster_path"`
	} `json:"results"`
}

// sanitizeQuery removes invisible cruft and trailing slashes/backslashes
func sanitizeQuery(q string) string {
	// remove those little badges some EPGs add
	q = strings.ReplaceAll(q, "ᴺᵉʷ", "")
	q = strings.ReplaceAll(q, "ᴸᶦᵛᵉ", "")
	q = strings.TrimSpace(q)
	q = strings.Trim(q, "\\/ \t\r\n")
	spaceRe := regexp.MustCompile(`\s+`)
	q = spaceRe.ReplaceAllString(q, " ")
	return q
}

// depunctuate produces a softer variant for a second pass, e.g. "F1 Pressekonferenz"
func depunctuate(q string) string {
	re := regexp.MustCompile(`[^0-9A-Za-zÀ-ÖØ-öø-ÿ\s]`)
	q = re.ReplaceAllString(q, " ")
	spaceRe := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(spaceRe.ReplaceAllString(q, " "))
}

// SearchItem looks up a poster and returns a full TMDb image URL (w500 by default).
// It caches only the poster "path" (not the full URL) so we can change sizes later.
func SearchItem(logger *slog.Logger, searchTerm, mediaType, tmdbApiKey, imageCacheFile string) (string, error) {
	// 1) Clean search term
	origTerm := sanitizeQuery(searchTerm)
	if origTerm == "" {
		return "", nil
	}

	// 2) Endpoint by media type
	var tmdbUrl string
	switch mediaType {
	case "SH", "EP":
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/tv")
		mediaType = "SH"
	case "MV":
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/movie")
		mediaType = "MV"
	default:
		// prefer tv for ambiguous EPG items
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/tv")
		mediaType = "SH"
	}

	// 3) Cache hit?
	cache, err := getCache(imageCacheFile)
	if err != nil {
		return "", fmt.Errorf("tmdb: error preparing cache: %w", err)
	}

	if cachedPath, err := cache.getImageURL(origTerm + "-" + mediaType); err != nil {
		return "", fmt.Errorf("tmdb: error checking cache: %w", err)
	} else if cachedPath != "" {
		return posterURL(cachedPath, ""), nil // default w500
	}
	fetchLogOnce.Do(func() {
		logger.Info("TMDb: fetching posters; this can take a while while the cache is primed")
	})

	// 4) HTTP client and request scaffold
	buildReq := func(qStr, lang string) (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, tmdbUrl, nil)
		if err != nil {
			return nil, err
		}
		q := req.URL.Query()
		q.Add("query", qStr)
		if lang != "" {
			q.Add("language", lang)
		}
		q.Add("page", "1")
		q.Add("include_adult", "false")

		if !isV4Token(tmdbApiKey) {
			// v3 keys must be sent as query params; using them as Bearer tokens will
			// cause TMDb to close the connection, leading to unexpected EOF errors.
			q.Add("api_key", tmdbApiKey)
		}
		req.URL.RawQuery = q.Encode()

		if isV4Token(tmdbApiKey) {
			// TMDb v4 Read Access Token (JWT) via Bearer
			req.Header.Set("Authorization", "Bearer "+tmdbApiKey)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", userAgent)
		return req, nil
	}

	// 5) Try German first, then English
	langs := []string{"de-DE", "de", "en-US", "en"}
	tryTerms := []string{origTerm}

	// Add a depunctuated fallback if it differs
	if soft := depunctuate(origTerm); soft != "" && !strings.EqualFold(soft, origTerm) {
		tryTerms = append(tryTerms, soft)
	}

	var lastHTTPStatus int
	var lastErr error
	var posterPath string

	for _, term := range tryTerms {
		for _, lang := range langs {
			req, err := buildReq(term, lang)
			if err != nil {
				lastErr = err
				continue
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				// network error: remember, but continue
				lastErr = err
				continue
			}
			func() {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					// keep a hint for diagnostics; don't abort overall flow
					lastHTTPStatus = resp.StatusCode
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
					logger.Warn("tmdb non-200", "status", resp.Status, "lang", lang, "term", term, "body", string(b))
					return
				}

				var r searchResponse
				if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
					lastErr = fmt.Errorf("tmdb decode error: %w", err)
					return
				}
				if len(r.Results) > 0 && r.Results[0].PosterPath != "" {
					posterPath = r.Results[0].PosterPath
				}
			}()

			if posterPath != "" {
				break
			}
		}
		if posterPath != "" {
			break
		}
	}

	// 6) If we still have nothing, return gracefully (no poster is not an error)
	if posterPath == "" {
		// if there was a hard error and *also* no result, surface the error to help debugging
		if lastErr != nil {
			return "", fmt.Errorf("tmdb lookup failed: %w", lastErr)
		}
		// non-200 without a concrete error: keep quiet; upstream can decide how to log
		if lastHTTPStatus != 0 && lastHTTPStatus != http.StatusOK {
			return "", fmt.Errorf("tmdb returned HTTP %d with no usable results", lastHTTPStatus)
		}
		return "", nil
	}

	// 7) Cache the *path* (not full URL)
	if err := cache.addImageToCache(origTerm+"-"+mediaType, posterPath); err != nil {
		logger.Error("tmdb: error adding to cache", "error", err)
	}

	// 8) Return full URL with default w500 size
	return posterURL(posterPath, ""), nil
}

type imageCache struct {
	filePath string
	entries  map[string]string
	mu       sync.RWMutex
	loaded   bool
	saveMu   sync.Mutex
}

func getCache(cacheFile string) (*imageCache, error) {
	cachePtr, _ := caches.LoadOrStore(cacheFile, &imageCache{filePath: cacheFile, entries: make(map[string]string)})
	cache := cachePtr.(*imageCache)

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.loaded {
		return cache, nil
	}

	f, err := os.Open(cache.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			cache.loaded = true
			return cache, nil
		}
		return nil, fmt.Errorf("error opening image cache file: %w", err)
	}
	defer f.Close()

	var diskCache []map[string]string
	dec := json.NewDecoder(f)
	if err := dec.Decode(&diskCache); err != nil && err != io.EOF {
		return nil, fmt.Errorf("error decoding JSON: %w", err)
	}

	for _, entry := range diskCache {
		if name, ok := entry["name"]; ok {
			if url, ok := entry["url"]; ok {
				cache.entries[name] = url
			}
		}
	}
	cache.loaded = true

	return cache, nil
}

func (c *imageCache) getImageURL(name string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[name], nil
}

func (c *imageCache) addImageToCache(name, url string) error {
	c.mu.Lock()
	if existing, ok := c.entries[name]; ok && existing == url {
		c.mu.Unlock()
		return nil
	}
	c.entries[name] = url

	// Take a snapshot so we can write to disk without holding the primary lock
	snapshot := make(map[string]string, len(c.entries))
	for n, u := range c.entries {
		snapshot[n] = u
	}
	c.mu.Unlock()

	c.saveMu.Lock()
	defer c.saveMu.Unlock()

	f, err := os.OpenFile(c.filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error opening image cache file: %w", err)
	}
	defer f.Close()

	// Re-encode entire map to keep file consistent with in-memory state
	cacheSlice := make([]map[string]string, 0, len(snapshot))
	for n, u := range snapshot {
		cacheSlice = append(cacheSlice, map[string]string{"name": n, "url": u})
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("error seeking to beginning of file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("error truncating file: %w", err)
	}

	enc := json.NewEncoder(f)
	if err := enc.Encode(cacheSlice); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	return nil
}
