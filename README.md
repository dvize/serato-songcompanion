# Serato Companion

A floating always-on-top companion app for Serato DJ that watches what you're currently playing and instantly surfaces harmonically compatible track recommendations from your library.

## Features

- **Floating pill UI** — collapses to a slim bar, expands on hover, stays above Serato at all times
- **Harmonic key matching** via the Camelot wheel (same key, relative, adjacent ±1, energy boost +7, dominant +3) — each tier can be toggled or reweighted in Settings
- **BPM filtering** — absolute min/max range or percentage tolerance with optional half/double-time matching
- **Energy levels** — match exact, within ±N, higher only, lower only, or ignore
- **Genre matching** — match any or all of the first N genres of the current track
- **Date Added / Song Year** range filter with per-track source toggle
- **Hide played** — automatically excludes tracks already played this session
- **Replay cooldown** — hide tracks played within the last N minutes
- **Session stats** — tracks played tonight and top genres, in the header
- **Settings panel** (⚙) — every matching knob, auto-saved and persisted across launches
- **Smart window sizing** — position and expanded size persist, expand clamps to screen edges, multi-monitor aware
- **Expand hotkey** — global shortcut to toggle expand/collapse
- **Idle dim** — collapsed pill fades when untouched, returns on hover
- **Tray icon (macOS)** — click the menu bar icon to show/hide the companion
- **Collapsible filters panel** — hide filters to maximise visible recommendations
- **Copy to clipboard** — click any recommendation to copy artist – title
- **Drag to deck** — drag recommendation cards directly onto Serato decks
- **Keyboard shortcuts** — `R` refresh, `C` copy top pick, `F` toggle filters
- **DPI / zoom slider** — scale the UI 50–150%
- **Cross-platform** — macOS (DMG), Windows (portable exe), Linux (AppImage)

---

## Installation

### Windows

> **Requires:** [Node.js 20 LTS+](https://nodejs.org) (18 is the minimum; official builds use 22)

1. Download `serato-companion-windows.zip` from the [latest release](https://github.com/dvize/serato-companion/releases/latest)
2. Unzip to any folder (e.g. `C:\Tools\serato-companion`)
3. Double-click `start.bat`

On first launch, `start.bat` installs Electron automatically (~120 MB, one-time). Subsequent launches are instant.

---

### macOS

> **Requires:** [Node.js 20 LTS+](https://nodejs.org) (18 is the minimum; official builds use 22)

1. Download `serato-companion-mac.zip` from the [latest release](https://github.com/dvize/serato-companion/releases/latest)
2. Unzip to any folder
3. Open **Terminal**, `cd` into the unzipped folder, and run:

```bash
bash start.sh
```

On first launch, dependencies are installed automatically (~120 MB, one-time). Subsequent launches are instant.

> **Apple Silicon (M1/M2/M3):** the arm64 binary is included and selected automatically.
> **Intel Mac:** the amd64 binary is used automatically.

---

## Usage

1. Open Serato DJ and start playing
2. The companion pill appears on your screen — hover over it to expand
3. Recommendations update automatically as each new track loads
4. **Click** any recommendation card to copy `Artist – Title` to clipboard
5. **Drag** a recommendation card onto a Serato deck to load it
6. Use **🔒 Lock** to freeze the current track (ignores new plays); click **↺ Sync** to re-sync
7. Click **⚙** for settings — matching defaults, window sizing, hotkey, tray

---

## Filters Reference

| Filter | Description |
|---|---|
| **Harmonic Key** | Camelot wheel compatibility — same, relative (A↔B), adjacent (±1), energy boost (+7), dominant (+3). Tiers toggleable in Settings |
| **Same Genre** | Only show tracks sharing a genre with the current track |
| **Energy** | Any / Exact / Match ±1..4 / Higher / Lower relative to the current track |
| **Hide Played** | Exclude tracks already played this session |
| **BPM Min / Max** | Hard lower and upper BPM limits |
| **Max Results** | Cap the list at 10 / 25 / 50 / 100 / All |
| **From Year / To Year** | Year range filter; clear both for no limit |
| **Date Added / Song Year** | Whether the year filter applies to when you added the track to Serato, or the track's release year ID3 tag |

## Settings Reference

Settings live in the ⚙ panel and auto-save to:

- **macOS:** `~/Library/Application Support/Serato Companion/settings.json`
- **Windows:** `%APPDATA%\Serato Companion\settings.json`
- **Linux:** `~/.config/Serato Companion/settings.json`

Override the location with the `SERATO_SETTINGS_PATH` environment variable.

| Section | Setting | Description |
|---|---|---|
| Matching | Default Energy | Energy mode applied to recommendations (filters panel can override per session) |
| Matching | Genres to Match | Any-of vs all-of, and how many of the current track's first N genres must match |
| Matching | BPM Tolerance % | Percentage window around the current BPM; Half/Double toggle allows half/double-time matches |
| Matching | Key Tiers | Enable/disable each Camelot compatibility tier |
| Matching | Max Results / Hide Played | Defaults for the filters panel |
| Matching | Replay Cooldown | Hide tracks played within the last N minutes |
| Window | Collapsed / Expanded Size | Pill and expanded window dimensions |
| Window | Idle Dim | Fade the collapsed pill after N seconds to N% opacity |
| Window | Expand Hotkey | Click the field and press the combo (must include Cmd/Ctrl/Alt, or be an F-key). Escape clears. Saved and active immediately |
| Window | Tray Icon | Menu bar icon toggles the window (macOS) |
| General | Session Stats | Show tracks-played summary in the header |

---

## Building from Source

### Prerequisites

- [Go 1.21+](https://golang.org/dl/)
- [Node.js 20 LTS+](https://nodejs.org) — official CI builds use Node 22; Electron only needs 18+ locally, but 20+ is recommended

### Windows

```bat
go build -o companion.exe ./cmd/companion/
cd electron
npm install
npm start
```

### macOS

```bash
go build -o companion-mac-arm64 ./cmd/companion/   # Apple Silicon
# or
go build -o companion-mac-amd64 ./cmd/companion/   # Intel
cd electron
npm install
npm start
```

### Cross-compile Mac binaries from Windows

```powershell
$env:GOOS="darwin"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -o companion-mac-arm64 ./cmd/companion/

$env:GOARCH="amd64"
go build -o companion-mac-amd64 ./cmd/companion/
```

---

## Configuration

`config.json` (next to the binary; all fields optional — leave empty strings to auto-detect):

```json
{
  "database_path": "",
  "master_sqlite": ""
}
```

| Field | Default (auto-detected) |
|---|---|
| `database_path` | Windows: `%USERPROFILE%\Music\_Serato_\database V2`  Mac: `~/Music/_Serato_/database V2` |
| `master_sqlite` | Windows: `%LOCALAPPDATA%\Serato\Library\master.sqlite`  Mac: `~/Library/Application Support/Serato/Library/master.sqlite` |

All other preferences live in the per-user `settings.json` (see Settings Reference) — no need to edit `config.json` for matching behavior.

---

## How It Works

1. **Library parser** reads Serato's `database V2` flat-file format (chunk-tagged UTF-16BE records) to load your full track library including BPM, Camelot key, genre, energy (Mixed In Key), and date added.
2. **Session watcher** polls `master.sqlite` every 2 seconds for new history entries to detect the currently playing track in real time.
3. **Recommendation engine** scores every library track against the current track using Camelot wheel rules, BPM proximity, genre, and energy, then returns the top matches.
4. **Electron UI** displays everything in a transparent frameless always-on-top window that collapses to a pill when not in use.
