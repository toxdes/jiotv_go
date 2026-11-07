package external

import (
	"fmt"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (p *Plugin) vodLiveHandler(c *fiber.Ctx, id string) error {
	stream, err := p.vodResolve(id, vodAfter(c.Query("after")))
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	manifest, err := p.vodResourceURL(stream.ID, stream.ManifestURL)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Set("Cache-Control", "no-store")
	return c.Redirect(manifest, fiber.StatusTemporaryRedirect)
}

func (p *Plugin) vodPlayerHandler(c *fiber.Ctx, id string) error {
	stream, err := p.vodResolve(id, vodAfter(c.Query("after")))
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	manifest, err := p.vodResourceURL(stream.ID, stream.ManifestURL)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	certificate := ""
	license := ""
	if stream.IsDRM {
		license = "/" + p.Name + "/vod/license/" + stream.ID
		if p.cfg.VOD.CertificateRequest != "" {
			certificate = "/" + p.Name + "/vod/certificate/" + stream.ID
		}
	}
	next := fmt.Sprintf("/%s/player/%s?after=%d&af=1", p.Name, url.PathEscape(id), stream.Programme.EndEpoch)
	c.Set("Cache-Control", "no-store")
	values := fiber.Map{"experimental_vod_schedule": p.HasVODSchedule(id), "experimental_vod_start": stream.Programme.StartEpoch, "experimental_vod_end": stream.Programme.EndEpoch, "experimental_vod_duration": stream.Content.Duration, "experimental_vod_server_time": time.Now().UnixMilli(), "experimental_vod_clock_url": "/" + p.Name + "/vod/time", "experimental_vod_padding": stream.Padding}
	if p.HasVODSchedule(id) {
		next = "/" + p.Name + "/player/" + url.PathEscape(id) + "?af=1"
		if p.cfg.VOD.Schedules[id].Logo {
			for _, ch := range p.cfg.Channels {
				if ch.ID == id {
					values["experimental_vod_logo"] = ch.Logo
					values["experimental_vod_channel_name"] = ch.Name
					break
				}
			}
		}
	}
	for key, value := range (fiber.Map{"play_url": manifest, "license_url": license, "experimental_vod_certificate_url": certificate, "player_mode": "vod", "experimental_vod": true, "experimental_vod_next_url": next}) {
		values[key] = value
	}
	return c.Render("views/player_drm", values)
}

func (p *Plugin) vodSelectionHandler(c *fiber.Ctx) error {
	id := c.Params("id")
	if !p.vodEnabled(id) {
		return fiber.ErrNotFound
	}
	stream, err := p.vodResolve(id, vodAfter(c.Query("after")))
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"experimental": true, "fallback": stream.Fallback, "age_rating": stream.Content.AgeRating, "channel_id": id, "programme": stream.Programme, "content_id": stream.Content.ID, "content_title": stream.Content.Title, "asset_type": stream.Content.AssetType, "release_date": stream.Content.ReleaseDate, "drm": stream.IsDRM, "duration": stream.Content.Duration, "scheduled": p.HasVODSchedule(id), "server_time": time.Now().UnixMilli()})
}
