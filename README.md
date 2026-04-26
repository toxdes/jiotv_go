# JioTV Go 📺

JioTV Go, an exciting project that allows you to stream Live TV channels on the web and IPTV. It's a web wrapper around the JioTV Android app, utilizing the same API to fetch and stream channels.

Ready to dive in? Download the latest binary for your operating system from [here](https://github.com/jiotv-go/jiotv_go/releases/latest), and explore the [documentation](https://jiotv_go.rabil.me/) to start your JioTV Go adventure! 🚀

_Give us 🌟 on GitHub if you like this project!_

We have video tutorials for [Windows](https://youtu.be/BnNTYTSvVBc), and [Android](https://youtu.be/ejiuml11g8o) users. Please watch them if you are unsure about the installation process.

## Fork Updates

This fork maintains the upstream codebase with the following additions:

### Channel Favourites (Config-based)

Configure a list of favourite channels in your config file using the `favorite_channel_ids` field (or the `JIOTV_FAVORITE_CHANNEL_IDS` environment variable):

```yaml
favorite_channel_ids:
  - "ChannelID1"
  - "ChannelID2"
```

These are used server-side to generate filtered M3U playlists (see `fav=true` parameter below).

### Client-side Favourites in Web UI

The web interface includes a per-channel star button that marks channels as favourites. Favourites are stored in your browser's `localStorage` and are preserved across sessions. The order in which you add favourites is preserved (newest last). **Note:** This client-side feature is independent of the config-based favourites above and does not sync with the server.

### Channel ID Display in Web UI

Each channel card in the web interface now shows its channel ID (`ID:...`) for easy reference — useful for configuring favourites or debugging.

### `fav=true` Query Parameter

The `/channels` endpoint accepts a `fav=true` query parameter. When combined with `type=m3u`, it returns an M3U playlist containing only the channels listed in `favorite_channel_ids` (config-based), preserving the configured order:

```
/channels?type=m3u&fav=true
```

### `/fav` Alias

A dedicated `/fav` endpoint is available as a shorthand for `/channels?type=m3u&fav=true`. You can also pass additional query parameters (e.g. language filters) which are forwarded:

```
/fav
/fav?language=english
```

---

## Features 🌟

- 📺 Stream Live TV channels, just like in the JioTV Android app.
- 🎬 M3U playlist support for IPTV.
- 🌐 Web interface for watching Live TV.
- 📅 EPG (Electronic Program Guide) support in compressed GZipped XML or JSON format.
- 🎥 Quality selection (Low, Medium, High) supported.
- ⚙️ Configurable port and host.
- 🔐 Authentication using Jio number with OTP.
- 👥 Support for multiple clients simultaneously.
- 🚀 Written in Go, ensuring it's fast, lightweight, and portable.
- 💻 Command-line interface for server management and self-update.
- 🔄 Background start and stop feature.

Get Started with JioTV Go by following the [Get Started](https://jiotv_go.rabil.me/get_started) guide.

## Table of Contents

<details close>
  <summary>Click to expand/collapse</summary>
  
- [JioTV Go 📺](#jiotv-go-)
  - [Fork Updates](#fork-updates)
    - [Channel Favourites (Config-based)](#channel-favourites-config-based)
    - [Client-side Favourites in Web UI](#client-side-favourites-in-web-ui)
    - [Channel ID Display in Web UI](#channel-id-display-in-web-ui)
    - [`fav=true` Query Parameter](#favtrue-query-parameter)
    - [`/fav` Alias](#fav-alias)
  - [Features 🌟](#features-)
  - [Table of Contents](#table-of-contents)
  - [Documentation](#documentation)
  - [Join the community on Telegram:](#join-the-community-on-telegram)
  - [Star History](#star-history)
  - [Contributors](#contributors)
  - [Let's Make JioTV Go Better Together! 🤝](#lets-make-jiotv-go-better-together-)
    - [**Report Bugs**](#report-bugs)
    - [**Ready to Contribute? Join the Journey! 🚀**](#ready-to-contribute-join-the-journey-)
  - [**License: Attribution 4.0 International (CC BY 4.0)**](#license-attribution-40-international-cc-by-40)
</details>

## Documentation

The complete documentation for JioTV Go is available at https://jiotv_go.rabil.me/ 📖

## Join the community on Telegram:

- [Announcement Channel (`jiotv_go`)](https://telegram.me/jiotv_go)
- [Support Group (`jiotv_go_chat`)](https://telegram.me/jiotv_go_chat)

## Star History

<a href="https://star-history.com/#jiotv-go/jiotv_go&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=jiotv-go/jiotv_go&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=jiotv-go/jiotv_go&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=jiotv-go/jiotv_go&type=Date" />
  </picture>
</a>

## Contributors

[![Contributors](https://contributors-img.web.app/image?repo=jiotv-go/jiotv_go)](https://github.com/jiotv-go/jiotv_go/graphs/contributors)

## Let's Make JioTV Go Better Together! 🤝

### **Report Bugs**

Found a pesky bug? No worries! Please help us improve JioTV Go by creating an issue [here](https://github.com/jiotv-go/jiotv_go/issues/new/choose). Be sure to include detailed steps to reproduce the bug, describe the expected behavior, and, if possible, attach screenshots. Your feedback is invaluable!

### **Ready to Contribute? Join the Journey! 🚀**

We wholeheartedly welcome your contributions. If you have ideas, fixes, or enhancements in mind, don't hesitate to create a pull request with your changes. For significant alterations, start by creating an issue to discuss your plans with us. Together, we can make JioTV Go even more incredible.

## **License: Attribution 4.0 International (CC BY 4.0)**

**Embrace the Spirit of Free Software!** JioTV Go is open-source and free to use. We're committed to keeping it accessible to everyone. If you come across any unauthorized attempts to sell this project, please report them to [me](mailto:mail@rabil.me) so we can take swift action. Your support is essential in safeguarding our project's values. 🙌📜💼
