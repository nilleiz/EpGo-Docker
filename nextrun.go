// nextrun.go
package main

import (
	"fmt"
	"os"
	"time"

	// Corrected the typo in the import path below
	"github.com/robfig/cron/v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: nextrun \"<cron_schedule>\"")
		os.Exit(1)
	}

	// Use a parser for the standard 5-part cron schedule
	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	s, err := p.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error parsing schedule:", err)
		os.Exit(1)
	}

	// Calculate and print the next run time from now
	nextTime := s.Next(time.Now())
	fmt.Println(nextTime.Format(time.RFC1123Z))
}