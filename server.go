package main

import (
	"net/http"
	"path/filepath"

)

// StartServer starts a local HTTP server to serve files from the specified directory.
func StartServer(dir string, port string) {
	fs := http.FileServer(http.Dir(dir))
	http.Handle("/", fs)

	logger.Info("Starting server", "address", "http://"+Config.Server.Address+":"+port, "serving", filepath.Clean(dir))
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		logger.Error("Server failed to start", "error", err)
	}
}
