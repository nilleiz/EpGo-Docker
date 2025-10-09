package main

import "strings"

// resolveSDImageForProgram chooses the best SD image for a program without downloading it.
// It mirrors the build-time selection logic used by Cache.GetIcon:
//   1) Optional aspect filter (Options.Images.PosterAspect). "all" or "" = no filter.
//   2) Prefer poster-like categories in this order (tie-break by larger width):
//        Poster Art > Box Art > Banner-L1 > Banner-L2 > VOD Art
//   3) Fallback if no preferred category was found: take the FIRST candidate (matches build-time).
func (c *cache) resolveSDImageForProgram(programID string) (Data, bool) {
	m, ok := c.Metadata[programID]
	if !ok {
		return Data{}, false
	}

	candidates := m.Data
	if len(candidates) == 0 {
		return Data{}, false
	}

	// 1) Aspect filter
	desired := strings.TrimSpace(Config.Options.Images.PosterAspect)
	filtered := make([]Data, 0, len(candidates))
	if desired == "" || strings.EqualFold(desired, "all") {
		filtered = candidates
	} else {
		for _, d := range candidates {
			if strings.EqualFold(d.Aspect, desired) {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			// No exact aspect match -> fall back to all
			filtered = candidates
		}
	}

	// 2) Category preference with width tie-break
	prefer := func(cat string) int {
		switch strings.ToLower(cat) {
		case "poster art":
			return 1
		case "box art":
			return 2
		case "banner-l1":
			return 3
		case "banner-l2":
			return 4
		case "vod art":
			return 5
		default:
			return 999
		}
	}

	var chosen Data
	bestRank := 999
	bestWidth := -1

	for _, d := range filtered {
		r := prefer(d.Category)
		if r < bestRank {
			bestRank = r
			bestWidth = d.Width
			chosen = d
			continue
		}
		if r == bestRank && d.Width > bestWidth {
			bestWidth = d.Width
			chosen = d
		}
	}

	// 3) Fallback: mirror build-time behavior — FIRST candidate (not "widest")
	if chosen.URI == "" && len(filtered) > 0 {
		chosen = filtered[0]
	}

	if chosen.URI == "" {
		return Data{}, false
	}
	return chosen, true
}
