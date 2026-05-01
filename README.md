# Serato Companion

A floating always-on-top companion app for Serato DJ that watches what you're currently playing and instantly surfaces harmonically compatible track recommendations from your library.

## Features

- **Floating pill UI** — collapses to a slim bar, expands on hover, stays above Serato at all times
- **Harmonic key matching** via the Camelot wheel (same key, relative, adjacent ±1, energy boost +7, dominant +3)
- **BPM filtering** — absolute min/max range
- **Energy level filter** — Mixed In Key energy within ±1 step
- **Genre match** filter
- **Date Added / Song Year** range filter with per-track source toggle
- **Hide played** — automatically excludes tracks already played this session
- **Resizable window** — remembers your preferred size across sessions
- **Collapsible filters panel** — hide filters to maximise visible recommendations
- **Copy to clipboard** — click any recommendation to copy artist – title
- **Drag to deck** — drag recommendation cards directly onto Serato decks (Windows)
- **DPI / zoom slider** — scale the UI 50–150%

---

## Installation

### Windows

> **Requires:** [Node.js 18+](https://nodejs.org)

1. Download `serato-companion-windows.zip` from the [latest release](https://github.com/dvize/serato-companion/releases/latest)
2. Unzip to any folder (e.g. `C:\Tools\serato-companion`)
3. Double-click `start.bat`

On first launch, `start.bat` installs Electron automatically (~120 MB, one-time). Subsequent launches are instant.

---

### macOS

> **Requires:** [Node.js 18+](https://nodejs.org)

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
5. **Drag** a recommendation card onto a Serato deck to load it (Windows only)
6. Use **🔒 Lock** to freeze the current track (ignores new plays); click **↺ Sync** to re-sync

---

## Filters Reference

| Filter | Description |
|---|---|
| **Harmonic Key** | Camelot wheel compatibility — same, relative (A↔B), adjacent (±1), energy boost (+7), dominant (+3) |
| **Same Genre** | Only show tracks whose genre matches the current track |
| **Energy ±1** | Mixed In Key energy level within one step of the current track |
| **Hide Played** | Exclude tracks already played this session |
| **BPM Min / Max** | Hard lower and upper BPM limits |
| **Max Results** | Cap the list at 10 / 25 / 50 / 100 / All |
| **From Year / To Year** | Year range filter; clear both for no limit |
| **Date Added / Song Year** | Whether the year filter applies to when you added the track to Serato, or the track's release year ID3 tag |

---

## Building from Source

### Prerequisites

- [Go 1.21+](https://golang.org/dl/)
- [Node.js 18+](https://nodejs.org)

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

`config.json` (all fields optional — leave empty strings to auto-detect):

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

---

## How It Works

1. **Library parser** reads Serato's `database V2` flat-file format (chunk-tagged UTF-16BE records) to load your full track library including BPM, Camelot key, genre, energy (Mixed In Key), and date added.
2. **Session watcher** polls `master.sqlite` every 2 seconds for new history entries to detect the currently playing track in real time.
3. **Recommendation engine** scores every library track against the current track using Camelot wheel rules, BPM proximity, genre, and energy, then returns the top matches.
4. **Electron UI** displays everything in a transparent frameless always-on-top window that collapses to a pill when not in use.
