package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestOrderedChannelDisplayNames(t *testing.T) {
	tests := []struct {
		name     string
		station  string
		callsign string
		want     []string
	}{
		{
			name:     "station name is first then callsign",
			station:  "Eurosport 1",
			callsign: "EUROSGR",
			want:     []string{"Eurosport 1", "EUROSGR"},
		},
		{
			name:     "fallback to callsign when station name missing",
			station:  "",
			callsign: "EUROSGR",
			want:     []string{"EUROSGR"},
		},
		{
			name:     "avoid duplicate values",
			station:  "EUROSGR",
			callsign: "EUROSGR",
			want:     []string{"EUROSGR"},
		},
		{
			name:     "trimmed station name stays first",
			station:  "  Das Erste HD  ",
			callsign: "ARDH",
			want:     []string{"Das Erste HD", "ARDH"},
		},
		{
			name:     "avoid duplicate values case insensitive",
			station:  "Eurosport 1",
			callsign: "eurosport 1",
			want:     []string{"Eurosport 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orderedChannelDisplayNames(tt.station, tt.callsign)
			if len(got) != len(tt.want) {
				t.Fatalf("orderedChannelDisplayNames() length = %d, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i].Value != tt.want[i] {
					t.Fatalf("orderedChannelDisplayNames()[%d] = %q, want %q", i, got[i].Value, tt.want[i])
				}
			}
		})
	}
}

func TestConfiguredStationName(t *testing.T) {
	original := Config
	defer func() { Config = original }()

	Config.Station = []channel{{Name: "Das Erste HD", ID: "90447", Lineup: "DEU-1000097-DEFAULT"}}

	if got := configuredStationName("90447"); got != "Das Erste HD" {
		t.Fatalf("configuredStationName() = %q, want %q", got, "Das Erste HD")
	}

	if got := configuredStationName("missing"); got != "" {
		t.Fatalf("configuredStationName() = %q, want empty string", got)
	}

	if got := configuredStationName("90447.schedulesdirect.org"); got != "Das Erste HD" {
		t.Fatalf("configuredStationName() with suffix = %q, want %q", got, "Das Erste HD")
	}
}

func TestBuildXMLChannelDisplayNameOrderMarshalledXML(t *testing.T) {
	original := Config
	defer func() { Config = original }()

	Config.Station = []channel{
		{Name: "Das Erste HD", ID: "90447", Lineup: "DEU-1000097-DEFAULT"},
		{Name: "one HD", ID: "90457", Lineup: "DEU-1000097-DEFAULT"},
		{Name: "Eurosport 1", ID: "66603", Lineup: "DEU-1000097-DEFAULT"},
	}

	tests := []struct {
		name       string
		cache      EPGoCache
		wantFirst  string
		wantSecond string
	}{
		{
			name:       "ard configured name before callsign",
			cache:      EPGoCache{StationID: "90447", Name: "ARDGRHD", Callsign: "ARDGRHD"},
			wantFirst:  "Das Erste HD",
			wantSecond: "ARDGRHD",
		},
		{
			name:       "one configured name before callsign",
			cache:      EPGoCache{StationID: "90457", Name: "ONEARHD", Callsign: "ONEARHD"},
			wantFirst:  "one HD",
			wantSecond: "ONEARHD",
		},
		{
			name:       "eurosport configured name before callsign",
			cache:      EPGoCache{StationID: "66603", Name: "EUROSGR", Callsign: "EUROSGR"},
			wantFirst:  "Eurosport 1",
			wantSecond: "EUROSGR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := buildXMLChannel(tt.cache)
			out, err := xml.Marshal(ch)
			if err != nil {
				t.Fatalf("xml.Marshal() error = %v", err)
			}
			xmlStr := string(out)
			expectedOrder := "<display-name>" + tt.wantFirst + "</display-name><display-name>" + tt.wantSecond + "</display-name>"
			reversedOrder := "<display-name>" + tt.wantSecond + "</display-name><display-name>" + tt.wantFirst + "</display-name>"

			if !strings.Contains(xmlStr, expectedOrder) {
				t.Fatalf("marshalled XML missing expected display-name order: %s in %s", expectedOrder, xmlStr)
			}
			if strings.Contains(xmlStr, reversedOrder) {
				t.Fatalf("marshalled XML contains wrong display-name order: %s in %s", reversedOrder, xmlStr)
			}

			var decoded channel
			if err := xml.Unmarshal(out, &decoded); err != nil {
				t.Fatalf("xml.Unmarshal() error = %v", err)
			}
			if len(decoded.DisplayName) < 2 {
				t.Fatalf("decoded channel %s has %d display-names, want at least 2", decoded.ID, len(decoded.DisplayName))
			}
			if decoded.DisplayName[0].Value != tt.wantFirst {
				t.Fatalf("decoded first display-name for %s = %q, want %q", decoded.ID, decoded.DisplayName[0].Value, tt.wantFirst)
			}
			if decoded.DisplayName[1].Value != tt.wantSecond {
				t.Fatalf("decoded second display-name for %s = %q, want %q", decoded.ID, decoded.DisplayName[1].Value, tt.wantSecond)
			}
		})
	}
}
