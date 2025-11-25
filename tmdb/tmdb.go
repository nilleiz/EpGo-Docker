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

	cacheMu       sync.RWMutex
	cacheEntries  map[string]string
	cacheFilePath string
)

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
	if cachedPath, err := getCachedImage(origTerm+"-"+mediaType, imageCacheFile); err != nil {
		return "", fmt.Errorf("tmdb: error checking cache: %w", err)
	} else if cachedPath != "" {
		return posterURL(cachedPath, ""), nil // default w500
	}
	fetchLogOnce.Do(func() {
		logger.Info("TMDb: fetching posters; this can take a while while the cache is primed")
	})

	// 4) HTTP client and request scaffold
	client := &http.Client{Timeout: httpTimeout}
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
		req.URL.RawQuery = q.Encode()

		// TMDb v4 Read Access Token (JWT) via Bearer
		req.Header.Set("Authorization", "Bearer "+tmdbApiKey)
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

			resp, err := client.Do(req)
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
	if err := addImageToCache(origTerm+"-"+mediaType, posterPath, imageCacheFile); err != nil {
		logger.Error("tmdb: error adding to cache", "error", err)
	}

	// 8) Return full URL with default w500 size
	return posterURL(posterPath, ""), nil
}

func ensureCache(cacheFile string) error {
	cacheMu.RLock()
	ready := cacheEntries != nil && cacheFilePath == cacheFile
	cacheMu.RUnlock()
	if ready {
		return nil
	}

	entries := map[string]string{}
	if f, err := os.Open(cacheFile); err == nil {
		defer f.Close()
		var cache []map[string]string
		dec := json.NewDecoder(f)
		if err := dec.Decode(&cache); err != nil && err != io.EOF {
			return fmt.Errorf("error decoding JSON: %w", err)
		}
		for _, entry := range cache {
			name := entry["name"]
			url := entry["url"]
			if name != "" && url != "" {
				entries[name] = url
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error opening image cache file: %w", err)
	}

	cacheMu.Lock()
	cacheEntries = entries
	cacheFilePath = cacheFile
	cacheMu.Unlock()
	return nil
}

func getCachedImage(name, cacheFile string) (string, error) {
	if err := ensureCache(cacheFile); err != nil {
		return "", err
	}
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return cacheEntries[name], nil
}

func addImageToCache(name, url, cacheFile string) error {
	if name == "" || url == "" {
		return nil
	}
	if err := ensureCache(cacheFile); err != nil {
		return err
	}

	cacheMu.Lock()
	if cacheEntries == nil {
		cacheEntries = map[string]string{}
	}
	if existing, ok := cacheEntries[name]; ok && existing == url {
		cacheMu.Unlock()
		return nil
	}
	cacheEntries[name] = url
	entries := make([]map[string]string, 0, len(cacheEntries))
	for n, u := range cacheEntries {
		entries = append(entries, map[string]string{"name": n, "url": u})
	}
	cacheMu.Unlock()

	f, err := os.OpenFile(cacheFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error opening image cache file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(entries); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	return nil
}
