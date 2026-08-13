package handlers

import (
	"testing"

	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
)

func TestParseTVFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{name: "empty", input: "", want: nil},
		{name: "multiple values", input: "1, 2,4", want: []int{1, 2, 4}},
		{name: "invalid values are ignored", input: "1,nope,3", want: []int{1, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTVFilter(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseTVFilter(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parseTVFilter(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTVPlaybackURLs(t *testing.T) {
	originalEnableDRM := EnableDRM
	originalDRMList := drmList
	t.Cleanup(func() {
		EnableDRM = originalEnableDRM
		drmList = originalDRMList
	})

	tests := []struct {
		name       string
		channel    television.Channel
		quality    string
		enableDRM  bool
		drmIDs     []string
		wantPlayer string
		wantStream string
		wantUse    bool
	}{
		{
			name:       "ordinary hls channel uses requested quality",
			channel:    television.Channel{ID: "123"},
			quality:    "high",
			enableDRM:  true,
			drmIDs:     []string{"456"},
			wantPlayer: "/player/123?q=high",
			wantStream: "/live/high/123.m3u8",
		},
		{
			name:       "known drm channel uses upstream player",
			channel:    television.Channel{ID: "123"},
			quality:    "medium",
			enableDRM:  true,
			drmIDs:     []string{"123"},
			wantPlayer: "/mpd/123?q=medium",
			wantStream: "/live/medium/123.m3u8",
			wantUse:    true,
		},
		{
			name:       "drm disabled stays hls",
			channel:    television.Channel{ID: "123"},
			quality:    "low",
			enableDRM:  false,
			drmIDs:     []string{"123"},
			wantPlayer: "/player/123?q=low",
			wantStream: "/live/low/123.m3u8",
		},
		{
			name:       "upstream key url marks drm channel",
			channel:    television.Channel{ID: "123", KeyURL: "/live/key/123"},
			quality:    "high",
			enableDRM:  true,
			wantPlayer: "/mpd/123?q=high",
			wantStream: "/live/high/123.m3u8",
			wantUse:    true,
		},
		{
			name:       "plugin stays on plugin route",
			channel:    television.Channel{ID: "plugin-1", IsCustom: true, PluginID: "provider"},
			quality:    "auto",
			enableDRM:  true,
			drmIDs:     []string{"plugin-1"},
			wantPlayer: "/provider/player/plugin-1?q=auto",
			wantStream: "/provider/plugin-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			EnableDRM = tt.enableDRM
			drmList = tt.drmIDs

			playerURL, streamURL, usePlayer := tvPlaybackURLs(tt.channel, tt.quality)
			if playerURL != tt.wantPlayer || streamURL != tt.wantStream || usePlayer != tt.wantUse {
				t.Errorf("tvPlaybackURLs() = (%q, %q, %t), want (%q, %q, %t)", playerURL, streamURL, usePlayer, tt.wantPlayer, tt.wantStream, tt.wantUse)
			}
		})
	}
}
