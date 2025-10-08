package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Names (base = your fork, non-base = upstream)
const (
	AppName     = "EPGo-Docker" // your Docker fork
	Version     = "v1.1.4"
	BaseName    = "EPGo"        // upstream app
	BaseVersion = "v3.2.1"
)

// Config : Config file (struct)
var Config config
var Config2 string
var logger *slog.Logger

func main() {
	var configure = flag.String("configure", "", "= Create or modify the configuration file. [filename.yaml]")
	var config = flag.String("config", "", "= Get data from Schedules Direct with configuration file. [filename.yaml]")
	var version = flag.Bool("version", false, "= Get version")
	var serve = flag.String("serve", "", "= Start a local HTTP server to serve files from the specified directory. [directory:port]")
	var h = flag.Bool("h", false, ": Show help")

	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

	flag.Parse()
	Config2 = *config

	// Startup banner
	logger.Info(fmt.Sprintf("%s starting", AppName),
		"version", Version,
		"based_on", fmt.Sprintf("%s version=%s", BaseName, BaseVersion),
	)

	if *h {
		fmt.Println()
		flag.Usage()
		os.Exit(0)
	}

	if *version {
		fmt.Println(Version)
		os.Exit(0)
	}

	// Handle: epgo -configure filename.yaml
	if len(*configure) != 0 {
		if err := Configure(*configure); err != nil {
			logger.Error("unable to create the configuration file", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Standalone file server mode: epgo -serve dir:port
	if len(*serve) != 0 {
		parts := strings.Split(*serve, ":")
		if len(parts) != 2 {
			logger.Error("Invalid serve argument. Use format [directory:port]", "argument", *serve)
			os.Exit(1)
		}
		dir := parts[0]
		port := parts[1]
		StartServer(dir, port) // blocks
		return
	}

	// Normal mode: epgo -config file.yaml
	if len(*config) != 0 {
		var sd SD
		// Try to grab EPG; even if it fails, we may still start the proxy (if enabled).
		err := sd.Update(*config)
		if err != nil {
			logger.Error("unable to get data from Schedules Direct", "error", err)
		}

		// If Server.Enable is true, start the proxy regardless of EPG result.
		if Config.Server.Enable {
			imgDir := strings.TrimSpace(Config.Options.Images.Path)
			if imgDir == "" {
				imgDir = "images"
			}
			port := strings.TrimSpace(Config.Server.Port)
			if port == "" {
				port = "8080"
			}

			if err != nil {
				logger.Warn("EPG fetch failed; starting image proxy anyway so cached artwork keeps working",
					"address", Config.Server.Address, "port", port, "dir", imgDir)
			} else {
				logger.Info("Starting image proxy after successful EPG refresh",
					"address", Config.Server.Address, "port", port, "dir", imgDir)
			}

			StartServer(imgDir, port) // blocks
			return
		}

		// If the server isn't enabled, preserve exit semantics.
		if err != nil {
			os.Exit(1)
		}
		return
	}

	// If neither -serve nor -config nor -configure was provided, show usage and exit.
	flag.Usage()
	os.Exit(2)
}
