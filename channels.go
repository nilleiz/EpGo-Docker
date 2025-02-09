package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/manifoldco/promptui"
)

func (e *Entry) manageChannels(sd *SD) (err error) {

	defer func() {
		Config.Save()
		Cache.Save()
	}()

	var menu Menu
	var entry Entry

	err = Cache.Open()
	if err != nil {
		ShowErr(err)
		return
	}

	Cache.Init()

	menu.Entry = make(map[int]Entry)

	menu.Select = getMsg(0204)
	menu.Headline = e.Value

	// Cancel
	entry.Key = 0
	entry.Value = getMsg(0200)
	menu.Entry[0] = entry

	index := 1 // Start index at 1 to avoid conflict with the "Cancel" option
	for _, lineup := range sd.Resp.Status.Lineups {
		entry.Key = index
		entry.Value = fmt.Sprintf("%s [%s]", lineup.Name, lineup.Lineup)
		entry.Lineup = lineup.Lineup

		menu.Entry[index] = entry
		index++
	}

	selection := menu.Show()

	switch selection {
	case 0:
		return // Cancel selected
	default:
		entry = menu.Entry[selection]
	}

	sd.Req.Parameter = fmt.Sprintf("/%s", entry.Lineup)
	sd.Req.Type = "GET"

	err = sd.Lineups()
	if err != nil {
		ShowErr(err)
		return
	}

	entry.headline()
	var channelNames []string
	var existing map[string]bool

	for _, station := range sd.Resp.Lineup.Stations {
		channelNames = append(channelNames, station.Name)
	}

	sort.Strings(channelNames)

	Config.GetChannels()

	existing = make(map[string]bool)
	for _, id := range Config.ChannelIDs {
		existing[id] = true
	}

	// Prepare items for promptui
	var promptItems []map[string]interface{}
	for _, cName := range channelNames {
		for _, station := range sd.Resp.Lineup.Stations {
			if cName == station.Name {
				status := "-"
				if existing[station.StationID] {
					status = "+"
				}
				promptItems = append(promptItems, map[string]interface{}{
					"Name":     station.Name,
					"ID":       station.StationID,
					"Display":  fmt.Sprintf("[%s] %s [%s] %v", status, station.Name, station.StationID, station.BroadcastLanguage), //Pre-calculate Display
					"Status":   status,
					"Lineup":   entry.Lineup,
					"Language": station.BroadcastLanguage, // Store language for searching
				})
				break
			}
		}
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "> {{ .Display | green }}",
		Inactive: "  {{ .Display }}",
		Selected: "{{ .Display | yellow }}",
		Details: `
--------- Channel Details ----------
{{ "Name:" | faint }}	{{ .Name }}
{{ "ID:" | faint }}	{{ .ID }}
`,
	}

	prompt := promptui.Select{
		Label:     "Available Channels (Space to select, Enter to finish)",
		Items:     promptItems,
		Size:      20,
		Templates: templates,

	}

	// custom searcher function based on channel name, station id and language
	prompt.Searcher = func(input string, index int) bool {
		item := promptItems[index]
		name := item["Name"].(string)

		return caseInsensitiveContains(name, input) 
	}

	for {
		index, _, err := prompt.Run()

		if err != nil {
			if err == promptui.ErrInterrupt {
				break // User pressed Ctrl+C, exit loop
			}
			if err == promptui.ErrEOF { // Check for Ctrl+D (EOF)
				break
			}
			log.Printf("Prompt failed %v\n", err)
			return err
		}

		// Toggle selection status
		item := promptItems[index]
		if item["Status"] == "+" {
			item["Status"] = "-"
		} else {
			item["Status"] = "+"
		}

		// Update the Display field *after* toggling the status
		item["Display"] = fmt.Sprintf("[%s] %s [%s] %v", item["Status"], item["Name"], item["ID"], item["Language"])

		// Update promptItems to reflect the change
		promptItems[index] = item

		// prompt.Items = promptItems // Not strictly necessary, but good for consistency

	}
	// Collect selected channels and add/remove accordingly.
	for _, item := range promptItems {
		if item["Status"] == "+" {
			ch := channel{Name: item["Name"].(string), ID: item["ID"].(string), Lineup: item["Lineup"].(string)}
			Config.AddChannel(&ch)
		} else {
			ch := channel{Name: item["Name"].(string), ID: item["ID"].(string), Lineup: item["Lineup"].(string)}
			Config.RemoveChannel(&ch)
		}
	}

	return
}

// Helper function for case-insensitive contains
func caseInsensitiveContains(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

func (c *config) AddChannel(ch *channel) {
	for _, existingCh := range c.Station {
		if existingCh.ID == ch.ID {
			return // Channel already exists; don't add it again
		}
	}
	c.Station = append(c.Station, *ch)
}

func (c *config) RemoveChannel(ch *channel) {
	var tmp []channel
	for _, old := range c.Station {
		if old.ID != ch.ID {
			tmp = append(tmp, old)
		}
	}
	c.Station = tmp
}

func (c *config) GetChannels() {
	c.ChannelIDs = []string{}
	for _, channel := range c.Station {
		c.ChannelIDs = append(c.ChannelIDs, channel.ID)
	}
}
