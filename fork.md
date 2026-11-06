# Fork Features

This fork adds two TV-focused interfaces alongside the upstream application.

## `/tv`

`/tv` is a TV-friendly channel browser with:

- Directional remote navigation
- Channel-number entry and channel up/down surfing
- Favourites mode with configurable channel order
- Configured plugin channels included in the channel list
- A fast in-page player experience

## `/tv2`

`/tv2` is a full-screen viewing experience with:

- A unified list of standard and configured plugin channels
- Remote browsing that previews a channel and its programme information before playback changes
- A three-row programme view with current programme progress and upcoming listings
- Programme-guide navigation and a clear message when programme information is unavailable
- Favourites mode, channel-number entry, and a full channel guide
- Audio-language selection using the Blue remote button or `L` key, with Hindi preferred when available
- Playback fixes for channels that previously loaded without video
- A small live-broadcast delay to reduce buffering
- Reuse of fresh stream information during startup, with automatic refresh when needed
- If a high-quality stream is missing, `/tv2` retries it before trying auto quality

## Experimental plugin VOD

Optionally replaces configured plugin channels with on-demand content matched
from their programme guide. Shows use the latest available episode; movies play
directly. Per-channel fallback rules can choose random show episodes or movies
filtered by language and rating. Playback uses `/tv2` without seek controls.
The feature is disabled by default; numeric channels retain normal playback.

To set up:

1. Serve a plugin JSON containing the channel list and a `vod` adapter, then
   register its URL in `plugins` as usual. Configure any fallback rules there.
2. Serve a valid account session JSON from a privately accessible HTTP(S) URL,
   separate from the public plugin JSON. Use HTTPS outside a trusted network.
3. Enable it in the server config:

   ```toml
   experimental_plugin_vod = true
   plugin_vod_session_url = "https://private.example.com/session.json"
   ```

4. Restart the server and open the configured channel in `/tv2`. When account
   credentials expire, replace the served session JSON; no restart is needed.

See [adapter and session details](docs/experimental-plugin-vod.md). Disabling
`experimental_plugin_vod` restores the plugin's original playback.
