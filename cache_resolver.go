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

	// allowed categories only and skip blocked IDs
	filtered := make([]Data, 0, len(m.Data))
	for _, d := range m.Data {
		imageID := sdImageIDFromURI(d.URI)
		if imageID == "" {
			continue
		}
		if _, allowed := allowedCategoryRank(d.Category); !allowed {
			continue
		}
		if isImageBlocked(imageID) {
			continue
		}
		filtered = append(filtered, d)
	}
	if len(filtered) == 0 {
		return Data{}, false
	}

	// desired aspect enforcement
	if desired != "" && !strings.EqualFold(desired, "all") {
		tmp := make([]Data, 0, len(filtered))
		for _, d := range filtered {
			if strings.EqualFold(d.Aspect, desired) {
				tmp = append(tmp, d)
			}
		}
		if len(tmp) == 0 {
			return Data{}, false
		}
		filtered = tmp
	}

	// scoring (prefer show > season > episode via tier)
	var chosen Data
	bestScore := 1 << 30
	bestWidth := -1
	for _, d := range filtered {
		catRank, _ := allowedCategoryRank(d.Category)
		aRank := 0
		if desired == "" || strings.EqualFold(desired, "all") {
			aRank = aspectRank(d.Aspect)
		}
		score := tierRank(d.Tier)*100 + catRank*10 + aRank
		if score < bestScore || (score == bestScore && d.Width > bestWidth) {
			bestScore = score
			bestWidth = d.Width
			chosen = d
		}
	}

	if chosen.URI == "" {
		return Data{}, false
	}
	return chosen, true
}
