package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Update : Update data from Schedules Direct and create the XMLTV file
func (sd *SD) Update(filename string) (err error) {

	Config.File = strings.TrimSuffix(filename, filepath.Ext(filename))

	// reads the Config file
	_, err = os.ReadFile(fmt.Sprintf("%s.yaml", Config.File))

	if err != nil {
		return
	}

	err = Config.Open()
	if err != nil {
		return
	}

	// loads default functions and variables
	err = sd.Init()
	if err != nil {
		return
	}

	if len(sd.Token) == 0 {
		// store token in a file
		token := struct {
			Token string `json:"token"`
			Date  time.Time `json:"date"`
		}{
			Token: sd.Token,
		}

		tokenF, err := os.Stat("token.json")
		if err != nil || tokenF.Size() == 0 {
			if os.IsNotExist(err) || tokenF.Size() == 0 {
				// Create the file if it doesn't exist, but don't try to read from it yet.
				err := os.WriteFile("token.json", []byte("{}"), 0644) // Write empty JSON initially
				if err != nil {
					return fmt.Errorf("creating token file: %w", err) // wrap error for context
				}
			} else {
				return fmt.Errorf("reading token file: %w", err) // wrap error for context
			}
		}
		tokenFile, err := os.OpenFile("token.json", os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("unmarshaling token file: %w", err) // wrap error for context
		}
		dec := json.NewDecoder(tokenFile)
		err = dec.Decode(&token)
		if err != nil {
			return fmt.Errorf("unmarshaling token file: %w", err) // wrap error for context
		}
		defer tokenFile.Close() // Important to close the file!

		sd.Token = token.Token
		err = sd.Status()
		if time.Since(token.Date) > time.Hour*23 || err != nil {
			logger.Debug("Current Token expired grabbing new token")
			// check this when everything is working
			if sd.Resp.Status.Code != 0 {
				return err
			}
			err = sd.Login()
			if err != nil {
				return err
			}
			token.Token = sd.Token
			token.Date = time.Now()
			if _, err := tokenFile.Seek(0, 0); err != nil {
				return fmt.Errorf("could not delete token file")
			}
			if err := tokenFile.Truncate(0); err != nil {
				return fmt.Errorf("could not delete token file")
			}
			enc := json.NewEncoder(tokenFile)
			if err := enc.Encode(token); err != nil {
				return fmt.Errorf("could not unmarshal token file")
			}
		} else {
			logger.Debug("Using cached credentials for SD")
		}
		sd.Token = token.Token
	}

	sd.GetData()

	runtime.GC()

	err = CreateXMLTV(filename)
	if err != nil {
		ShowErr(err)
		return
	}

	Cache.CleanUp()

	runtime.GC()

	return
}

// GetData : Get data from Schedules Direct
func (sd *SD) GetData() {

	var err error
	var wg sync.WaitGroup
	var count = 0

	err = Cache.Open()
	if err != nil {
		ShowErr(err)
		return
	}
	Cache.Init()

	// Channel list
	sd.Status()
	Cache.Channel = make(map[string]G2GCache)

	var lineup []string

	for _, l := range sd.Resp.Status.Lineups {
		lineup = append(lineup, l.Lineup)
	}

	for _, id := range lineup {

		sd.Req.Parameter = fmt.Sprintf("/%s", id)
		sd.Req.Type = "GET"

		sd.Lineups()

		Cache.AddStations(&sd.Resp.Body, id)

	}

	// Schedule
	showInfo("G2G", fmt.Sprintf("Download Schedule: %d Day(s)", Config.Options.Schedule))

	var limit = 5000

	var days = make([]string, 0)
	var channels = make([]interface{}, 0)

	for i := 0; i < Config.Options.Schedule; i++ {
		var nextDay = time.Now().Add(time.Hour * time.Duration(24*i))
		days = append(days, nextDay.Format("2006-01-02"))
	}

	for i, channel := range Config.Station {

		count++

		channel.Date = days
		channels = append(channels, channel)

		if count == limit || i == len(Config.Station)-1 {

			sd.Req.Data, err = json.Marshal(channels)
			if err != nil {
				ShowErr(err)
				return
			}

			sd.Schedule()

			wg.Add(1)
			go func() {

				Cache.AddSchedule(&sd.Resp.Body)

				wg.Done()

			}()

			count = 0

		}

	}

	wg.Wait()

	// Program and Metadata
	count = 0
	sd.Req.Data = []byte{}

	var types = []string{"programs", "metadata"}
	var programIds = Cache.GetRequiredProgramIDs()
	var allIDs = Cache.GetAllProgramIDs()
	var programs = make([]interface{}, 0)

	showInfo("G2G", fmt.Sprintf("Download Program Informations: New: %d / Cached: %d", len(programIds), len(allIDs)-len(programIds)))

	for _, t := range types {

		switch t {
		case "metadata":
			sd.Req.URL = fmt.Sprintf("%smetadata/programs", sd.BaseURL)
			sd.Req.Call = "metadata"
			programIds = Cache.GetRequiredMetaIDs()
			limit = 500
			showInfo("G2G", fmt.Sprintf("Download missing Metadata: %d ", len(programIds)))

		case "programs":

			sd.Req.URL = fmt.Sprintf("%sprograms", sd.BaseURL)
			sd.Req.Call = "programs"
			limit = 5000

		}

		for i, p := range programIds {

			count++

			programs = append(programs, p)

			if count == limit || i == len(programIds)-1 {

				sd.Req.Data, err = json.Marshal(programs)
				if err != nil {
					ShowErr(err)
					return
				}

				err := sd.Program()
				if err != nil {
					ShowErr(err)
				}

				wg.Add(1)

				switch t {
				case "metadata":
					go Cache.AddMetadata(&sd.Resp.Body, &wg)

				case "programs":
					go Cache.AddProgram(&sd.Resp.Body, &wg)

				}

				count = 0
				programs = make([]interface{}, 0)
				wg.Wait()

			}

		}

	}

	err = Cache.Save()
	if err != nil {
		ShowErr(err)
		return
	}
}
