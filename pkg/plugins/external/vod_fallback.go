package external

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

func (p *Plugin) vodFallbackItems(session *vodSession, rule VODFallback) ([]vodItem, error) {
	if rule.Language == "" || len(rule.Items) == 0 {
		return nil, fmt.Errorf("fallback pool is not configured")
	}
	pool := slices.Clone(rule.Items)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	var candidates []vodItem
	for _, configured := range pool {
		if configured.AssetType == p.cfg.VOD.AssetTypes["show"] {
			return []vodItem{configured}, nil
		}
		if configured.AssetType != p.cfg.VOD.AssetTypes["movie"] {
			continue
		}
		items, err := p.vodSearch(session, configured.Title, rule.Language)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ID != configured.ID || item.AssetType != p.cfg.VOD.AssetTypes["movie"] || !slices.Contains(item.Languages, rule.Language) || item.AgeRating == "" || !slices.Contains(rule.AllowedRatings, item.AgeRating) {
				continue
			}
			candidates = append(candidates, item)
			break
		}
		if len(candidates) >= 5 {
			break
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no fallback movies satisfy the configured language and rating rules")
	}
	return candidates, nil
}
