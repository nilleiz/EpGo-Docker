package main

import (
	"fmt"
	"log"
	"sort"

	"github.com/manifoldco/promptui"
)

func (e *Entry) manageChannels(sd *SD) (err error) {

        defer func() {
                Config.Save()
                Cache.Save()
        }()

        var index, selection int

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
        entry.Key = index
        entry.Value = getMsg(0200)
        menu.Entry[index] = entry

        var ch channel

        for _, lineup := range sd.Resp.Status.Lineups {

                index++
                entry.Key = index
                entry.Value = fmt.Sprintf("%s [%s]", lineup.Name, lineup.Lineup)
                entry.Lineup = lineup.Lineup

                menu.Entry[index] = entry

        }

        selection = menu.Show()

        switch selection {

        case 0:
                return

        default:
                entry = menu.Entry[selection]
                ch.Lineup = entry.Lineup

        }

        sd.Req.Parameter = fmt.Sprintf("/%s", entry.Lineup)
        sd.Req.Type = "GET"

        err = sd.Lineups()

        sd.Req.Parameter = fmt.Sprintf("/%s", entry.Lineup)
        sd.Req.Type = "GET"

        err = sd.Lineups()

        entry.headline()
        var channelNames []string
        var existing map[string]bool
        var addThese []channel
        var removeThese []channel
		var selectedChannels []string // Keep track of selected channel names

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
        var promptItems []string
        for _, cName := range channelNames {
                for _, station := range sd.Resp.Lineup.Stations {
                        if cName == station.Name {
                                status := "-"
                                if existing[station.StationID] {
                                        status = "+"
                                }
                                promptItems = append(promptItems, fmt.Sprintf("[%s] %s [%s] %v", status, station.Name, station.StationID, station.BroadcastLanguage))
                                break
                        }
                }
        }


    prompt := promptui.Select{
        Label: "Available Channels (Enter to select, Ctrl+C to finish)",
        Items: promptItems,
        Size:  20,
        Templates: &promptui.SelectTemplates{
            Label:    "{{ . }}",
            Active:   "> {{ . | green }}",
            Inactive: "  {{ . }}",
            Selected: "{{ . | yellow }}", // Show selected items in yellow
        },
    }

    for {
        index, _, err := prompt.Run()
        if err != nil {
            if err == promptui.ErrInterrupt {
                break // User pressed Ctrl+C, exit loop
            }
            log.Fatalf("Prompt failed %v\n", err)
            return err
        }

        selectedCName := channelNames[index]

        // Check if the channel is already selected. If it is, remove it. If it is not, add it.
        alreadySelected := false
                for i, c := range selectedChannels{
                        if c == selectedCName {
                                alreadySelected = true
                                selectedChannels = append(selectedChannels[:i], selectedChannels[i+1:]...)
                                break
                        }
                }

        for _, station := range sd.Resp.Lineup.Stations {
            if selectedCName == station.Name {
                ch := channel{Name: station.Name, ID: station.StationID}

                                if alreadySelected {
                                        removeThese = append(removeThese, ch)
                                } else {
                                        addThese = append(addThese, ch)
                                        selectedChannels = append(selectedChannels, selectedCName)
                                }
                break
            }
        }
    }

        for _, ch := range addThese {
				ch.Lineup = entry.Lineup
                Config.AddChannel(&ch)
        }

        for _, ch := range removeThese {
                Config.RemoveChannel(&ch)
        }

        return
}

func (c *config) AddChannel(ch *channel) {

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
