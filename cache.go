package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var mu sync.Mutex

// Open loads the cache file from disk into memory.
func (c *cache) Open() (err error) {
	if FileExists(Config.Files.Cache) {
		c.Lock()
		defer c.Unlock()

		jsonFile, err := os.Open(Config.Files.Cache)
		if err != nil {
			logger.Error("unable to open the cache", "error", err)
			return err
		}
		defer jsonFile.Close()

		byteValue, _ := io.ReadAll(jsonFile)
		err = json.Unmarshal(byteValue, &Cache)
		if err != nil {
			logger.Error("unable to unmarshal the cache", "error", err)
			return err
		}
	}
	return
}

// Save writes the in-memory cache back to disk.
func (c *cache) Save() (err error) {
	c.Lock()
	defer c.Unlock()

	file, err := json.MarshalIndent(Cache, "", "  ")
	if err != nil {
		logger.Error("unable to marshal the cache", "error", err)
		return err
	}

	err = os.WriteFile(Config.Files.Cache, file, 0644)
	if err != nil {
		logger.Error("unable to write the cache", "error", err)
		return err
	}
	return
}

// GetIcon returns an Icon slice for a program. In ProxyMode we do NOT pre-download.
// When not in ProxyMode but Download=true, we pre-download to local storage.
func (c *cache) GetIcon(id string) (i []Icon) {
	if m, ok := c.Metadata[id]; ok {
		// 1) Filter by configured SD aspect string ("16x9", "2x3", "4x3"). "all" or empty = no filter.
		desired := strings.TrimSpace(Config.Options.Images.PosterAspect)
		candidates := make([]Data, 0, len(m.Data))
		for _, d := range m.Data {
			if desired == "" || strings.EqualFold(desired, "all") || strings.EqualFold(d.Aspect, desired) {
				candidates = append(candidates, d)
			}
		}
		if len(candidates) == 0 {
			// No exact aspect match → fall back to whatever SD has for this item
			candidates = m.Data
		}

		// 2) Prefer poster-like categories, tie-break by width (bigger first)
		categoryPrefs := map[string]int{
			"Poster Art": 0,
			"Box Art":    1,
			"Banner-L1":  2,
			"Banner-L2":  3,
			"VOD Art":    4,
		}
		var chosen Data
		bestScore := 1 << 30
		for _, d := range candidates {
			catScore, ok := categoryPrefs[d.Category]
			if !ok {
				continue
			}
			score := catScore*1000 - d.Width
			if score < bestScore {
				bestScore = score
				chosen = d
			}
		}
		if chosen.URI == "" && len(candidates) > 0 {
			chosen = candidates[0]
		}

		if chosen.URI != "" {
			uri := chosen.URI
			if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
				uri = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s?token=%s", uri, Token)
			}
			out := Icon{Src: uri, Height: chosen.Height, Width: chosen.Width}

			// IMPORTANT: in ProxyMode we do NOT pre-download here.
			if Config.Options.Images.Download && !Config.Options.Images.ProxyMode {
				downloadImage(out.Src, id)
			}
			i = append(i, out)
		}
	}
	return
}

// resolveSDImageForProgram chooses the SD image record for a program without downloading it.
// Used by the lazy proxy endpoint so it can fetch exactly once on first client request.
func (c *cache) resolveSDImageForProgram(id string) (Data, bool) {
	if m, ok := c.Metadata[id]; ok {
		desired := strings.TrimSpace(Config.Options.Images.PosterAspect)
		candidates := make([]Data, 0, len(m.Data))
		for _, d := range m.Data {
			if desired == "" || strings.EqualFold(desired, "all") || strings.EqualFold(d.Aspect, desired) {
				candidates = append(candidates, d)
			}
		}
		if len(candidates) == 0 {
			candidates = m.Data
		}
		if len(candidates) == 0 {
			return Data{}, false
		}
		categoryPrefs := map[string]int{
			"Poster Art": 0,
			"Box Art":    1,
			"Banner-L1":  2,
			"Banner-L2":  3,
			"VOD Art":    4,
		}
		var chosen Data
		bestScore := 1 << 30
		for _, d := range candidates {
			catScore, ok := categoryPrefs[d.Category]
			if !ok {
				continue
			}
			score := catScore*1000 - d.Width
			if score < bestScore {
				bestScore = score
				chosen = d
			}
		}
		if chosen.URI == "" {
			chosen = candidates[0]
		}
		return chosen, true
	}
	return Data{}, false
}

// downloadImage downloads a single image URL and saves it to local disk as {Image Path}/{programID}.jpg.
// This is only used when not running in ProxyMode (i.e., eager mode).
func downloadImage(url string, programID string) {
	localPath := filepath.Join(Config.Options.Images.Path, programID) + ".jpg"

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if _, err := os.Stat(Config.Options.Images.Path); os.IsNotExist(err) {
			if err := os.MkdirAll(Config.Options.Images.Path, 0755); err != nil {
				logger.Error("unable to create the images path", "error", err)
				return
			}
		}

		resp, err := http.Get(url)
		if err != nil {
			logger.Error("unable to download image", "error", err)
			return
		}
		defer resp.Body.Close()

		out, err := os.Create(localPath)
		if err != nil {
			logger.Error("unable to create local image file", "error", err)
			return
		}
		defer out.Close()

		if _, err = io.Copy(out, resp.Body); err != nil {
			logger.Error("unable to save downloaded image", "error", err)
			return
		}
	}
}
