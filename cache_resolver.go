package main

import "strings"

// resolveSDImageForProgram chooses the best SD image for a program without downloading it.
// It respects the configured Poster Aspect (Options.Images.PosterAspect) when possible,
// then prefers poster-like categories; ties are broken by width (larger first).
func (c *cache) resolveSDImageForProgram(programID string) (Data, bool) {
	m, ok := c.Metadata[programID]
	if !ok {
		return Data{}, false
	}

	candidates := m.Data
	if len(candidates) == 0 {
		return Data{}, false
	}

	// 1) Try to honor the configured aspect (e.g., "2x3", "4x3", "16x9").
	desired := strings.TrimSpace(Config.Options.Images.PosterAspect)
	if desired != "" && !strings.EqualFold(desired, "all") {
		filtered := make([]Data, 0, len(candidates))
		for _, d := range candidates {
			if strings.EqualFold(d.Aspect, desired) {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}

	// 2) Prefer poster-like categories; tie-break by width (bigger preferred).
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
		score := catScore*1000 - d.Width // lower is better
		if score < bestScore {
			bestScore = score
			chosen = d
		}
	}

	// 3) Fallback: if no preferred category found, pick the widest of what's left.
	if chosen.URI == "" {
		for _, d := range candidates {
			if d.Width > chosen.Width {
				chosen = d
			}
		}
	}

	if chosen.URI == "" {
		return Data{}, false
	}
	return chosen, true
}
