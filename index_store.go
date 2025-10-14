package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ProgramID -> imageID persistent index used by the proxy to serve cached files
// even if metadata for a ProgramID isn't loaded yet.
//
// The index is stored in a sidecar JSON file next to your Cache file, e.g.:
//   /app/config_cache.imgindex.json

var (
	indexOnce   sync.Once
	indexMu     sync.RWMutex
	indexMap    map[string]string
	indexLoaded bool
	indexPathV  string
)

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
		indexMap = map[string]string{}
		// Ensure directory exists
		if dir := filepath.Dir(indexPathV); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
		// Load if present
		if data, err := os.ReadFile(indexPathV); err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &indexMap)
		}
		indexLoaded = true
	})
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
	if !indexLoaded {
		indexInit()
	}
	indexMu.RLock()
	defer indexMu.RUnlock()
	id, ok := indexMap[programID]
	return id, ok
}

func indexSet(programID, imageID string) error {
	if !indexLoaded {
		indexInit()
	}
	indexMu.Lock()
	indexMap[programID] = imageID
	indexMu.Unlock()
	return indexSave()
}

func indexDelete(programID string) error {
	if !indexLoaded {
		indexInit()
	}
	indexMu.Lock()
	delete(indexMap, programID)
	indexMu.Unlock()
	return indexSave()
}
