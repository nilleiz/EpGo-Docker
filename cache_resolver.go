package main

import "strings"

// resolveSDImageForProgram mirrors the strict selection from GetChosenSDImage:
// - Allowed categories only (Poster/Box/Banner/VOD Art)
// - If Poster Aspect is set (and not "all"), enforce exact aspect
// - Score = tierRank*100 + catRank*10 + aspectRank; ties by larger width
// - No generic fallback (returns false if no qualifying image)
func (c *cache) resolveSDImageForProgram(programID string) (Data, bool) {
	m, ok := c.Metadata[programID]
	if !ok || len(m.Data) == 0 {
		return Data{}, false
	}

	desired := strings.TrimSpace(Config.Options.Images.PosterAspect)

	// scoring (prefer show > season > episode via tier)
	var chosen Data
	bestScore := 1 << 30
	bestWidth := -1
	matchCount := 0
	for _, d := range m.Data {
		score, allowed := imageScore(d, desired)
		if !allowed {
			continue
		}
		matchCount++
		if score < bestScore || (score == bestScore && d.Width > bestWidth) {
			bestScore = score
			bestWidth = d.Width
			chosen = d
		}
	}

	if matchCount == 0 {
		return Data{}, false
	}

	if chosen.URI == "" {
		return Data{}, false
	}
	return chosen, true
}
