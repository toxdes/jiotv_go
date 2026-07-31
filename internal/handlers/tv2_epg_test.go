package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
)

func TestParseTV2XMLTV(t *testing.T) {
	data := `<tv>
  <channel id="plugin-news-hd"><display-name>Example News HD</display-name></channel>
  <channel id="provider-101"><display-name>Example News</display-name></channel>
  <channel id="provider-101-hd"><display-name>Example News HD</display-name></channel>
  <programme channel="plugin-news-hd" start="20260731100000 +0530" stop="20260731110000 +0530"><title>Expired bulletin</title></programme>
  <programme channel="movie-channel" start="20260731150000 +0530" stop="20260731153000 +0530"><title>Movie One</title></programme>
  <programme channel="movie-channel" start="20260731143000 +0530" stop="20260731150000 +0530"><title>Movie Zero</title></programme>
  <programme channel="provider-101" start="20260731150000 +0530" stop="20260731153000 +0530"><title>News Bulletin</title></programme>
  <programme channel="provider-101-hd" start="20260731150000 +0530" stop="20260731153000 +0530"><title>High Definition Bulletin</title></programme>
  <programme channel="missing-title" start="20260731150000 +0530" stop="20260731153000 +0530"><title></title></programme>
</tv>`

	now := time.Date(2026, time.July, 31, 14, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60)).UnixMilli()
	programmes, err := parseTV2XMLTV(strings.NewReader(data), now)
	if err != nil {
		t.Fatalf("parseTV2XMLTV() error = %v", err)
	}

	guide := programmes.programmes["movie-channel"]
	if len(guide) != 2 {
		t.Fatalf("programme count = %d, want 2", len(guide))
	}
	if guide[0].ShowName != "Movie Zero" || guide[1].ShowName != "Movie One" {
		t.Fatalf("programmes were not sorted: %#v", guide)
	}
	if guide[0].StartEpoch >= guide[1].StartEpoch {
		t.Fatalf("programme times were not parsed in chronological order: %#v", guide)
	}
	if got := programmes.channelIDsByName[normaliseTV2EPGName("Example News HD")]; len(got) != 3 || got[1] != "provider-101" {
		t.Fatalf("name lookup candidates = %#v, want matching source channels", got)
	}
	if got := tv2EPGChannelCandidates(programmes, "Example News HD"); len(got) != 3 || got[0] != "plugin-news-hd" || got[1] != "provider-101-hd" || got[2] != "provider-101" {
		t.Fatalf("ordered name lookup candidates = %#v, want quality-preserving candidates first", got)
	}

	guide = findTV2ProgrammeGuide(programmes, "plugin-news-hd", "Example News HD", true, now)
	if len(guide) != 1 || guide[0].ShowName != "High Definition Bulletin" {
		t.Fatalf("fresh name-matched guide = %#v, want the exact-name provider guide", guide)
	}
	if guide = findTV2ProgrammeGuide(programmes, "plugin-news-hd", "Example News HD", false, now); len(guide) != 0 {
		t.Fatalf("non-plugin guide = %#v, want no name-matched guide", guide)
	}
}

func TestPrioritiseTV2EPGSources(t *testing.T) {
	primary := "https://primary.invalid/guide.xml.gz"
	alternate := "https://alternate.invalid/guide.xml.gz"

	if got := prioritiseTV2EPGSources(primary, alternate, true); len(got) != 2 || got[0] != alternate || got[1] != primary {
		t.Fatalf("plugin source order = %#v, want alternate then primary", got)
	}
	if got := prioritiseTV2EPGSources(primary, alternate, false); len(got) != 2 || got[0] != primary || got[1] != alternate {
		t.Fatalf("standard source order = %#v, want primary then alternate", got)
	}
}

func TestNormaliseTV2EPGNameIgnoresFormattingAndQuality(t *testing.T) {
	if got, want := normaliseTV2EPGName("Example Channel HD"), "examplechannel"; got != want {
		t.Fatalf("normalised name = %q, want %q", got, want)
	}
	if got, want := normaliseTV2EPGName("ExampleChannel SD"), "examplechannel"; got != want {
		t.Fatalf("normalised compact name = %q, want %q", got, want)
	}
}

func TestFindTV2JioEPGChannelID(t *testing.T) {
	jioChannels := []television.Channel{
		{ID: "101", Name: "Example Channel"},
		{ID: "102", Name: "Example Channel HD"},
	}
	if got := findTV2JioEPGChannelID("examplechannel hd", jioChannels); got != "102" {
		t.Fatalf("matched EPG channel ID = %q, want %q", got, "102")
	}
}

func TestFindTV2JioEPGChannelIDInXMLTV(t *testing.T) {
	index := tv2ExternalEPGIndex{
		channelIDsByName: map[string][]string{
			normaliseTV2EPGName("Example Channel HD"): {"plugin-channel", "101", "provider-channel"},
		},
		channelIDsByDisplayName: map[string][]string{
			normaliseTV2EPGDisplayName("Example Channel HD"): {"plugin-channel", "101"},
		},
		channelNamesByID: map[string]string{
			"plugin-channel":   "Example Channel HD",
			"101":              "Example Channel HD",
			"provider-channel": "Example Channel",
		},
	}
	if got := findTV2JioEPGChannelIDInXMLTV(index, "examplechannel hd"); got != "101" {
		t.Fatalf("matched XMLTV Jio EPG channel ID = %q, want %q", got, "101")
	}
}

func TestAddTV2EPGProgrammeKeepsEarliestProgrammes(t *testing.T) {
	programmesByChannel := make(map[string][]tv2EPGProgramme)
	for i := 0; i <= tv2EPGProgrammesPerChannel; i++ {
		addTV2EPGProgramme(programmesByChannel, "example-channel", tv2EPGProgramme{StartEpoch: int64(i), ShowName: "Programme"})
	}
	addTV2EPGProgramme(programmesByChannel, "example-channel", tv2EPGProgramme{StartEpoch: -1, ShowName: "Earlier programme"})

	got := programmesByChannel["example-channel"]
	if len(got) != tv2EPGProgrammesPerChannel {
		t.Fatalf("programme count = %d, want %d", len(got), tv2EPGProgrammesPerChannel)
	}
	for _, programme := range got {
		if programme.StartEpoch == int64(tv2EPGProgrammesPerChannel) {
			t.Fatalf("latest programme was retained: %#v", got)
		}
	}
}
