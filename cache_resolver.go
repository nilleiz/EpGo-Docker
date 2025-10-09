package main

import (
	"math"
	"strconv"
	"strings"
)

// resolveSDImageForProgram chooses the best SD image for a program without downloading it.
// It mirrors the EPG grab intent for posters:
//   • If a poster-like aspect is configured (e.g., "2x3", "3x4"), EXCLUDE portrait-ish/person
//     images (Iconic, Headshot, Cast, Person, …) and banners (Banner-L2/Banner/Landscape).
//   • Prefer poster categories: Poster/Poster Art/Showcard/Series/Season/Box Art/Key Art/Cover Art.
//   • Respect the configured aspect when possible (±small tolerance).
//   • If nothing acceptable remains from SD, return (Data{}, false) so the caller can 404.
//     (That’s OK because the XMLTV you generated would already point to TMDB for that program.)
func (c *cache) resolveSDImageForProgram(programID string) (Data, bool) {
	m, ok := c.Metadata[programID]
	if !ok {
		return Data{}, false
	}
	candidates := m.Data
	if len(candidates) == 0 {
		return Data{}, false
	}

	// Desired aspect: default to poster-ish 2x3 if configured empty.
	targetAspect := 2.0 / 3.0
	aspCfg := strings.TrimSpace(Config.Options.Images.PosterAspect)
	if aspCfg != "" {
		if r, ok := parseAspect(aspCfg); ok {
			targetAspect = r
		}
	}
	const aspectTol = 0.06 // acceptable deviation
	posterMode := targetAspect < 1.0 // portrait == poster-like

	// Categories we consider "bad" for posters (portraits / non-poster formats).
	isBadPosterCategory := func(cat string) bool {
		c := strings.ToLower(strings.TrimSpace(cat))
		switch c {
		case "iconic", "cast", "person", "people", "headshot", "celebrity", "photo", "portrait":
			return true
		case "banner-l2", "banner", "landscape", "horizontal art", "background", "fanart":
			return true
		default:
			if strings.Contains(c, "headshot") ||
				strings.Contains(c, "cast") ||
				strings.Contains(c, "celebrity") ||
				strings.Contains(c, "person") ||
				strings.Contains(c, "portrait") ||
				strings.Contains(c, "banner") {
				return true
			}
		}
		return false
	}

	// Categories we strongly prefer for posters.
	isGoodPosterCategory := func(cat string) bool {
		c := strings.ToLower(strings.TrimSpace(cat))
		switch c {
		case "poster", "poster art", "showcard", "series", "season", "box art", "boxart", "key art", "cover art":
			return true
		default:
			return strings.Contains(c, "poster") || strings.Contains(c, "showcard") || strings.Contains(c, "box")
		}
	}

	// Split candidates into exact-aspect matches and loose ones; also prefilter
	// to poster categories when posterMode is on (exclude bad categories entirely).
	var exact, loose []Data
	for _, d := range candidates {
		if strings.TrimSpace(d.URI) == "" {
			continue
		}
		// Hard filter in poster mode
		if posterMode && isBadPosterCategory(d.Category) {
			continue
		}

		ratio := guessRatio(d)
		exactMatch := math.Abs(ratio-targetAspect) <= aspectTol

		// For exact aspect, only treat as "exact" if it's a good poster category in posterMode.
		if exactMatch && (!posterMode || isGoodPosterCategory(d.Category)) {
			exact = append(exact, d)
			continue
		}
		loose = append(loose, d)
	}

	// If posterMode left us with no options at all, bail out (EPG would have used TMDB).
	if posterMode && len(exact) == 0 && len(loose) == 0 {
		return Data{}, false
	}

	// Choose a winner by score; prefer good poster categories, "primary", then bigger width.
	pickBest := func(list []Data) Data {
		best := Data{}
		bestScore := -1
		for _, d := range list {
			score := 50
			if isGoodPosterCategory(d.Category) {
				score = 100
			}
			if d.Primary {
				score += 8
			}
			score += d.Width / 40 // gentle bias for larger images

			if score > bestScore || (score == bestScore && d.Width > best.Width) {
				best = d
				bestScore = score
			}
		}
		return best
	}

	// 1) Prefer exact-aspect matches.
	if len(exact) > 0 {
		return pickBest(exact), true
	}

	// 2) Fall back to loose matches (still filtered by posterMode rules).
	if len(loose) > 0 {
		return pickBest(loose), true
	}

	return Data{}, false
}

// parseAspect parses strings like "2x3" or "16x9" into a float ratio (w/h).
func parseAspect(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	sep := "x"
	if strings.Contains(s, ":") {
		sep = ":"
	}
	parts := strings.Split(s, sep)
	if len(parts) != 2 {
		return 0, false
	}
	a, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	b, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
		return 0, false
	}
	return a / b, true
}

// guessRatio tries to determine the ratio (w/h). If Data.Aspect is a parseable form like "2x3",
// use that; otherwise fall back to Width/Height; otherwise assume poster-ish 2x3.
func guessRatio(d Data) float64 {
	if r, ok := parseAspect(d.Aspect); ok {
		return r
	}
	if d.Width > 0 && d.Height > 0 {
		return float64(d.Width) / float64(d.Height)
	}
	return 2.0 / 3.0
}
