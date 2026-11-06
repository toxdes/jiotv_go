# Experimental plugin VOD

This optional feature resolves the programme currently shown in an external
plugin channel's EPG to on-demand content that the signed-in account can play.
It is disabled by default. Numeric channel IDs and all normal channel playback
paths retain their existing behavior.

The experiment is implemented in `pkg/plugins/external/vod*.go`. Startup passes
in a guide resolver that reuses the existing TV2 EPG mapping and XMLTV sources.
Only registered plugin channels matching the configured `vod.channel_prefix`
are eligible. Numeric IDs are explicitly excluded regardless of that prefix.

## Runtime adapter

Add a `vod` object to the plugin's remotely served runtime JSON. It defines:

- `channel_prefix`, `language_codes`, and numeric `asset_types` for `movie`,
  `show`, and `episode`.
- `session_cookies`, `session_defaults`, and `required_session` for importing
  the provider's authenticated browser session.
- `constants` for public API versions, device capabilities, and other values.
- Optional `certificate_request` (base64) and `certificate_response_field` for
  fetching a public DRM certificate and extracting it from a binary envelope.
- `catalogue.request`, `playback.request`, and `license`, each containing a URL,
  HTTP method, headers, query values, and optional JSON body.
- `catalogue.items_path` and `catalogue.fields`, mapping the response to generic
  item fields: `id`, `title`, `original_title`, `asset_type`, `languages`,
  `release_date`, `episode_number`, `parent_id`, and `parent_title`.
- Optional `series.page_size`, `series.seasons`, and `series.episodes` for
  looking up a matched show's seasons and episode pages. Season fields also
  map `order` and `total_items`; requests can use `{{season.id}}`, `{{page}}`,
  and `{{page_size}}`.
- `playback.fields`, locating `trailer`, `drm`, `dash_manifest`, `hls_manifest`,
  and optional `manifest` values in the playback response.

Response paths use dot-separated keys and `*` to traverse arrays, for example
`data.results.*.items.*`. Requests can reference `{{session.token}}`,
`{{query}}`, `{{language}}`, `{{content.id}}`, and configured `{{constants.*}}`
values. An exact placeholder preserves its JSON type. `{{base64:constants.capabilities}}`
encodes the configured JSON value. License templates can also reference
`{{response.*}}` fields from the successful playback response.

A request's optional `error_path` identifies an error in its JSON response.
API URLs must use HTTPS on the signed-in provider's domain. Redirects are not
followed for credential-bearing API requests. An optional `epg_id` on each
runtime channel explicitly selects its guide ID; otherwise the existing TV2
mapping matches the channel name. The adapter contains public protocol details,
not secret session values, and is inactive until the server flag is enabled.

## Setup

Sign in to the provider and obtain an authenticated session JSON using your
own external tooling. Browser export and authentication helpers are personal
tools maintained outside this project. The session JSON supplies the fields
named in the adapter's `required_session` mapping, the matching provider
`origin`, and optional Unix-seconds `expires_at`.

Serve the session JSON from a privately accessible HTTP(S) endpoint. It contains
account credentials: keep it separate from the public channel adapter JSON and
do not publish it in a gist. Use HTTPS outside a trusted private network.

Enable the feature in your server's TOML:

```toml
experimental_plugin_vod = true
plugin_vod_session_url = "https://private.example.com/session.json"
```

The equivalent variables are `JIOTV_EXPERIMENTAL_PLUGIN_VOD` and
`JIOTV_PLUGIN_VOD_SESSION_URL`; JSON and YAML use the same snake-case keys.
The server fetches the session on each selection with a 30-second timeout and
a 1 MiB response limit. Redirects are rejected. Requests do not send provider
Origin, Referer, or session authorization headers to this endpoint. Fetch errors
omit the URL and response body. Session origin, required fields, and expiry
are checked before use. This adapter does not renew account tokens automatically;
replace the served private JSON with a fresh session when credentials expire.
No restart is needed. Disabling the flag restores the plugin's original live path.

## Playback

Open the existing channel in `/tv2`. The existing plugin player URL is reused,
and the existing Shaka/Widevine template handles playback. Provider media and
license proxy routes stay inside that plugin, with their own authorization;
the normal media and license handlers are not changed.

Show titles are normalized and compared using text similarity, constrained by
channel language and movie categories when the guide supplies them. Ambiguous
matches or scores below 0.8 are rejected. A matched movie plays directly. A
matched show uses the configured season and episode catalogue to select its
newest released episode, trying up to five recent episodes if newer ones are
not playable. Adapters without this lookup use episodes in search results. Catalogue
matches are cached for 15 minutes and playback selections for five minutes.

Playback starts at the beginning and has no seek bar, duration control, chapter
navigation, playback-rate selector, or keyboard seeking. When an item ends,
the player selects the next guide slot. Programme lengths can differ from the
original broadcast, so this is an approximate schedule rather than a replica
of the broadcast. Missing guide entries, unavailable titles, ambiguous matches,
and entitlement restrictions produce an explicit error by default. Optional
`vod.fallbacks` rules keyed by exact plugin channel ID replace unmatched titles
only. Each rule supplies `language` and `items` (`id`, `title`, `assetType`).
A show item selects a random season, page, and released episode. Movie pools
are shuffled and rechecked against catalogue IDs, language, and exact
`allowed_ratings`; missing or disallowed ratings are rejected. This requires
the catalogue to map `age_rating`. Existing matches and channels without a
rule retain their behavior; authentication and API errors do not trigger a
fallback. Selections stay stable for the slot cache lifetime.

The first implementation targets the web player;
existing HLS-only IPTV clients may not support VOD responses requiring DASH.

For a sanitized current selection, request
`/<plugin>/vod/selection/<channel-id>`. It reports programme and content
metadata without session tokens, signed media URLs, or license authorization.
