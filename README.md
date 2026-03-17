# Mixxx Now Playing

`mixxx-nowplaying` is a small HTTP server that reads the currently playing track from the Mixxx database and serves a minimal now playing page.

The UI shows:

- cover art in a square block on the left
- artist and title on the right
- a built-in fallback image when no cover art is available

## Features

- serves a simple now playing web page
- exposes a JSON endpoint for the current track
- reads embedded cover art directly from track metadata
- falls back to a built-in placeholder image when no artwork exists

## Requirements

- a `mixxx-nowplaying` binary for your platform
- access to a local Mixxx database

Default database path:

```text
$HOME/.mixxx.dev/mixxxdb.sqlite
```

Set `MIXXX_DB_PATH` to use a different database file.

## Running

```bash
./mixxx-nowplaying
```

With a custom database path:

```bash
MIXXX_DB_PATH="$HOME/.mixxx/mixxxdb.sqlite" ./mixxx-nowplaying
```

The server listens on:

```text
http://0.0.0.0:9000
```

## Endpoints

- `/` serves the now playing page
- `/api/now` returns track data as JSON
- `/api/cover?track=<id>` returns cover art for the currently known track

Example response from `/api/now`:

```json
{
  "trackId": 698,
  "artist": "Markus Homm",
  "title": "Don't Know Why (Original Mix)",
  "coverUrl": "/api/cover?track=698",
  "retrievedAt": "2026-03-17T21:12:00Z"
}
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
