package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jiotv-go/jiotv_go/v3/pkg/plugins/external"
)

var pluginVODGuides = struct {
	sync.Mutex
	entries map[string]struct {
		Guide   []external.Programme
		Expires time.Time
	}
}{entries: make(map[string]struct {
	Guide   []external.Programme
	Expires time.Time
})}

func PluginVODSchedule(id, name string) ([]external.Programme, error) {
	key := id + "|" + name
	pluginVODGuides.Lock()
	defer pluginVODGuides.Unlock()
	if entry, ok := pluginVODGuides.entries[key]; ok && time.Now().Before(entry.Expires) {
		return append([]external.Programme(nil), entry.Guide...), nil
	}
	var programmes []external.Programme
	jioID := id
	if _, err := strconv.Atoi(jioID); err != nil {
		jioID = ""
		if index, err := getTV2ExternalEPG(tv2JioEPGMappingSource()); err == nil {
			jioID = findTV2JioEPGChannelIDInXMLTV(index, name)
		}
	}
	if numeric, err := strconv.Atoi(jioID); err == nil {
		for offset := 0; offset < 2; offset++ {
			body, _, status, err := webEPGWithCorrectedDay(numeric, offset)
			if err != nil || status != http.StatusOK {
				continue
			}
			var data struct {
				EPG []external.Programme `json:"epg"`
			}
			if json.Unmarshal(body, &data) == nil {
				programmes = append(programmes, data.EPG...)
			}
		}
	}
	if len(programmes) == 0 {
		for _, source := range tv2EPGSources(id) {
			index, err := getTV2ExternalEPG(source)
			if err != nil {
				continue
			}
			guide := findTV2ProgrammeGuide(index, id, name, true, time.Now().UnixMilli())
			for _, entry := range guide {
				programmes = append(programmes, external.Programme{Title: entry.ShowName, StartEpoch: entry.StartEpoch, EndEpoch: entry.EndEpoch})
			}
			if len(programmes) != 0 {
				break
			}
		}
	}
	if len(programmes) == 0 {
		return nil, fmt.Errorf("no programme guide is available for this external channel")
	}
	if len(pluginVODGuides.entries) > 512 {
		clear(pluginVODGuides.entries)
	}
	pluginVODGuides.entries[key] = struct {
		Guide   []external.Programme
		Expires time.Time
	}{programmes, time.Now().Add(5 * time.Minute)}
	return append([]external.Programme(nil), programmes...), nil
}
