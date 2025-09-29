package main

import (
	"bytes"
	"encoding/xml"
	"epgo/tmdb"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CreateXMLTV builds the XMLTV file from the cache.
func CreateXMLTV(filename string) (err error) {
	defer func() { runtime.GC() }()

	Config.File = strings.TrimSuffix(filename, filepath.Ext(filename))

	var generator xml.Attr
	generator.Name = xml.Name{Local: "EPGo"}
	generator.Value = Version

	var source xml.Attr
	source.Name = xml.Name{Local: "source-info-name"}
	source.Value = "Schedules Direct"

	var info xml.Attr
	info.Name = xml.Name{Local: "source-info-url"}
	info.Value = "http://schedulesdirect.org"

	buf := &bytes.Buffer{}
	buf.WriteString(xml.Header)

	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")

	he := func(err error) {
		if err != nil {
			logger.Error("unable to encode the XML", "error", err)
		}
	}

	if err = Config.Open(); err != nil {
		return
	}
	if err = Cache.Open(); err != nil {
		return
	}

	Cache.Init()
	if err = Cache.Open(); err != nil {
		logger.Error("unable to open the cache", "error", err)
		return
	}

	logger.Info("Create XMLTV File", "filename", Config.Files.XMLTV)
	he(enc.EncodeToken(xml.StartElement{Name: xml.Name{Local: "tv"}, Attr: []xml.Attr{generator, source, info}}))

	// Channels
	for _, cache := range Cache.Channel {
		var xmlCha channel
		xmlCha.ID = fmt.Sprintf("%s.schedulesdirect.org", cache.StationID)
		xmlCha.Icon = cache.getLogo()
		xmlCha.DisplayName = append(xmlCha.DisplayName, DisplayName{Value: cache.Callsign})
		xmlCha.DisplayName = append(xmlCha.DisplayName, DisplayName{Value: cache.Name})
		he(enc.Encode(xmlCha))
	}

	// Programmes
	for _, cache := range Cache.Channel {
		programmes := getProgram(cache)
		he(enc.Encode(programmes))
	}

	he(enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: "tv"}}))
	he(enc.Flush())

	// Write file
	if err = os.WriteFile(Config.Files.XMLTV, buf.Bytes(), 0644); err != nil {
		panic(err)
	}
	return
}

// Channel logo
func (channel *EPGoCache) getLogo() (icon Icon) {
	icon.Src = channel.Logo.URL
	icon.Height = channel.Logo.Height
	icon.Width = channel.Logo.Width
	return
}

func getProgram(channel EPGoCache) (p []Programme) {
	if schedule, ok := Cache.Schedule[channel.StationID]; ok {
		for _, s := range schedule {
			var pro Programme

			countryCode := Config.GetLineupCountry(channel.StationID)

			// Channel ID
			pro.Channel = fmt.Sprintf("%s.schedulesdirect.org", channel.StationID)

			// Start and stop time
			timeLayout := "2006-01-02 15:04:05 +0000 UTC"
			t, err := time.Parse(timeLayout, s.AirDateTime.Format(timeLayout))
			if err != nil {
				logger.Error("unable to parse the time", "error", err)
				return
			}
			dateArray := strings.Fields(t.String())
			offset := " " + dateArray[2]
			pro.Start = t.Format("20060102150405") + offset
			pro.Stop = t.Add(time.Second*time.Duration(s.Duration)).Format("20060102150405") + offset

			// Title
			lang := "en"
			if len(channel.BroadcastLanguage) != 0 {
				lang = channel.BroadcastLanguage[0]
			}
			pro.Title = Cache.GetTitle(s.ProgramID, lang)

			// Mini tags in title
			if s.LiveTapeDelay == "Live" && Config.Options.LiveIcons {
				pro.Title[0].Value = pro.Title[0].Value + " ᴸᶦᵛᵉ"
			}
			if s.New && s.LiveTapeDelay != "Live" && Config.Options.LiveIcons {
				pro.Title[0].Value = pro.Title[0].Value + " ᴺᵉʷ"
			}

			// Sub title, description, credits, categories
			pro.SubTitle = Cache.GetSubTitle(s.ProgramID, pro.SubTitle.Value)
			pro.Desc = Cache.GetDescs(s.ProgramID, pro.SubTitle.Value)
			pro.Credits = Cache.GetCredits(s.ProgramID)
			pro.Categorys = Cache.GetCategory(s.ProgramID)

			// Language and episode numbers
			pro.Language = lang
			pro.EpisodeNums = Cache.GetEpisodeNum(s.ProgramID)

			// Icon: use proxy URL in ProxyMode; otherwise fall back to predownload or raw SD URL.
			var imageURL string
			icons := Cache.GetIcon(s.ProgramID)
			if len(icons) != 0 {
				if Config.Options.Images.ProxyMode && Config.Server.Enable {
					// If Proxy Base URL is set (e.g., https://epgo.example.com), use it verbatim.
					base := strings.TrimRight(Config.Options.Images.ProxyBaseURL, "/")
					if base == "" {
						// Fallback to local HTTP (behind your reverse proxy)
						base = "http://" + Config.Server.Address + ":" + Config.Server.Port
					}
					imageURL = base + "/proxy/sd/" + s.ProgramID
				} else if Config.Options.Images.Download {
					imageURL = "http://" + Config.Server.Address + ":" + Config.Server.Port + "/" + s.ProgramID + ".jpg"
				} else {
					// Raw SD URL (expiring token) – avoid when possible
					imageURL = icons[0].Src
				}
			}

			if imageURL == "" && Config.Options.Images.Tmdb.Enable {
				imageURL, err = tmdb.SearchItem(logger, pro.Title[0].Value, pro.EpisodeNums[0].Value[0:2], Config.Options.Images.Tmdb.ApiKey, Config.Files.TmdbCacheFile)
				if err != nil {
					logger.Error("could not connect to tmdb. check your api key", "error", err)
				}
			}
			pro.Icon = []Icon{{Src: imageURL}}

			// Ratings
			pro.Rating = Cache.GetRating(s.ProgramID, countryCode)

			// Video
			for _, v := range s.VideoProperties {
				switch strings.ToLower(v) {
				case "hdtv", "sdtv", "uhdtv", "3d":
					pro.Video.Quality = strings.ToUpper(v)
				}
			}

			// Audio
			for _, a := range s.AudioProperties {
				switch a {
				case "stereo", "dvs":
					pro.Audio.Stereo = "stereo"
				case "DD 5.1", "Atmos":
					pro.Audio.Stereo = "dolby digital"
				case "Dolby":
					pro.Audio.Stereo = "dolby"
				case "dubbed", "mono":
					pro.Audio.Stereo = "mono"
				default:
					pro.Audio.Stereo = "mono"
				}
			}

			// New / previously shown
			if s.New {
				pro.New = &New{Value: ""}
			} else {
				pro.PreviouslyShown = Cache.GetPreviouslyShown(s.ProgramID)
			}

			// Live
			if s.LiveTapeDelay == "Live" {
				pro.Live = &Live{Value: ""}
			}

			p = append(p, pro)
		}
	}
	return
}
