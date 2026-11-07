package external

import (
	"fmt"
	"sort"
	"time"
)

func (p *Plugin) vodSeriesEpisodes(session *vodSession, show vodItem, language string, random bool, seeds ...uint64) ([]vodItem, error) {
	rng := vodRandom(seeds...)
	spec := p.cfg.VOD.Series
	if spec == nil || spec.PageSize <= 0 {
		return nil, fmt.Errorf("episode catalogue is not configured")
	}
	variables := p.vodVariables(session)
	variables["content"] = map[string]any{"id": show.ID, "title": show.Title}
	variables["language"] = language
	variables["page_size"] = spec.PageSize
	response, err := p.vodJSON(session, spec.Seasons.Request, variables)
	if err != nil {
		return nil, err
	}
	seasons := vodReadItems(response, spec.Seasons)
	sort.SliceStable(seasons, func(i, j int) bool { return seasons[i].Order > seasons[j].Order })
	if random {
		rng.Shuffle(len(seasons), func(i, j int) { seasons[i], seasons[j] = seasons[j], seasons[i] })
	}
	for _, season := range seasons {
		variables["season"] = map[string]any{"id": season.ID, "order": season.Order, "total_items": season.TotalItems}
		pages := []int{1}
		if last := (season.TotalItems + spec.PageSize - 1) / spec.PageSize; last > 1 {
			pages = append(pages, last)
		}
		if random {
			pages = []int{1 + rng.IntN(max(1, (season.TotalItems+spec.PageSize-1)/spec.PageSize))}
		}
		seen := make(map[string]bool)
		var episodes []vodItem
		for _, page := range pages {
			variables["page"] = page
			response, err := p.vodJSON(session, spec.Episodes.Request, variables)
			if err != nil {
				return nil, err
			}
			for _, episode := range vodReadItems(response, spec.Episodes) {
				if seen[episode.ID] || episode.AssetType != p.cfg.VOD.AssetTypes["episode"] {
					continue
				}
				if episode.TVShow != nil && episode.TVShow.ID != show.ID {
					continue
				}
				if episode.ReleaseDate != "" && episode.ReleaseDate[:min(10, len(episode.ReleaseDate))] > time.Now().Format("2006-01-02") {
					continue
				}
				seen[episode.ID] = true
				episode.TVShow = &show
				episodes = append(episodes, episode)
			}
		}
		sort.SliceStable(episodes, func(i, j int) bool {
			if episodes[i].EpisodeNumber != episodes[j].EpisodeNumber {
				return episodes[i].EpisodeNumber > episodes[j].EpisodeNumber
			}
			return episodes[i].ReleaseDate > episodes[j].ReleaseDate
		})
		if random {
			rng.Shuffle(len(episodes), func(i, j int) { episodes[i], episodes[j] = episodes[j], episodes[i] })
		}
		if len(episodes) > 0 {
			return episodes[:min(5, len(episodes))], nil
		}
	}
	return nil, fmt.Errorf("matched show has no released episodes in its episode catalogue")
}
