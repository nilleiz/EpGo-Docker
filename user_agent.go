package main

import "fmt"

// Public repo URL (helps Schedules Direct identify the app / contact)
const ProjectURL = "https://github.com/nilleiz/EpGo-Docker"

// Build a versioned User-Agent per SD guidelines.
// Example: "EPGo-Docker/v1.1 (https://github.com/nilleiz/EpGo-Docker)"
// Uses AppName and Version defined in main.go.
func userAgent() string {
	return fmt.Sprintf("%s/%s (%s)", AppName, Version, ProjectURL)
}
