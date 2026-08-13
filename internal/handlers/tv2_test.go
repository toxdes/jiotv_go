package handlers

import "testing"

func TestTV2PlayerURL(t *testing.T) {
	originalEnableDRM := EnableDRM
	t.Cleanup(func() {
		EnableDRM = originalEnableDRM
	})

	tests := []struct {
		name      string
		enableDRM bool
		channelID string
		pluginID  string
		isPlugin  bool
		expectURL string
	}{
		{
			name:      "standard channel uses DRM player when enabled",
			enableDRM: true,
			channelID: "123",
			expectURL: "/mpd/123?q=high&af=1",
		},
		{
			name:      "standard channel uses HLS player when DRM is disabled",
			enableDRM: false,
			channelID: "123",
			expectURL: "/player/123?q=high&af=1",
		},
		{
			name:      "plugin channel uses plugin player regardless of DRM setting",
			enableDRM: false,
			channelID: "plugin-channel",
			pluginID:  "provider",
			isPlugin:  true,
			expectURL: "/provider/player/plugin-channel?q=high&af=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			EnableDRM = tt.enableDRM
			if got := tv2PlayerURL(tt.channelID, tt.pluginID, tt.isPlugin); got != tt.expectURL {
				t.Fatalf("tv2PlayerURL() = %q, want %q", got, tt.expectURL)
			}
		})
	}
}
