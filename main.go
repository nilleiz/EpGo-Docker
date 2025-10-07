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
	Version     = "v1.1"
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
		flag.usage()
		os.Exit(0)
	}

	if *version {
		// Keep printing the upstream EPGo version here (as before)
		fmt.Println(Version)
		os.Exit(0)
	}

	if len(*configure) != 0 {
		err := Configure(*configure)
		if err != nil {
			logger.Error("unable to create the configuration file", "error", err)
		}
		os.Exit(0)
	}

	if len(*config) != 0 {
		var sd SD
		err := sd.Update(*config)
		if err != nil {
			// EPG grab failed — log the error first
			logger.Error("unable to get data from Schedules Direct", "error", err)

			// Optional behavior: keep the image proxy up so cached artwork still works
			// This uses the new config switch: Server.Start proxy when EPG grab fails
			if Config.Server.Enable && Config.Server.StartProxyOnGrabFail {
				// Decide where to serve from; default to "images" if empty
				imgDir := Config.Options.Images.Path
				if strings.TrimSpace(imgDir) == "" {
					imgDir = "images"
				}
				port := Config.Server.Port
				if strings.TrimSpace(port) == "" {
					port = "8080"
				}

				logger.Warn("EPG fetch failed; starting image proxy anyway so cached artwork keeps working",
					"address", Config.Server.Address, "port", port, "dir", imgDir)

				// StartServer blocks; we intentionally keep the process alive serving cached images
				StartServer(imgDir, port)

				// NOTE: If you prefer to keep returning to caller after starting the server,
				// run it asynchronously: go StartServer(imgDir, port); select {}
			}

			// If the switch is off, exit with non-zero (preserve current behavior)
			os.Exit(1)
		}
	}

	if len(*serve) != 0 {
		parts := strings.Split(*serve, ":")
		if len(parts) != 2 {
			logger.Error("Invalid serve argument. Use format [directory:port]", "argument", *serve)
			os.Exit(1)
		}
		dir := parts[0]
		port := parts[1]
		StartServer(dir, port)
	}
}
