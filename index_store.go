package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProgramID -> imageID persistent index used by the proxy to serve cached files
// even if metadata for a ProgramID isn't loaded yet.
//
// The index is stored in a sidecar JSON file next to your Cache file, e.g.:
//   /app/config_cache.imgindex.json

type indexEntry struct {
	ImageID         string `json:"imageID"`
	LastRequestUnix int64  `json:"lastRequestUnix,omitempty"`
}

var (
	indexOnce          sync.Once
	indexMu            sync.RWMutex
	indexMap           map[string]indexEntry
	indexImageRequests map[string]int64
	indexLoaded        bool
	indexPathV         string

	overridesOnce     sync.Once
	overridesPath     string
	overridesEnabled  bool
	overrideTitleToID map[string]string
	overrideImageIDs  map[string]struct{}
)

func (e indexEntry) lastRequest() time.Time {
	if e.LastRequestUnix <= 0 {
		return time.Time{}
	}
	return time.Unix(e.LastRequestUnix, 0).UTC()
}

func indexFilePath() string {
	// Sidecar next to the cache file
	p := Config.Files.Cache
	if p == "" {
		// Fallback default within container
		return "/app/config_cache.imgindex.json"
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	return base + ".imgindex.json"
}

func indexInit() {
	indexOnce.Do(func() {
		indexPathV = indexFilePath()
		indexMap = map[string]indexEntry{}
		indexImageRequests = map[string]int64{}
		// Ensure directory exists
		if dir := filepath.Dir(indexPathV); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
		// Load if present
		if data, err := os.ReadFile(indexPathV); err == nil && len(data) > 0 {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(data, &raw); err == nil {
				for programID, blob := range raw {
					var entry indexEntry
					if err := json.Unmarshal(blob, &entry); err == nil && entry.ImageID != "" {
						indexMap[programID] = entry
						if entry.LastRequestUnix > 0 {
							if prev, ok := indexImageRequests[entry.ImageID]; !ok || entry.LastRequestUnix > prev {
								indexImageRequests[entry.ImageID] = entry.LastRequestUnix
							}
						}
						continue
					}
					var imageID string
					if err := json.Unmarshal(blob, &imageID); err == nil && imageID != "" {
						entry = indexEntry{ImageID: imageID}
						indexMap[programID] = entry
					}
				}
			}
		}
		indexLoaded = true
	})
}

func overridesFilePath() string {
	return filepath.Join(filepath.Dir(indexFilePath()), "overrides.txt")
}

func overridesInit() {
	overridesOnce.Do(func() {
		overridesPath = overridesFilePath()
		overrideTitleToID = map[string]string{}
		overrideImageIDs = map[string]struct{}{}

		data, err := os.ReadFile(overridesPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logger.Warn("Overrides: failed to read overrides file", "path", overridesPath, "error", err)
			}
			return
		}

		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			r := csv.NewReader(strings.NewReader(line))
			r.TrimLeadingSpace = true
			record, err := r.Read()
			if err != nil {
				logger.Warn("Overrides: unable to parse line", "path", overridesPath, "line", lineNo, "error", err)
				continue
			}
			if len(record) != 2 {
				logger.Warn("Overrides: invalid record length", "path", overridesPath, "line", lineNo, "fields", len(record))
				continue
			}

			title := strings.TrimSpace(record[0])
			imageID := strings.TrimSpace(record[1])
			if title == "" || imageID == "" {
				logger.Warn("Overrides: empty title or imageID", "path", overridesPath, "line", lineNo)
				continue
			}

			normTitle := strings.ToLower(title)

			overrideTitleToID[normTitle] = imageID
			overrideImageIDs[imageID] = struct{}{}
		}

		if len(overrideTitleToID) > 0 {
			overridesEnabled = true
			logger.Info("Overrides: loaded image overrides", "count", len(overrideTitleToID), "path", overridesPath)
		}
	})
}

func overrideImageForTitle(title string) (string, bool) {
	overridesInit()
	if !overridesEnabled {
		return "", false
	}
	normTitle := strings.ToLower(strings.TrimSpace(title))
	imageID, ok := overrideTitleToID[normTitle]
	return imageID, ok
}

func overrideImageForProgram(programID string) (string, bool) {
	overridesInit()
	if !overridesEnabled {
		return "", false
	}

	p, ok := Cache.Program[programID]
	if !ok {
		return "", false
	}

	for _, t := range p.Titles {
		if imageID, ok := overrideImageForTitle(t.Title120); ok {
			return imageID, true
		}
	}

	return "", false
}

// overrideImageForProgramOrTitle attempts to resolve an override for a program.
// It first checks the cached program metadata (for Title120 matches) and, if that
// fails, it will also attempt to match the provided fallbackTitle (e.g. from a
// schedule entry) so overrides still work when program metadata is missing.
func overrideImageForProgramOrTitle(programID, fallbackTitle string) (string, bool) {
	if id, ok := overrideImageForProgram(programID); ok {
		return id, true
	}
	return overrideImageForTitle(fallbackTitle)
}

func isOverrideImageID(imageID string) bool {
	overridesInit()
	if !overridesEnabled || imageID == "" {
		return false
	}
	_, ok := overrideImageIDs[imageID]
	return ok
}

func indexSave() error {
	if !indexLoaded {
		indexInit()
	}
	indexMu.RLock()
	defer indexMu.RUnlock()
	blob, err := json.MarshalIndent(indexMap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPathV, blob, 0644)
}

func indexGet(programID string) (string, bool) {
	entry, ok := indexGetEntry(programID)
	if !ok || entry.ImageID == "" {
		return "", false
	}
	return entry.ImageID, true
}

func indexGetEntry(programID string) (indexEntry, bool) {
	if !indexLoaded {
		indexInit()
	}
	indexMu.RLock()
	defer indexMu.RUnlock()
	entry, ok := indexMap[programID]
	return entry, ok
}

func indexSet(programID, imageID string) error {
	if !indexLoaded {
		indexInit()
	}
	if imageID == "" {
		return nil
	}
	nowUnix := time.Now().Unix()

	var recalc []string

	indexMu.Lock()
	old := indexMap[programID]
	indexMap[programID] = indexEntry{ImageID: imageID, LastRequestUnix: nowUnix}
	if indexImageRequests == nil {
		indexImageRequests = map[string]int64{}
	}
	indexImageRequests[imageID] = nowUnix
	if old.ImageID != "" && old.ImageID != imageID {
		recalc = append(recalc, old.ImageID)
	}
	indexMu.Unlock()

	if len(recalc) > 0 {
		indexRecalculateImageRequests(recalc)
	}

	return indexSave()
}

func indexDelete(programID string) error {
	if !indexLoaded {
		indexInit()
	}
	var recalc []string
	indexMu.Lock()
	if entry, ok := indexMap[programID]; ok {
		if entry.ImageID != "" {
			recalc = append(recalc, entry.ImageID)
		}
		delete(indexMap, programID)
	}
	indexMu.Unlock()

	if len(recalc) > 0 {
		indexRecalculateImageRequests(recalc)
	}

	return indexSave()
}

func indexDeleteImageIDs(imageIDs []string) error {
	if !indexLoaded {
		indexInit()
	}
	if len(imageIDs) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(imageIDs))
	for _, id := range imageIDs {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	indexMu.Lock()
	changed := false
	for programID, entry := range indexMap {
		if _, ok := set[entry.ImageID]; ok {
			delete(indexMap, programID)
			changed = true
		}
	}
	if len(indexImageRequests) > 0 {
		for id := range set {
			delete(indexImageRequests, id)
		}
	}
	indexMu.Unlock()
	if changed {
		return indexSave()
	}
	return nil
}

func indexLastRequestForImage(imageID string) time.Time {
	if !indexLoaded {
		indexInit()
	}
	if imageID == "" {
		return time.Time{}
	}
	indexMu.RLock()
	defer indexMu.RUnlock()
	if ts, ok := indexImageRequests[imageID]; ok && ts > 0 {
		return time.Unix(ts, 0).UTC()
	}
	return time.Time{}
}

func indexRecalculateImageRequests(imageIDs []string) {
	if len(imageIDs) == 0 {
		return
	}
	if !indexLoaded {
		indexInit()
	}
	indexMu.Lock()
	defer indexMu.Unlock()
	if indexImageRequests == nil {
		indexImageRequests = map[string]int64{}
	}
	for _, imageID := range imageIDs {
		if imageID == "" {
			continue
		}
		var latest int64
		for _, entry := range indexMap {
			if entry.ImageID != imageID {
				continue
			}
			if entry.LastRequestUnix > latest {
				latest = entry.LastRequestUnix
			}
		}
		if latest == 0 {
			delete(indexImageRequests, imageID)
		} else {
			indexImageRequests[imageID] = latest
		}
	}
}
