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

// Open : Open cache file and read data from file
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

// Save : Save data to cache file
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

func (c *cache) GetIcon(id string) (i []Icon) {

	if m, ok := c.Metadata[id]; ok {
		// 1) Aspekt-Filter ("16x9" / "2x3" / "4x3" / "all")
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

		// 2) Poster bevorzugen; bei Gleichstand größere Breite
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

			// WICHTIG: im ProxyMode KEIN Vorab-Download
			if Config.Options.Images.Download && !Config.Options.Images.ProxyMode {
				downloadImage(out.Src, id)
			}
			i = append(i, out)
		}
	}
	return
}

// resolveSDImageForProgram: Auswahl des SD-Bildes ohne Download (für Proxy).
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

// downloadImage : Download image from url and save to disk
func downloadImage(url string, file string) {

	var localPath = filepath.Join(Config.Options.Images.Path, file) + ".jpg"

	if _, err := os.Stat(localPath); os.IsNotExist(err) {

		if _, err := os.Stat(Config.Options.Images.Path); os.IsNotExist(err) {
			err := os.MkdirAll(Config.Options.Images.Path, 0755)
			if err != nil {
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

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			logger.Error("unable to save downloaded image", "error", err)
			return
		}

	}
}
