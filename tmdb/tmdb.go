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
	"time"
)

const (
	tmdbURL           = "https://api.themoviedb.org/3/%s"
	tmdbImageBase     = "https://image.tmdb.org/t/p"
	defaultPosterSize = "w500" // sharper default for Plex posters
	httpTimeout       = 8 * time.Second
	userAgent         = "EpGo-Docker (+https://github.com/nilleiz/EpGo-Docker)"
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
	if cachedPath, err := getImageURL(origTerm+"-"+mediaType, imageCacheFile); err != nil {
		return "", fmt.Errorf("tmdb: error checking cache: %w", err)
	} else if cachedPath != "" {
		return posterURL(cachedPath, ""), nil // default w500
	}

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

func getImageURL(name, cacheFile string) (string, error) {
	f, err := os.Open(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("error opening image cache file: %w", err)
	}
	defer f.Close()

	var cache []map[string]string
	dec := json.NewDecoder(f)
	if err := dec.Decode(&cache); err != nil {
		if err != io.EOF {
			return "", fmt.Errorf("error decoding JSON: %w", err)
		}
	}

	for _, entry := range cache {
		if entry["name"] == name {
			return entry["url"], nil
		}
	}
	return "", nil
}

func addImageToCache(name, url, cacheFile string) error {
	entry := map[string]string{
		"name": name,
		"url":  url,
	}

	f, err := os.OpenFile(cacheFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error opening image cache file: %w", err)
	}
	defer f.Close()

	var cache []map[string]string
	dec := json.NewDecoder(f)
	if err := dec.Decode(&cache); err != nil && err != io.EOF {
		return fmt.Errorf("error decoding JSON: %w", err)
	}

	// Avoid duplicates
	for _, e := range cache {
		if e["name"] == name && e["url"] == url {
			return nil
		}
	}
	cache = append(cache, entry)

	// Seek to start & truncate to avoid trailing bytes from previous content
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("error seeking to beginning of file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("error truncating file: %w", err)
	}

	enc := json.NewEncoder(f)
	if err := enc.Encode(cache); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	return nil
}
