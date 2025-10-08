package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// in-memory index (persisted to sidecar JSON file)
var (
	indexOnce   sync.Once
	indexMu     sync.RWMutex
	indexMap    map[string]string
	indexLoaded bool
	indexPathV  string
)

func indexFilePath() string {
	// Sidecar next to the cache file, e.g. /app/config_cache.imgindex.json
	p := Config.Files.Cache
	if p == "" {
		// Fall back to a safe default in /app (matches typical container path)
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
		// Try load existing
		data, err := os.ReadFile(indexPathV)
		if err == nil && len(data) > 0 {
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
