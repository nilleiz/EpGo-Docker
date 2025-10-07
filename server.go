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
	"time"
)

// ensureProgramMetadata fetches SD metadata for a single ProgramID and stores it in Cache.
// Returns true if metadata is present afterwards.
func ensureProgramMetadata(programID string) bool {
	// Already present?
	if _, ok := Cache.Metadata[programID]; ok {
		return true
	}

	logger.Info("Proxy: metadata missing, fetching", "programID", programID)

	// Prepare SD client
	var sd SD
	if err := sd.Init(); err != nil {
		logger.Error("Proxy: SD init failed", "programID", programID, "error", err)
		return false
	}

	// POST to /metadata/programs/ with a single-element array body
	sd.Req.URL = fmt.Sprintf("%smetadata/programs/", sd.BaseURL)
	sd.Req.Type = "POST"
	sd.Req.Call = "metadata"
	sd.Req.Compression = false

	body, err := json.Marshal([]string{programID})
	if err != nil {
		logger.Error("Proxy: marshal metadata request failed", "programID", programID, "error", err)
		return false
	}
	sd.Req.Data = body

	if err := sd.Connect(); err != nil {
		logger.Error("Proxy: SD metadata connect failed", "programID", programID, "error", err)
		return false
	}

	// Reuse existing cache parsing to fill Cache.Metadata
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

// StartServer starts a local HTTP server: static files + lazy SD image proxy.
// HTTPS termination should be done by your reverse proxy (Cloudflare/Nginx).
func StartServer(dir string, port string) {
	mux := http.NewServeMux()

	// On-demand SD image proxy: /proxy/sd/{programID}
	// First request downloads from SD (with a fresh token) and stores by imageID.
	// Next requests are served from local disk.
	mux.HandleFunc("/proxy/sd/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/proxy/sd/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "missing programID", http.StatusBadRequest)
			return
		}
		programID := parts[0]

		// Destination folder for cached images
		folderImage := Config.Options.Images.Path
		if folderImage == "" {
			folderImage = "images"
		}
		if err := os.MkdirAll(folderImage, 0755); err != nil {
			http.Error(w, "failed to prepare image folder", http.StatusInternalServerError)
			return
		}

		// Choose SD image for the program; if metadata is missing, fetch it on-demand.
		chosen, ok := Cache.resolveSDImageForProgram(programID)
		if !ok || chosen.URI == "" {
			if ensureProgramMetadata(programID) {
				if ch2, ok2 := Cache.resolveSDImageForProgram(programID); ok2 && ch2.URI != "" {
					chosen = ch2
					ok = true
				}
			}
		}
		if !ok || chosen.URI == "" {
			logger.Warn("Proxy: no suitable image in metadata", "programID", programID)
			http.NotFound(w, r)
			return
		}

		imageID := sdImageID(chosen.URI) // normalize to bare imageID
		filePath := filepath.Join(folderImage, imageID+".jpg")

		// Serve from disk if present
		if fi, err := os.Stat(filePath); err == nil && !fi.IsDir() {
			logger.Info("Proxy: serve from cache", "programID", programID, "imageID", imageID, "path", filePath)
			serveFileCached(w, r, filePath)
			return
		}

		// Not cached yet → fetch once with a fresh token
		token, err := ensureFreshToken()
		if err != nil {
			logger.Error("Proxy: token error", "programID", programID, "error", err)
			http.Error(w, "token error", http.StatusBadGateway)
			return
		}

		imageURL := fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s?token=%s", imageID, token)
		logger.Info("Proxy: downloading image from SD", "programID", programID, "imageID", imageID)

		client := &http.Client{Timeout: 20 * time.Second}
		fetch := func(url string) (*http.Response, error) {
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("User-Agent", AppName)
			return client.Do(req)
		}

		resp, err := fetch(imageURL)
		if err != nil {
			logger.Error("Proxy: fetch failed", "programID", programID, "imageID", imageID, "error", err)
			http.Error(w, "fetch failed", http.StatusBadGateway)
			return
		}
		// If token expired, refresh once and retry.
		if resp.StatusCode == http.StatusUnauthorized {
			logger.Warn("Proxy: SD token unauthorized, refreshing", "programID", programID)
			resp.Body.Close()
			token, _ = ensureFreshToken()
			imageURL = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s?token=%s", imageID, token)
			resp, err = fetch(imageURL)
			if err != nil {
				logger.Error("Proxy: fetch retry failed", "programID", programID, "imageID", imageID, "error", err)
				http.Error(w, "fetch retry failed", http.StatusBadGateway)
				return
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Warn("Proxy: SD returned non-200", "programID", programID, "imageID", imageID, "status", resp.Status)
			http.Error(w, resp.Status, resp.StatusCode)
			return
		}

		// Save to disk once (by imageID)
		out, err := os.Create(filePath)
		if err != nil {
			logger.Error("Proxy: save failed (create)", "programID", programID, "imageID", imageID, "path", filePath, "error", err)
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		if _, err = io.Copy(out, resp.Body); err != nil {
			out.Close()
			_ = os.Remove(filePath)
			logger.Error("Proxy: save failed (write)", "programID", programID, "imageID", imageID, "path", filePath, "error", err)
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		out.Close()
		logger.Info("Proxy: saved image", "programID", programID, "imageID", imageID, "path", filePath)

		// Serve cached file
		logger.Info("Proxy: serve freshly cached", "programID", programID, "imageID", imageID, "path", filePath)
		serveFileCached(w, r, filePath)
	})

	// Static file server at "/"
	fs := http.FileServer(http.Dir(dir))
	mux.Handle("/", fs)

	logger.Info("Starting server", "address", "http://"+Config.Server.Address+":"+port, "serving", filepath.Clean(dir))
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("Server failed to start", "error", err)
	}
}

// Serve a local file with strong cache headers
func serveFileCached(w http.ResponseWriter, r *http.Request, path string) {
	fi, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
	http.ServeFile(w, r, path)
}
