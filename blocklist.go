package main

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	blockOnce    sync.Once
	blockMu      sync.RWMutex
	blockPathV   string
	blockEntries map[string]struct{}
	blockModTime time.Time
	blockActive  bool
)

func blockListFilePath() string {
	p := Config.Files.Cache
	if p == "" {
		return "/app/config_cache.imgblock.txt"
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	return base + ".imgblock.txt"
}

func loadBlockList() {
	blockOnce.Do(func() {
		blockPathV = blockListFilePath()
	})

	fi, err := os.Stat(blockPathV)
	if err != nil {
		blockMu.Lock()
		blockActive = false
		blockEntries = nil
		blockModTime = time.Time{}
		blockMu.Unlock()
		return
	}

	modTime := fi.ModTime()
	blockMu.RLock()
	if blockActive && !blockModTime.IsZero() && blockModTime.Equal(modTime) {
		blockMu.RUnlock()
		return
	}
	blockMu.RUnlock()

	data, err := os.ReadFile(blockPathV)
	if err != nil {
		logger.Warn("Proxy: unable to read blocklist file", "path", blockPathV, "error", err)
		blockMu.Lock()
		blockActive = false
		blockEntries = nil
		blockModTime = time.Time{}
		blockMu.Unlock()
		return
	}

	entries := make(map[string]struct{})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		entries[line] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("Proxy: error scanning blocklist file", "path", blockPathV, "error", err)
		blockMu.Lock()
		blockActive = false
		blockEntries = nil
		blockModTime = time.Time{}
		blockMu.Unlock()
		return
	}

	blockMu.Lock()
	blockEntries = entries
	blockActive = true
	blockModTime = modTime
	blockMu.Unlock()

	logger.Info("Proxy: loaded image blocklist", "path", blockPathV, "entries", len(entries))
}

func isImageBlocked(imageID string) bool {
	if imageID == "" {
		return false
	}
	loadBlockList()
	blockMu.RLock()
	defer blockMu.RUnlock()
	if !blockActive || len(blockEntries) == 0 {
		return false
	}
	_, ok := blockEntries[imageID]
	return ok
}

func purgeBlockedImage(imageID, folderImage string) {
	if imageID == "" {
		return
	}
	if folderImage == "" {
		folderImage = "images"
	}

	filePath := filepath.Join(folderImage, imageID+".jpg")
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Warn("Proxy: failed to remove blocked cached poster", "imageID", imageID, "path", filePath, "error", err)
	} else if err == nil {
		logger.Info("Proxy: removed blocked cached poster", "imageID", imageID, "path", filePath)
	}

	if err := indexDeleteImageIDs([]string{imageID}); err != nil {
		logger.Warn("Proxy: failed to prune index for blocked poster", "imageID", imageID, "error", err)
	}
}
