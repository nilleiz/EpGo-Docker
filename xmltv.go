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

// CreateXMLTV : Create XMLTV file from cache file
func CreateXMLTV(filename string) (err error) {

	defer func() {
		runtime.GC()
	}()

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
			return
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

	// XMLTV Channels
	for _, cache := range Cache.Channel {
		var xmlCha channel // struct_config.go
		xmlCha.ID = fmt.Sprintf("%s.schedulesdirect.org", cache.StationID)
		xmlCha.Icon = cache.getLogo()
		xmlCha.DisplayName = append(xmlCha.DisplayName, DisplayName{Value: cache.Callsign})
		xmlCha.DisplayName = append(xmlCha.DisplayName, DisplayName{Value: cache.Name})
		he(enc.Encode(xmlCha))
	}

	// XMLTV Programs
	for _, cache := range Cache.Channel {
		progs := getProgram(cache)
		he(enc.Encode(progs))
	}

	he(enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: "tv"}}))
	he(enc.Flush())

	// write the whole body at once
	if err = os.WriteFile(Config.Files.XMLTV, buf.Bytes(), 0644); err != nil {
		panic(err)
	}
	return
}

// Channel infos
func (channel *EPGoCache) getLogo() (icon Icon) {
	icon.Src = channel.Logo.URL
	icon.Height = channel.Logo.Height
	icon.Width = channel.Logo.Width
	return
}

func getProgram(channel EPGoCache) (p []Programme) {
	if schedule, ok := Cache.Schedule[channel.StationID]; !ok {
		return
	} else {
		for _, s := range schedule {

			var pro Programme

			countryCode := Config.GetLineupCountry(channel.StationID)

			// Channel ID
			pro.Channel = fmt.Sprintf("%s.schedulesdirect.org", channel.StationID)

			// Start and Stop time
			timeLayout := "2006-01-02 15:04:05 +0000 UTC"
			t, err := time.Parse(timeLayout, s.AirDateTime.Format(timeLayout))
			if err != nil {
				logger.Error("unable to parse the time", "error", err)
				return
			}
			dateArray := strings.Fields(t.String())
			offset := " " + dateArray[2]
			startTime := t.Format("20060102150405") + offset
			stopTime := t.Add(time.Second*time.Duration(s.Duration)).Format("20060102150405") + offset
			pro.Start = startTime
			pro.Stop = stopTime

			// Title
			lang := "en"
			if len(channel.BroadcastLanguage) != 0 {
				lang = channel.BroadcastLanguage[0]
			}
			pro.Title = Cache.GetTitle(s.ProgramID, lang)

			// New and Live guide mini-icons
			if s.LiveTapeDelay == "Live" && Config.Options.LiveIcons {
				pro.Title[0].Value = pro.Title[0].Value + " ᴸᶦᵛᵉ"
			}
			if s.New && s.LiveTapeDelay != "Live" && Config.Options.LiveIcons {
				pro.Title[0].Value = pro.Title[0].Value + " ᴺᵉʷ"
			}

			// Sub Title
			pro.SubTitle = Cache.GetSubTitle(s.ProgramID, pro.SubTitle.Value)

			// Description
			pro.Desc = Cache.GetDescs(s.ProgramID, pro.SubTitle.Value)

			// Credits
			pro.Credits = Cache.GetCredits(s.ProgramID)

			// Category
			pro.Categorys = Cache.GetCategory(s.ProgramID)

			// Language
			pro.Language = lang

			// EpisodeNum
			pro.EpisodeNums = Cache.GetEpisodeNum(s.ProgramID)

			// =========================
			// Icon selection (PINNED SD → TMDb fallback → blank)
			// =========================
			imageURL := ""

			if Config.Options.Images.ProxyMode && Config.Server.Enable {
				// Try SD pin
				if imageID, _, ok := Cache.GetChosenSDImage(s.ProgramID); ok && imageID != "" {
					base := strings.TrimRight(Config.Options.Images.ProxyBaseURL, "/")
					if base == "" {
						base = "http://" + Config.Server.Address + ":" + Config.Server.Port
					}
					imageURL = base + "/proxy/sd/" + s.ProgramID + "/" + imageID
				} else {
					// No SD image -> DO NOT force legacy proxy; allow TMDb fallback below
				}
			} else {
				// Non-proxy mode: direct SD or pre-downloaded
				icons := Cache.GetIcon(s.ProgramID)
				if len(icons) != 0 {
					if Config.Options.Images.Download {
						// Eager download mode
						imageURL = "http://" + Config.Server.Address + ":" + Config.Server.Port + "/" + s.ProgramID + ".jpg"
					} else {
						// Raw SD URL (expiring token)
						imageURL = icons[0].Src
					}
				}
			}

			// TMDb fallback (only if nothing from SD)
			if imageURL == "" && Config.Options.Images.Tmdb.Enable {
				imageURL, err = tmdb.SearchItem(
					logger,
					pro.Title[0].Value,
					func() string {
						if len(pro.EpisodeNums) > 0 && len(pro.EpisodeNums[0].Value) >= 2 {
							return pro.EpisodeNums[0].Value[0:2]
						}
						return ""
					}(),
					Config.Options.Images.Tmdb.ApiKey,
					Config.Files.TmdbCacheFile,
				)
				if err != nil {
					logger.Error("could not connect to tmdb. check your api key", "error", err)
				}
			}

			pro.Icon = []Icon{{Src: imageURL}}

			// Rating
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

			// New / PreviouslyShown
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
