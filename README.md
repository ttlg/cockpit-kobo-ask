# cockpit-kobo-ask

Answer [AGI Cockpit](https://agi-labo.com/tools/cockpit) Asks from a Kobo e-reader.

A small relay on the Mac forwards your answers to Cockpit through the `cockpit` CLI. On the Kobo, you can use either of two clients:

- **Native app** (`app/`): a standalone Go app launched from NickelMenu as **AGI Cockpit**. It draws directly to the e-ink screen, so it is fast and does not need KOReader's interface.
- **KOReader plugin** (`askterminal.koplugin/`): runs inside KOReader.

Both follow the Cockpit PWA inbox: free-form input on every question, an explicit Send button, a whole-Ask answer for multi-question Asks, previous/next navigation, and Close.

Tested with a Kobo Libra Colour (firmware 4.45), KOReader v2026.07.1, NickelMenu v0.6.0, Node.js 24, and macOS.

## How it works

```
Kobo (KOReader + askterminal.koplugin)
  │  Wi-Fi, HTTP, bearer token
  │  GET /asks every 15 seconds · POST /asks/:id/answer · POST /asks/:id/close
  ▼
Relay on the Mac (relay/server.ts, port 47390)
  │  cockpit ask list · cockpit ask answer · cockpit ask close
  ▼
AGI Cockpit
```

- **NickelMenu** adds a "KOReader" item to the stock Kobo menu.
- **KOReader** is an open-source reader that takes over the screen while it runs. Exiting it returns to the stock Kobo home.
- **askterminal.koplugin** polls the relay, renders the Ask screen, and sends answers.
- **The relay** calls the `cockpit` CLI, so Cockpit records answers from the Kobo as `answered_by: "cli"` and resumes the asking task.

The relay is needed because Tailscale does not run on the Kobo. It listens on your local network only.

## Requirements

- A Kobo e-reader with a touch screen
- A Mac running AGI Cockpit, with the `cockpit` CLI at `~/.agi-tools/bin/cockpit`
- Node.js 23.6 or later, which runs TypeScript files directly
- The Kobo and the Mac on the same Wi-Fi network

## Setup

### 1. Install NickelMenu and KOReader on the Kobo

1. Connect the Kobo over USB and choose **Connect** on the Kobo.
2. Download `KoboRoot.tgz` from [NickelMenu releases](https://github.com/pgaskin/NickelMenu/releases) and copy it to `.kobo/` on the Kobo.
3. Download `koreader-kobo-<version>.zip` from [KOReader releases](https://github.com/koreader/koreader/releases) and extract it into `.adds/` on the Kobo, so that `.adds/koreader/` exists. Firmware 5.x needs the `koreader-kobov5` package instead.
4. Create `.adds/nm/koreader` with this line:

   ```
   menu_item:main:KOReader:cmd_spawn:quiet:exec /mnt/onboard/.adds/koreader/koreader.sh
   ```

5. To keep KOReader's files out of the Kobo library, add this to `.kobo/Kobo/Kobo eReader.conf`:

   ```
   [FeatureSettings]
   ExcludeSyncFolders=(\\.(?!kobo|adobe).+|([^.][^/]*/)+\\..+)
   ```

6. Eject the Kobo and unplug it. It installs NickelMenu and restarts.

See the [KOReader installation guide](https://github.com/koreader/koreader/wiki/Installation-on-Kobo-devices) for details.

### 2. Start the relay

```bash
node relay/server.ts
```

The first run creates `relay/config.json` with the port and a random token. The file is ignored by Git. Keep the token private.

### 3. Run the relay at login (optional)

Render the launchd template and load it:

```bash
plist=~/Library/LaunchAgents/local.cockpit-kobo-ask-relay.plist
node_bin="$(which node)"
sed -e "s|__NODE_DIR__|$(dirname "$node_bin")|g" \
    -e "s|__NODE__|$node_bin|g" \
    -e "s|__REPO__|$PWD|g" \
    -e "s|__HOME__|$HOME|g" \
    launchd/cockpit-kobo-ask-relay.plist.template > "$plist"
launchctl bootstrap "gui/$(id -u)" "$plist"
```

The relay logs every request to `~/Library/Logs/cockpit-kobo-ask-relay.log`. To stop it:

```bash
launchctl bootout "gui/$(id -u)/local.cockpit-kobo-ask-relay"
```

### 4. Install the plugin on the Kobo

Connect the Kobo over USB, choose **Connect**, then run:

```bash
scripts/install-kobo.sh
```

The script copies the plugin, writes `config.lua` with the relay URL and token, and ejects the Kobo. It detects the Mac's IP from `en0`; pass `--url http://<ip>:47390` to override it.

Unplug the Kobo and open KOReader from the NickelMenu item. If KOReader was already running, restart it: tap the top edge of the screen, open the power icon, and choose **Restart KOReader**.

### 5. Adjust KOReader for an always-on terminal

- **Settings → Network**: turn on **Wi-Fi connection**, turn off **Disable Wi-Fi connection when inactive**, and turn on **Restore Wi-Fi connection on resume**.
- **Settings → Device**: turn off autosuspend while the Kobo runs on USB power. Polling stops while the Kobo sleeps.

### Native app (optional, instead of step 4)

The native app needs NickelMenu (step 1) and an FBInk binary. The install script copies the FBInk build bundled with KOReader, so keep KOReader installed or pass `--fbink <path>`.

```bash
scripts/build-app.sh
scripts/install-app.sh --language en
```

`build-app.sh` downloads Noto Sans JP into `app/fonts/` and cross-compiles `app/build/cockpit-kobo` for ARM Linux. `install-app.sh` copies the app to `.adds/cockpit/` on the Kobo, writes its `config.json`, and adds **AGI Cockpit** to NickelMenu. Restart the Kobo if the menu item does not appear.

When you open **AGI Cockpit**, it connects Wi-Fi, stops the stock Kobo software (Nickel), and takes over the screen, the same way KOReader does. Nickel holds the page-turn and power buttons exclusively, so it must stop for the app to read them. Wi-Fi stays connected.

- Swipe or use the page-turn buttons to scroll.
- Use the on-screen English keyboard for free-text answers.
- If you plug in a USB cable, the app offers to return to Kobo home so the Kobo can ask whether to connect to your computer.
- Tap **Return to Kobo home** or press the power button to exit. The app restarts Nickel, which takes a few seconds and opens the Kobo home screen.

While the app runs, the Kobo does not sleep, so keep it on USB power for long sessions. The last run's log is saved to `.adds/cockpit/last-run.log`.

Tested on a Kobo Libra Colour (1264×1680, touch axes swapped and mirrored). Other models may need different `touch` settings in `config.json`:

```json
{ "touch": { "device": "/dev/input/event1", "swapAxes": true, "mirrorX": false, "mirrorY": true } }
```

## Usage (KOReader plugin)

Open the menu by tapping the top edge of the screen, then go to **Tools → Cockpit Ask**:

| Item | Action |
|---|---|
| Receive Asks | Turn polling on or off |
| Open inbox | Open the Ask screen now |
| Return to Kobo home | Exit KOReader and return to the stock Kobo home |
| Change relay URL / Change relay token | Change the relay URL or token on the device |

A new Ask opens the Ask screen automatically. The × in the title bar hides it until another Ask arrives.

The plugin shows English or Japanese, following KOReader's language setting (**Settings → Language**). Japanese is used when KOReader is set to Japanese; English is used otherwise. The labels match the Cockpit PWA in each language.

## Limitations

- No file attachments, and images or videos attached to an Ask are not shown.
- Cockpit display notices are not shown, because the CLI does not expose their content.
- The Kobo has no speaker, so there is no notification sound. A new Ask triggers a full-screen refresh.
- The relay uses plain HTTP protected by a bearer token. Use it only on a trusted local network.

## Troubleshooting

- **No Asks appear**: check the relay log. If it shows no requests from the Kobo, make sure KOReader is running and Wi-Fi is connected, then choose **Open inbox** to see the error message.
- **The Mac's IP changed**: rerun `scripts/install-kobo.sh`, or change the URL from the Cockpit Ask menu.
- **Changes do not show up**: restart KOReader after copying a new plugin. KOReader loads plugins only at startup.
- **The Cockpit Ask menu is missing**: check `.adds/koreader/crash.log` for `Error when loading`. If a plugin file is missing and an `FSCK0000.000` file appears in the plugin folder, the Kobo's file system check discarded it after USB. Rerun `scripts/install-kobo.sh`, which recreates the plugin folder from scratch.

## License

[MIT](LICENSE)
