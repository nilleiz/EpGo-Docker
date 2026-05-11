package main

import "testing"

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
}
