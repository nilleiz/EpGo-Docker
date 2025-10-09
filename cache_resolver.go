package main

import (
	"math"
	"strconv"
	"strings"
)

// resolveSDImageForProgram chooses the best SD image for a program without downloading it.
// Mirrors the EPG chooser intent:
//   • Poster aspect (portrait) → exclude Iconic/Headshot/Cast/Person and banners (Banner-L2/Banner/Landscape).
//   • Prefer Series/Season/Poster/Showcard/Box/Key/Cover.
//   • Use ratio-based aspect with tolerance.
//   • If nothing acceptable remains, return false so proxy 404s (XML already points to TMDB in that case).
func (c *cache) resolveSDImageForProgram(programID string) (Data, bool) {
	m, ok := c.Metadata[programID]
	if !ok {
		return Data{}, false
	}
	candidates := m.Data
	if len(candidates) == 0 {
		return Data{}, false
	}

	targetAspect := 2.0 / 3.0
	aspCfg := strings.TrimSpace(Config.Options.Images.PosterAspect)
	if aspCfg != "" {
		if r, ok := parseAspect(aspCfg); ok {
			targetAspect = r
		}
	}
	const aspectTol = 0.08
	posterMode := targetAspect < 1.0
	const minW, minH = 600, 800

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
	isGoodPosterCategory := func(cat string) bool {
		c := strings.ToLower(strings.TrimSpace(cat))
		switch c {
		case "poster", "poster art", "showcard", "series", "season", "box art", "boxart", "key art", "cover art":
			return true
		default:
			return strings.Contains(c, "poster") || strings.Contains(c, "showcard") || strings.Contains(c, "box")
		}
	}

	var exact, loose []Data
	for _, d := range candidates {
		if strings.TrimSpace(d.URI) == "" {
			continue
		}
		if d.Width < minW || d.Height < minH {
			continue
		}
		if posterMode && isBadPosterCategory(d.Category) {
			continue
		}

		ratio := guessRatio(d)
		exactMatch := math.Abs(ratio-targetAspect) <= aspectTol

		if exactMatch && (!posterMode || isGoodPosterCategory(d.Category)) {
			exact = append(exact, d)
		} else {
			loose = append(loose, d)
		}
	}

	if posterMode && len(exact) == 0 && len(loose) == 0 {
		return Data{}, false
	}

	score := func(d Data) int {
		s := 50
		if isGoodPosterCategory(d.Category) {
			s = 100
		}
		s += d.Width / 40
		cat := strings.ToLower(strings.TrimSpace(d.Category))
		if strings.Contains(cat, "episode") || strings.Contains(cat, "still") {
			s -= 10
		}
		return s
	}
	pickBest := func(list []Data) Data {
		best := Data{}
		bestScore := -1
		for _, d := range list {
			s := score(d)
			if s > bestScore || (s == bestScore && d.Width > best.Width) {
				best = d
				bestScore = s
			}
		}
		return best
	}

	if len(exact) > 0 {
		return pickBest(exact), true
	}
	if len(loose) > 0 {
		return pickBest(loose), true
	}
	return Data{}, false
}

// --- helpers (duplicated here to keep this file self-contained) ---

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

func guessRatio(d Data) float64 {
	if r, ok := parseAspect(d.Aspect); ok {
		return r
	}
	if d.Width > 0 && d.Height > 0 {
		return float64(d.Width) / float64(d.Height)
	}
	return 2.0 / 3.0
}
