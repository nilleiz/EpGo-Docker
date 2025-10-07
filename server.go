package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

		// Choose SD image for the program
		chosen, ok := Cache.resolveSDImageForProgram(programID)
		if !ok || chosen.URI == "" {
			http.NotFound(w, r)
			return
		}
		imageID := sdImageID(chosen.URI)              // normalize to bare imageID
		filePath := filepath.Join(folderImage, imageID+".jpg") // dedup by imageID

		// Serve from disk if present
		if fi, err := os.Stat(filePath); err == nil && !fi.IsDir() {
			serveFileCached(w, r, filePath)
			return
		}

		// Not cached yet → fetch once with a fresh token
		token, err := ensureFreshToken()
		if err != nil {
			http.Error(w, "token error", http.StatusBadGateway)
			return
		}
		imageURL := fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s?token=%s", imageID, token)

		client := &http.Client{Timeout: 20 * time.Second}
		fetch := func(url string) (*http.Response, error) {
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("User-Agent", AppName)
			return client.Do(req)
		}

		resp, err := fetch(imageURL)
		if err != nil {
			http.Error(w, "fetch failed", http.StatusBadGateway)
			return
		}
		// If token expired, refresh once and retry.
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			token, _ = ensureFreshToken()
			imageURL = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s?token=%s", imageID, token)
			resp, err = fetch(imageURL)
			if err != nil {
				http.Error(w, "fetch retry failed", http.StatusBadGateway)
				return
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, resp.Status, resp.StatusCode)
			return
		}

		// Save to disk once (by imageID)
		out, err := os.Create(filePath)
		if err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		if _, err = io.Copy(out, resp.Body); err != nil {
			out.Close()
			_ = os.Remove(filePath)
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		out.Close()

		// Serve cached file
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
