package main

import "testing"

func TestBuildXMLTVDisplayName(t *testing.T) {
	tests := []struct {
		name      string
		station   string
		callsign  string
		stationID string
		want      string
	}{
		{
			name:      "configured station name first with identifiers",
			station:   "Das Erste HD",
			callsign:  "ARDGRHD",
			stationID: "90447",
			want:      "Das Erste HD (ARDGRHD,90447)",
		},
		{
			name:      "fallback to callsign when station name missing",
			station:   "",
			callsign:  "ARDGRHD",
			stationID: "90447",
			want:      "ARDGRHD (ARDGRHD,90447)",
		},
		{
			name:      "only station name when identifiers unavailable",
			station:   "Das Erste HD",
			callsign:  "",
			stationID: "",
			want:      "Das Erste HD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildXMLTVDisplayName(tt.station, tt.callsign, tt.stationID)
			if got != tt.want {
				t.Fatalf("buildXMLTVDisplayName() = %q, want %q", got, tt.want)
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
}
