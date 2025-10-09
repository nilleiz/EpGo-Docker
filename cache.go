package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Cache : Cache file
var Cache cache
var ImageError bool = false

// Init : Inti cache
func (c *cache) Init() {

	if c.Schedule == nil {
		c.Schedule = make(map[string][]EPGoCache)
	}

	c.Channel = make(map[string]EPGoCache)

	if c.Program == nil {
		c.Program = make(map[string]EPGoCache)
	}

	if c.Metadata == nil {
		c.Metadata = make(map[string]EPGoCache)
	}

}

func (c *cache) Remove() {
	if len(Config.Files.Cache) != 0 {
		logger.Info("Remove Cache File", "filename", Config.Files.Cache)
		os.RemoveAll(Config.Files.Cache)
		c.Init()
	}
}

func (c *cache) AddStations(data *[]byte, lineup string) {

	c.Lock()
	defer c.Unlock()

	var epgoCache EPGoCache
	var sdData SDStation

	err := json.Unmarshal(*data, &sdData)
	if err != nil {
		logger.Error("unable to unmarshal the JSON", "error", err)
		return
	}

	var channelIDs = Config.GetChannelList(lineup)

	for _, sd := range sdData.Stations {

		if ContainsString(channelIDs, sd.StationID) != -1 {

			epgoCache.StationID = sd.StationID
			epgoCache.Name = sd.Name
			epgoCache.Callsign = sd.Callsign
			epgoCache.Affiliate = sd.Affiliate
			epgoCache.BroadcastLanguage = sd.BroadcastLanguage
			epgoCache.Logo = sd.Logo

			c.Channel[sd.StationID] = epgoCache

		}

	}

}

func (c *cache) AddSchedule(data *[]byte) {

	c.Lock()
	defer c.Unlock()

	var epgoCache EPGoCache
	var sdData []SDSchedule

	err := json.Unmarshal(*data, &sdData)
	if err != nil {
		logger.Error("unable to unmarshal the JSON", "error", err)
		return
	}

	for _, sd := range sdData {

		if _, ok := c.Schedule[sd.StationID]; !ok {
			c.Schedule[sd.StationID] = []EPGoCache{}
		}

		for _, p := range sd.Programs {

			epgoCache.AirDateTime = p.AirDateTime
			epgoCache.AudioProperties = p.AudioProperties
			epgoCache.Duration = p.Duration
			epgoCache.LiveTapeDelay = p.LiveTapeDelay
			epgoCache.New = p.New
			epgoCache.Md5 = p.Md5
			epgoCache.ProgramID = p.ProgramID
			epgoCache.Ratings = p.Ratings
			epgoCache.VideoProperties = p.VideoProperties

			c.Schedule[sd.StationID] = append(c.Schedule[sd.StationID], epgoCache)

		}

	}

}

func (c *cache) AddProgram(gzip *[]byte, wg *sync.WaitGroup) {

	c.Lock()

	defer func() {
		c.Unlock()
		wg.Done()
	}()

	b, err := gUnzip(*gzip)
	if err != nil {
		logger.Error("unable to unzip programs", "error", err)
		return
	}

	var epgoCache EPGoCache
	var sdData []SDProgram

	err = json.Unmarshal(b, &sdData)
	if err != nil {
		logger.Error("unable to unmarshal the JSON", "error", err)
		return
	}

	for _, sd := range sdData {

		epgoCache.Descriptions = sd.Descriptions
		epgoCache.EpisodeTitle150 = sd.EpisodeTitle150
		epgoCache.Genres = sd.Genres

		epgoCache.HasEpisodeArtwork = sd.HasEpisodeArtwork
		epgoCache.HasImageArtwork = sd.HasImageArtwork
		epgoCache.HasSeriesArtwork = sd.HasSeriesArtwork
		epgoCache.Metadata = sd.Metadata
		epgoCache.OriginalAirDate = sd.OriginalAirDate
		epgoCache.ResourceID = sd.ResourceID
		epgoCache.ShowType = sd.ShowType
		epgoCache.Titles = sd.Titles
		epgoCache.ContentRating = sd.ContentRating
		epgoCache.Cast = sd.Cast
		epgoCache.Crew = sd.Crew

		c.Program[sd.ProgramID] = epgoCache

	}

}

func (c *cache) AddMetadata(gzip *[]byte, wg *sync.WaitGroup) {

	c.Lock()
	defer func() {
		c.Unlock()
		wg.Done()
	}()

	b, err := gUnzip(*gzip)
	if err != nil {
		logger.Error("unable to unzip metadata", "error", err)
		return
	}

	var tmp = make([]interface{}, 0)

	var epgoCache EPGoCache

	err = json.Unmarshal(b, &tmp)
	if err != nil {
		logger.Error("unable to unmarshal the JSON", "error", err)
		return
	}

	for _, t := range tmp {

		var sdData SDMetadata

		jsonByte, _ := json.Marshal(t)
		err = json.Unmarshal(jsonByte, &sdData)
		if err != nil {

			var sdError SDError
			err = json.Unmarshal(jsonByte, &sdError)
			if err == nil {

				if Config.Options.SDDownloadErrors {
					err = fmt.Errorf("%s [SD API Error Code: %d] Program ID: %s", sdError.Data.Message, sdError.Data.Code, sdError.ProgramID)
					logger.Error("unable to unmarshal the JSON", "error", err)
				}

			} else {
				if Config.Options.SDDownloadErrors {
					logger.Error("unable to unmarshal the JSON", "error", err)
				}
			}

		} else {

			epgoCache.Data = sdData.Data
			c.Metadata[sdData.ProgramID] = epgoCache

		}

	}
}

func (c *cache) GetAllProgramIDs() (programIDs []string) {

	for _, channel := range c.Schedule {

		for _, schedule := range channel {

			if ContainsString(programIDs, schedule.ProgramID) == -1 {
				programIDs = append(programIDs, schedule.ProgramID)
			}

		}

	}

	return
}

func (c *cache) GetRequiredProgramIDs() (programIDs []string) {

	var allProgramIDs = c.GetAllProgramIDs()

	for _, id := range allProgramIDs {

		if _, ok := c.Program[id]; !ok {

			if ContainsString(programIDs, id) == -1 {
				programIDs = append(programIDs, id)
			}

		}

	}

	return
}

func (c *cache) GetRequiredMetaIDs() (metaIDs []string) {

	for id := range c.Program {

		if len(id) > 10 {

			if _, ok := c.Metadata[id]; !ok {
				metaIDs = append(metaIDs, id)
			}

		}
	}

	return
}

// GetIcon returns the best poster image for XMLTV generation.
// Rules:
//   • Respect configured aspect using a ratio with tolerance (e.g., target 2/3 ± tol).
//   • Hard-deny portrait/person & banner categories in poster mode.
//   • Prefer Series/Season/Poster/Showcard/Box/Key/Cover (episode art is de-prioritized).
//   • Enforce a minimum size so tiny art is rejected.
//   • If nothing acceptable remains, return no icon so TMDB fallback is used.
func (c *cache) GetIcon(id string) (i []Icon) {
	m, ok := c.Metadata[id]
	if !ok || len(m.Data) == 0 {
		return
	}

	// Poster aspect (portrait) config
	targetAspect := 2.0 / 3.0
	aspCfg := strings.TrimSpace(Config.Options.Images.PosterAspect)
	if aspCfg != "" {
		if r, ok := parseAspect(aspCfg); ok {
			targetAspect = r
		}
	}
	const aspectTol = 0.08     // a bit wider to catch mislabeled posters
	const minW, minH = 600, 800 // minimum size for usable posters
	posterMode := targetAspect < 1.0

	isBadPosterCat := func(cat string) bool {
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
	// Prefer series/season/program poster categories
	isGoodPosterCat := func(cat string) bool {
		c := strings.ToLower(strings.TrimSpace(cat))
		switch c {
		case "poster", "poster art", "showcard", "series", "season", "box art", "boxart", "key art", "cover art":
			return true
		default:
			return strings.Contains(c, "poster") || strings.Contains(c, "showcard") || strings.Contains(c, "box")
		}
	}

	// Filter and split by aspect closeness
	var exact, loose []Data
	for _, d := range m.Data {
		if strings.TrimSpace(d.URI) == "" {
			continue
		}
		if d.Width < minW || d.Height < minH {
			continue
		}
		if posterMode && isBadPosterCat(d.Category) {
			continue
		}
		// Episode-vs-Season/Series: prefer non-episode by scoring later, we don't exclude here,
		// but SD often marks category as "Series"/"Season" for program posters.

		ratio := guessRatio(d)
		close := math.Abs(ratio-targetAspect) <= aspectTol

		// For exact-style consideration in poster mode, require a "good" poster category
		if close && (!posterMode || isGoodPosterCat(d.Category)) {
			exact = append(exact, d)
		} else {
			loose = append(loose, d)
		}
	}

	// Nothing left from SD? Let TMDB take over.
	if posterMode && len(exact) == 0 && len(loose) == 0 {
		return
	}

	score := func(d Data) int {
		s := 50
		if isGoodPosterCat(d.Category) {
			s = 100
		}
		// Gentle boost for larger size
		s += d.Width / 40
		// De-prioritize probable episode art by small nudge if category looks like episode/still
		cat := strings.ToLower(strings.TrimSpace(d.Category))
		if strings.Contains(cat, "episode") || strings.Contains(cat, "still") {
			s -= 10
		}
		return s
	}
	pick := func(list []Data) Data {
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

	chosen := Data{}
	if len(exact) > 0 {
		chosen = pick(exact)
	} else if len(loose) > 0 {
		chosen = pick(loose)
	}

	if strings.TrimSpace(chosen.URI) == "" {
		return
	}

	// Build absolute SD URL if needed (XML will later get rewritten to proxy URL).
	uri := chosen.URI
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		uri = fmt.Sprintf("https://json.schedulesdirect.org/20141201/image/%s?token=%s", uri, Token)
	}
	out := Icon{Src: uri, Height: chosen.Height, Width: chosen.Width}

	// Optional eager download
	if Config.Options.Images.Download {
		downloadImage(out.Src, id)
	}
	i = append(i, out)
	return
}

func (c *cache) Open() (err error) {

	data, err := os.ReadFile(Config.Files.Cache)

	if err != nil {
		c.Init()
		c.Save()
		return nil
	}

	err = json.Unmarshal(data, &c)
	if err != nil {
		return
	}

	return
}

func (c *cache) Save() (err error) {

	c.Lock()
	defer c.Unlock()

	data, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(Config.Files.Cache, data, 0644)
	if err != nil {
		return
	}

	return
}

func (c *cache) CleanUp() {

	var count int
	logger.Info("Clean up Cache", "filename", Config.Files.Cache)

	var programIDs = c.GetAllProgramIDs()

	for id := range c.Program {

		if ContainsString(programIDs, id) == -1 {

			count++
			delete(c.Program, id)
			delete(c.Metadata, id)

		}

	}

	c.Channel = make(map[string]EPGoCache)
	c.Schedule = make(map[string][]EPGoCache)

	logger.Info("Clean up Cache", "count", count)

	err := c.Save()
	if err != nil {
		logger.Error("unable to save the JSON", "error", err)
		return
	}
}

// Get data from cache
func (c *cache) GetTitle(id, lang string) (t []Title) {

	if p, ok := c.Program[id]; ok {

		var title Title

		for _, s := range p.Titles {
			title.Value = s.Title120
			title.Lang = lang
			t = append(t, title)
		}

	}

	if len(t) == 0 {
		var title Title
		title.Value = "No EPG Info"
		title.Lang = "en"
		t = append(t, title)
	}

	return
}

func (c *cache) GetSubTitle(id, lang string) (s SubTitle) {

	if p, ok := c.Program[id]; ok {

		if len(p.EpisodeTitle150) != 0 {

			s.Value = p.EpisodeTitle150
			s.Lang = lang

		} else {

			for _, d := range p.Descriptions.Description100 {

				s.Value = d.Description
				s.Lang = d.DescriptionLanguage

			}

		}

	}

	return
}

func (c *cache) GetDescs(id, subTitle string) (de []Desc) {

	if p, ok := c.Program[id]; ok {

		d := p.Descriptions

		var desc Desc

		for _, tmp := range d.Description1000 {

			switch Config.Options.SubtitleIntoDescription {

			case true:
				if len(subTitle) != 0 {
					desc.Value = fmt.Sprintf("[%s]\n%s", subTitle, tmp.Description)
					break
				}

				fallthrough
			case false:
				desc.Value = tmp.Description

			}

			desc.Lang = tmp.DescriptionLanguage

			de = append(de, desc)
		}

	}

	return
}

func (c *cache) GetCredits(id string) (cr Credits) {

	if Config.Options.Credits {

		if p, ok := c.Program[id]; ok {

			// Crew
			for _, crew := range p.Crew {

				switch crew.Role {

				case "Director":
					cr.Director = append(cr.Director, Director{Value: crew.Name})

				case "Producer":
					cr.Producer = append(cr.Producer, Producer{Value: crew.Name})

				case "Presenter":
					cr.Presenter = append(cr.Presenter, Presenter{Value: crew.Name})

				case "Writer":
					cr.Writer = append(cr.Writer, Writer{Value: crew.Name})

				}

			}

			// Cast
			for _, cast := range p.Cast {

				switch cast.Role {

				case "Actor":
					cr.Actor = append(cr.Actor, Actor{Value: cast.Name, Role: cast.CharacterName})

				}

			}

		}

	}

	return
}

func (c *cache) GetCategory(id string) (ca []Category) {

	if p, ok := c.Program[id]; ok {

		for _, g := range p.Genres {

			var category Category
			category.Value = g
			category.Lang = "en"

			ca = append(ca, category)

		}

	}

	return
}

func (c *cache) GetEpisodeNum(id string) (ep []EpisodeNum) {

	var seaseon, episode int

	if p, ok := c.Program[id]; ok {

		for _, m := range p.Metadata {

			seaseon = m.Gracenote.Season
			episode = m.Gracenote.Episode

			var episodeNum EpisodeNum

			if seaseon != 0 && episode != 0 {

				episodeNum.Value = fmt.Sprintf("%d.%d.", seaseon-1, episode-1)
				episodeNum.System = "xmltv_ns"
				ep = append(ep, episodeNum)
			}

		}

		if seaseon != 0 && episode != 0 {

			var episodeNum EpisodeNum
			episodeNum.Value = fmt.Sprintf("S%d E%d", seaseon, episode)
			episodeNum.System = "onscreen"
			ep = append(ep, episodeNum)

		}

		// ✅ Fixed dd_progid formatting
		if len(ep) == 0 {

			var episodeNum EpisodeNum

			switch id[0:2] {

			case "EP":
				// Correct format: keep prefix, add dot + episode part if present
				if len(id) > 10 {
					episodeNum.Value = fmt.Sprintf("%s.%s", id[:10], id[10:])
				} else {
					episodeNum.Value = id + ".0000"
				}

			case "SH", "MV":
				episodeNum.Value = id + ".0000"

			default:
				episodeNum.Value = id
			}

			episodeNum.System = "dd_progid"
			ep = append(ep, episodeNum)

		}

		if len(p.OriginalAirDate) > 0 {

			var episodeNum EpisodeNum
			episodeNum.Value = p.OriginalAirDate
			episodeNum.System = "original-air-date"
			ep = append(ep, episodeNum)

		}

	}

	return
}

func (c *cache) GetPreviouslyShown(id string) (prev *PreviouslyShown) {

	prev = &PreviouslyShown{}

	if p, ok := c.Program[id]; ok {
		prev.Start = p.OriginalAirDate
	}

	return
}

func (c *cache) GetRating(id, countryCode string) (ra []Rating) {

	if !Config.Options.Rating.Guidelines {
		return
	}

	var add = func(code, body, country string) {

		switch Config.Options.Rating.CountryCodeAsSystem {

		case true:
			ra = append(ra, Rating{Value: code, System: country})

		case false:
			ra = append(ra, Rating{Value: code, System: body})

		}

	}

	if p, ok := c.Program[id]; ok {

		switch len(Config.Options.Rating.Countries) {

		case 0:
			for _, r := range p.ContentRating {

				if len(ra) == Config.Options.Rating.MaxEntries && Config.Options.Rating.MaxEntries != 0 {
					return
				}

				if countryCode == r.Country {
					add(r.Code, r.Body, r.Country)
				}

			}

			for _, r := range p.ContentRating {

				if len(ra) == Config.Options.Rating.MaxEntries && Config.Options.Rating.MaxEntries != 0 {
					return
				}

				if countryCode != r.Country {
					add(r.Code, r.Body, r.Country)
				}

			}

		default:
			for _, cCode := range Config.Options.Rating.Countries {

				for _, r := range p.ContentRating {

					if len(ra) == Config.Options.Rating.MaxEntries && Config.Options.Rating.MaxEntries != 0 {
						return
					}

					if cCode == r.Country {

						add(r.Code, r.Body, r.Country)

					}

				}

			}

		}

	}

	return
}

func downloadImage(imageURL, programID string) (string, error) {

	folderImage := Config.Options.Images.Path

	if Config.Options.Images.Path == "" {
		folderImage = "images"
	}

	// Create the "images" folder if it doesn't exist
	if _, err := os.Stat(folderImage); os.IsNotExist(err) {
		err = os.MkdirAll(folderImage, 0755)
		if err != nil {
			return "", fmt.Errorf("failed to create images folder: %w", err)
		}
	}

	// Extract filename from URL
	filename := programID + ".jpg"
	filePath := filepath.Join(folderImage, filename)
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download image: %s", resp.Status)
	}
	defer resp.Body.Close()

	out, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	return filePath, nil
}

// ---- helpers for aspect handling (shared idea with proxy resolver) ----

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
	a, err1 := parseFloat(parts[0])
	b, err2 := parseFloat(parts[1])
	if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
		return 0, false
	}
	return a / b, true
}

func parseFloat(s string) (float64, error) {
	return strconvParseFloat(strings.TrimSpace(s), 64)
}

func strconvParseFloat(s string, bitSize int) (float64, error) {
	// Avoid importing strconv in this file’s header; we already import a lot.
	// We can rely on fmt.Sscan as a tiny helper.
	var f float64
	_, err := fmt.Sscan(s, &f)
	return f, err
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
