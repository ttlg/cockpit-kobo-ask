# kobo-ask

Answer AGI Cockpit Asks from a Kobo e-reader running KOReader.

- `relay/server.ts`: LAN relay on the Mac. `GET /asks` lists open Asks; `POST /asks/:id/answer` answers through `cockpit ask answer`. Bearer token and port live in `relay/config.json` (generated on first run).
- `askterminal.koplugin/`: KOReader plugin. Copy it to `.adds/koreader/plugins/` and add `config.lua` with `url`, `token`, `interval`, and `enabled`.
- launchd agent: `~/Library/LaunchAgents/com.yoishika.kobo-ask-relay.plist`, log at `~/Library/Logs/kobo-ask-relay.log`.
