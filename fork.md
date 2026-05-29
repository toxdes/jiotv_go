# Fork Changelog

This fork builds on `jiotv-go/jiotv_go` with the following additions.

## External Plugin System

Added a generic plugin framework (`pkg/plugins/external/`) that loads channels and
streaming logic from runtime JSON configs. No provider-specific source code required.

### Architecture

- `pkg/plugins/external/plugin.go` — `Plugin` struct with `New(name, configURL)`,
  `FetchPlatformToken()`, `FetchVideoToken(channelID)`, `generateDDToken()`,
  `generateGuestToken()`
- `pkg/plugins/external/handlers.go` — Route handlers (`LiveHandler`, `PlayHandler`,
  `PlayerHandler`, `RenderHandler`, `ProxySegmentHandler`, `LogoHandler`) that proxy
  HLS playlists and segments through the server with signed URLs
- `internal/plugins/manager.go` — `Init()` iterates `config.Cfg.Plugins` map,
  creates external plugins, registers routes
- `internal/config/config.go` — `Plugins map[string]string` config field

### Config

```toml
# configs/jiotv-config.toml
plugins = { myprovider = "https://example.com/myprovider-config.json" }
```

### Runtime Config JSON Schema

```json
{
  "name": "myprovider",
  "category": 20,
  "channels": [
    {
      "id": "ch-123",
      "name": "Channel Name",
      "url": "https://cdn.example.com/.../index.m3u8",
      "logo": "https://cdn.example.com/.../logo.jpg",
      "language": 1,
      "genre": 5,
      "slug": "channel-slug"
    }
  ],
  "api": {
    "playback_url": "https://api.example.com/singlePlayback/getDetails/secure",
    "auth_url": "https://www.example.com/",
    "platform_token_regex": "\"token\":\"([^\"]+)\""
  },
  "headers": {
    "Origin": "https://www.example.com",
    "Referer": "https://www.example.com/"
  }
}
```

### Route Map (for plugin named `myprovider`)

| Route | Handler | Purpose |
|---|---|---|
| `GET /myprovider/:id` | `LiveHandler` | Fetch video token + serve master playlist |
| `GET /myprovider/play/:id` | `PlayHandler` | Render player page |
| `GET /myprovider/player/:id` | `PlayerHandler` | Render HLS player |
| `GET /myprovider/render/playlist.m3u8` | `RenderHandler` | Proxy child playlist with URL rewriting |
| `GET /myprovider/render/segment.ts` | `ProxySegmentHandler` | Proxy TS segments |
| `GET /myprovider/render/segment.mp4` | `ProxySegmentHandler` | Proxy MP4/fMP4 segments |
| `GET /myprovider/logo` | `LogoHandler` | Proxy channel logos |

### Key Design Decisions

- **No hardcoded provider names** — all service-specific strings (API URLs, regex,
  channel data) come from the runtime JSON config
- **URL encryption** — playlist and segment URLs are encrypted with `secureurl`
  before being written into rewritten playlists, preventing token leakage
- **Redirect-safe HTTP client** — custom `http.Client` with `CheckRedirect` that
  preserves `User-Agent`, `Origin`, `Referer` on CDN redirects
- **Auto-decompression** — Go's default transport handles gzip; no manual
  `Accept-Encoding` that would disable it

## WebOS / Smart TV App

A TV-optimized channel grid at `/tv` with spatial navigation (arrow keys),
channel number typing, channel up/down surfing, and an SPA player overlay.
An `.ipk` package is in `webos/`.

## Config-Based Favourites

`favorite_channel_ids` in `config.toml` controls M3U playlist order and the
TV UI favourites list. `/fav` endpoint returns an M3U with only favourites.
