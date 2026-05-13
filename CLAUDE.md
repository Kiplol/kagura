# Kagura — Project Guide

A terminal-based music client for Navidrome servers. Renders a full TUI in any terminal (macOS + Linux), streams audio via mpv, and supports Apple media keys.

**The active implementation is Go + tview.** (A previous bubbletea version was abandoned due to persistent ghost-content/overdraw bugs in its differential renderer. Remnants live in `internal/tui/views/` and `internal/tui/components/` but carry `//go:build ignore` tags and are not compiled. A Python/Textual spike also exists but is similarly dead code.)

---

## Tech Stack

| Layer | Choice | Notes |
|---|---|---|
| Language | Go 1.21+ | Single binary, ships cross-platform |
| TUI framework | tview + tcell | Widget-based, absolute cell addressing — no differential renderer |
| Audio playback | mpv over IPC | Unix socket at `/tmp/kagura-mpv.sock`; mpv launched as subprocess |
| Server protocol | Subsonic REST API | What Navidrome speaks; token auth (MD5 + salt) |
| Media keys (macOS) | CGo → MPRemoteCommandCenter | `internal/hotkey/media_keys_darwin.go` |
| Media keys (Linux) | Not yet implemented | Stub in `media_keys_linux.go` |
| Config | JSON at `~/.config/kagura/config.json` | Saved on first login |

---

## Project Layout

```
Navidrome ASCII Client/
├── cmd/kagura/
│   └── main.go                        # Entry point — calls tui.Run(cfg)
├── internal/
│   ├── config/config.go               # Load/save JSON config
│   ├── subsonic/
│   │   ├── client.go                  # Subsonic REST API client (token auth)
│   │   └── lrclib.go                  # lrclib.net lyrics fallback (no auth required)
│   ├── mpv/player.go                  # mpv IPC wrapper (Unix socket)
│   ├── bpm/detect.go                  # BPM detection via aubiotrack + ffmpeg
│   ├── hotkey/
│   │   ├── daemon.go                  # Hotkey daemon — routes commands to app
│   │   ├── media_keys_darwin.go       # macOS MPRemoteCommandCenter (CGo)
│   │   └── media_keys_linux.go        # Linux stub (MPRIS planned)
│   └── tui/
│       ├── app.go                     # ALL active TUI code — screens, widgets, key handling
│       ├── views/                     # OLD bubbletea views — all tagged //go:build ignore
│       └── components/                # OLD bubbletea components — all tagged //go:build ignore
└── go.mod / go.sum
```

`internal/tui/app.go` is the single source of truth for the TUI. It contains:
- `App` struct with all widget references and playback/navigation state
- `Run(cfg)` entry point
- Login screen (`buildLoginPage`) — ASCII bonsai sakura tree art
- Main screen (`buildMainPage`) — browser on the left, queue+visualizer panel on the right
- Tab fetching goroutines (`fetchTab`, `fetchSearch`, `fetchTabAndRestore`)
- Selection/navigation (`handleSelect`, `pushNav`, `popNav`)
- Queue management (`enqueue`, `enqueueSelected`, `replaceQueueFrom`, `clearQueue`)
- Play queue persistence (`savePlayQueue`, `loadPlayQueue`)
- Key handling (`handleKey`)
- Hotkey bridge (`handleHotkeyCmd`)
- Now-playing ticker (`ticker`)
- Beat ticker (`startBeatTicker`) — dedicated goroutine driving dancing DJ animation
- UI state persistence (`saveUIState`) — saves tab/page/row to config
- UI helpers (`updateTabBar`, `updateNowBar`, `updateQueuePanel`, `updateLyricsPanel`, `updateVisualizerPanel`, `setItems`, …)

---

## Building & Running

```bash
cd "Navidrome ASCII Client"

# First time — install mpv (required for audio)
brew install mpv

# Optional — install for BPM detection from audio stream
brew install aubio ffmpeg

# Build
go build ./cmd/kagura

# Run
./kagura
```

On first launch the login screen appears. Credentials are saved to
`~/.config/kagura/config.json`; subsequent launches go straight to the browser
with the previously saved queue restored (paused at the last position) and the
last-used tab/page/scroll row restored.

macOS media keys work automatically (CGo + system frameworks). If the Objective-C layer fails to
compile, the app still runs; hotkeys are silently disabled.

---

## In-App Keybindings

| Key | Action |
|---|---|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `Enter` | Select / drill in; on a song: replace queue with context, play from that song; auto-unpauses if paused |
| `a` | Enqueue highlighted item (song, album, or playlist) — append |
| `n` | Enqueue highlighted item — insert next |
| `c` | Clear queue and stop playback |
| `Space` | Play / pause |
| `.` or `>` | Next track |
| `,` or `<` | Previous track |
| `[` | Seek back 10 seconds |
| `]` | Seek forward 10 seconds |
| `+` / `=` | Volume up (+5%) |
| `-` | Volume down (-5%) |
| `r` | Toggle Auto DJ (fills queue with similar/random songs when running low) |
| `v` | Cycle visualizer (dancing DJ → beat-pulse bars) |
| `←` / `→` | Page through browser list |
| `1`–`6` | Switch browser tab (Album Artists/Albums/Songs/Playlists/Favorites/Search) |
| `/` | Jump to Search tab |
| `Backspace` | Go back (drill-down) |
| `?` | Toggle key hints bar |
| `q` / Ctrl-C | Quit (saves queue position first) |

### Media keys (macOS)
⏯ Play/Pause, ⏭ Next, ⏮ Previous — AirPods gestures, Control Center, and lock screen all work.

---

## Browser Tabs

| Tab | Contents | Drill-down |
|---|---|---|
| 1: Album Artists | All album artists (A–Z), paginated | → Artist page (albums + top songs + similar artists) → Songs |
| 2: Albums | All albums (A–Z), paginated | → Songs |
| 3: Songs | 50 random songs | Play directly |
| 4: Playlists | All playlists | → Songs |
| 5: Favorites | Starred artists, albums, songs (top 5 each + view all) | → full list |
| 6: Search | Type + Enter | Results: artists, albums, songs |

Lists longer than 50 items are paginated; `←`/`→` move between pages. The breadcrumb
shows `(page X/Y  ←→)` when more than one page exists.

### Artist page
Drilling into an album artist shows a rich page fetched in parallel from three endpoints:
1. **Albums** — first 5 albums with year, plus "View all N albums" if more exist
2. **Top Songs** — up to 10 most-played tracks globally (via `getTopSongs`, Last.fm-backed)
3. **Similar Artists** — up to 10 similar artists (via `getArtistInfo2`, Last.fm-backed); clicking any drills into their page

---

## Layout

```
┌─ tab bar ──────────────────────────────────────────────────────────────────┐
│ 1:Album Artists  2:Albums  3:Songs  4:Playlists  5:Favorites  6:Search     │
├─ breadcrumb ───────────────────────────────────────┬─ queue pane ──────────┤
│ Artist Name                                        │  ▶ Now Playing        │
│                                                    │  1. Song A            │
│  Artist page / album list / song list              │  2. Song B            │
│                                                    │  3. Song C            │
│                                                    │                       │
│                                                    │  [lyrics]             │
│                                                    │  ─────────────        │
│                                                    │  [dancing DJ]         │
├─ separator ────────────────────────────────────────┴───────────────────────┤
│ ▶  Track Title   ████████████████░░░░   1:23 / 4:56   vol 80%             │
├────────────────────────────────────────────────────────────────────────────┤
│  j/k:move  Enter:play  a:add  n:insert  c:clear  Space:⏯  >.:next  <,:prev │
│  +-:vol  r:autodj  v:vis  ←→:page  1-6:tabs  /:search  ⌫:back  q:quit  ?  │
└────────────────────────────────────────────────────────────────────────────┘
```

The right pane (~28 chars wide) shows the queue, synced lyrics (3 lines), and
the visualizer. The progress bar spans the full terminal width dynamically.
The key hints bar is 2 lines and can be toggled with `?`.

---

## Features

### Play Queue Persistence
The queue is synced with Navidrome's `savePlayQueue` / `getPlayQueue` Subsonic endpoints.
- On launch: queue is restored and loaded into mpv, paused at the saved position
- On change: saved immediately (after `replaceQueueFrom`, `enqueue`, `clearQueue`)
- Periodically: saved every ~10 seconds to keep playback position current
- On quit: saved synchronously before mpv is torn down
- Cross-client: the saved queue is visible from any Subsonic client (Feishin, etc.)

### Auto DJ
Press `r` to toggle (state persists across launches). When 2 or fewer songs remain in the
queue, Kagura fetches 20 similar songs via `getSimilarSongs` (requires Last.fm integration
in Navidrome) and falls back to `getRandomSongs`. Already-queued songs are filtered out.
The queue header shows `DJ:similar` or `DJ:random` to indicate the source.

### Favorites Tab
Shows starred artists, albums, and songs from Navidrome. Each section displays up to 5 items
with a "View all N" drill-down row. Uses the `getStarred2` Subsonic endpoint.

### Synced Lyrics
Fetched automatically when a song starts. Three-tier fallback:
1. `getLyricsBySongId` — OpenSubsonic extension, returns LRC timestamps from Navidrome
2. `getLyrics` — plain text fallback via Subsonic API
3. lrclib.net — free community lyrics database, no API key required

Displays 3 lines (previous / current / next) in the right pane, highlighted at the current
position when synced timestamps are available. Shows "lyrics unavailable" when a song is
loaded but no lyrics were found from any source.

### BPM Detection & Dancing DJ
BPM is sourced in two ways:
1. **File metadata tags** — read from `BPM`/`TBPM` tags via mpv on song start (instant)
2. **Stream analysis** — if no tag is found, `aubiotrack` + `ffmpeg` analyze the first
   30 seconds of the audio stream in the background (requires `brew install aubio ffmpeg`)

The dancing DJ animation runs at the detected BPM. Falls back to 120 BPM if neither source
yields a result. Debug output is logged to `/tmp/kagura.log`.

### Visualizer
Press `v` to switch between dancing DJ (ASCII art) and beat-pulse bars. The bars visualizer
peaks on every beat and smoothly decays to 20% before the next beat, using wall-clock time
since the last beat divided by the beat interval for a smooth animation effect.

### Window Title
The terminal window title uses a vaporwave decoration:
- Idle: `▓▓▒▒░░ KAGURA 神楽 ░░▒▒▓▓`
- Playing: `▓▓▒▒░░ Holocene — Bon Iver ░░▒▒▓▓`

### UI State Persistence
The app saves the current tab, page, and scroll row to config whenever they change and
every ~10 seconds. On next launch, the same tab/page/position is restored.

---

## Color Approach: Terminal-Native (No Hardcoded Theme)

The app uses **no hex colors** in the UI chrome. All colors are either `tcell.ColorDefault`
(transparent — inherits the terminal background) or ANSI 0-15 indices that the terminal
theme remaps automatically.

```go
tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault  // transparent
tview.Styles.ContrastBackgroundColor = tcell.ColorBlack     // ANSI 0
tview.Styles.BorderColor             = tcell.ColorGray      // ANSI 8
tview.Styles.PrimaryTextColor        = tcell.ColorDefault
tview.Styles.SecondaryTextColor      = tcell.ColorGray

// Tab bar + now-playing bar surface
widget.SetBackgroundColor(tcell.ColorBlack) // ANSI 0

// Selection highlight
selStyle := tcell.StyleDefault.
    Background(tcell.ColorBlack).
    Foreground(tcell.ColorNavy) // ANSI 4 → blue (remapped by theme)
```

**Exception:** the login screen bonsai art uses hex colors (`#f9a8c9`, `#f472b6`, etc.)
purely for decorative effect. These are intentional and isolated to the `loginArt` constant.

This means the app looks correct under any terminal color theme — Catppuccin Mocha, Dracula,
Tokyo Night, etc. — without any per-theme configuration.

---

## Key Design Decisions

### Why tview (not bubbletea)
bubbletea uses a differential renderer that moves the cursor up by counting `\n` characters in the
previous frame. Any variation in frame height (paginator rows, status bar, wide Unicode characters)
permanently desyncs the renderer, leaving ghost content on screen. Multiple fix attempts all failed.
tview renders to an absolute cell grid — no line counting, no drift.

### tview.Table (not tview.List)
`tview.List` has a secondary-text rendering quirk: `ShowSecondaryText(false)` doesn't reliably
suppress secondary text, which appeared as ghost lines ("1 albums", "2 albums") under artist names.
`tview.Table` has no secondary text concept at all and is used everywhere for browser content.

```go
a.list = tview.NewTable().SetSelectable(true, false)
```

### mpv TTY isolation (critical)
mpv writes ANSI escape sequences to the controlling TTY even with `--really-quiet` and
stdout/stderr redirected to `/dev/null`. These sequences scroll the display and corrupt tview's
rendering. The fix requires three things in combination:

```go
cmd := exec.Command("mpv",
    "--no-video",
    "--no-terminal", // tells mpv not to touch the TTY at all
    "--idle=yes",
    "--really-quiet",
    fmt.Sprintf("--input-ipc-server=%s", socketPath),
)
devNull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
cmd.Stdin  = devNull
cmd.Stdout = devNull
cmd.Stderr = devNull
cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // new session = no controlling TTY
```

`Setsid: true` creates a new process group/session, removing the controlling TTY entirely.

### screen.Clear() in BeforeDrawFunc
Even with ColorDefault backgrounds, leftover terminal content can bleed through transparent cells
between draw passes. Clearing the whole screen before each tview draw pass prevents this:

```go
a.tv.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
    screen.Clear()
    return false
})
```

### SetBackgroundColor chain-breaking (tview quirk)
In the installed tview version, `SetBackgroundColor` (inherited from `*tview.Box`) returns
`*tview.Box`, not the original widget type. This breaks method chains when the result is assigned
to a typed variable. Always split into two statements:

```go
// WRONG — compile error: cannot use *tview.Box as *tview.TextView
a.tabBar = tview.NewTextView().SetDynamicColors(true).SetBackgroundColor(tcell.ColorBlack)

// CORRECT
a.tabBar = tview.NewTextView().SetDynamicColors(true)
a.tabBar.SetBackgroundColor(tcell.ColorBlack)
```

### tview.Pages: visible=true + SwitchToPage
`AddPage` must be called with `visible=true` as the fourth argument, AND `SwitchToPage` must be
called explicitly after building the main page when skipping login:

```go
a.pages.AddPage("main", root, true, true)   // last param = visible
a.pages.SwitchToPage("main")                // also required
```

### Context-replace queue on Enter
When the user presses Enter on a song inside an album/playlist/search results, the entire visible
song list becomes the new queue (clearing whatever was there before), and playback starts from the
selected song. Songs before the selection are still in the queue as "past" tracks (navigable with
Prev). `a` and `n` enqueue only the highlighted item (not the whole list).

### Insert-next rebuilds the mpv playlist
`loadfile … append-play` always appends to the *end* of mpv's internal playlist. When inserting
next (`n` key), the full mpv playlist is rebuilt in the correct order via `LoadPlaylistAt`, then
seeks back to the saved position in the currently playing song. Plain append (`a`) still just
appends since that's always correct.

### Dedicated beat ticker goroutine
The dancing DJ animation runs in its own goroutine (`startBeatTicker`) rather than being driven
by the main 500ms polling ticker. This ensures every animation frame has an equal duration.

The goroutine is restartable: calling `startBeatTicker()` closes the previous goroutine's stop
channel (`beatStop`) before launching a new one. This is used when the BPM changes mid-song
(e.g. when aubio reports a result after the initial metadata read).

Two separate values track BPM state:
- `a.currentBPM` — the authoritative BPM for the current song (set once per song, never
  overwritten by the polling ticker)
- `a.beatInterval` — the derived tick duration, updated whenever `currentBPM` changes

This separation prevents the polling ticker from accidentally resetting the aubio-detected BPM
back to the file-tag value (or 0) on every 500ms tick.

```go
// Fixed-timestep advance — never snap baseline to now
a.lastBeat = a.lastBeat.Add(interval) // NOT: a.lastBeat = now
```

### BPM detection — aubiotrack PATH workaround
The Homebrew-installed `aubiotrack` binary is not in the process PATH (apps launched from a
desktop environment don't inherit the shell's PATH). `internal/bpm/detect.go` searches
explicit Homebrew prefix paths (`/opt/homebrew/bin/`, `/usr/local/bin/`, `/opt/local/bin/`)
in addition to `exec.LookPath`.

Additionally, the Homebrew build of aubiotrack lacks HTTP/libav support, so it cannot
read audio streams directly. The workaround uses ffmpeg to download the first 30 seconds
of the stream into a temp WAV file, which aubiotrack then reads locally.

### Next/Prev track — set_property playlist-pos (not playlist-next/prev)
`playlist-next soft` and `playlist-prev` are mpv commands that don't work reliably with HTTP
streams even when the playlist has multiple entries. Direct `set_property playlist-pos N` is
used instead and works correctly.

---

## What's Next

- [ ] As-you-type search — debounced live results (~250ms), min 2 chars, stale-response guard
- [ ] Album art → block character / half-block rendering
- [ ] MPRIS support on Linux (D-Bus via `github.com/godbus/dbus/v5`)
- [ ] Preferences panel — logout option, hotkey remapper
- [ ] Homebrew formula (or `go install`) for easy installation
- [ ] Auto DJ enqueue 5 songs instead of 20
- [ ] Remove unused code from previous attempts from project
- [ ] Sort albums by most recently released (or whatever makes sense)
- [ ] Sometimes DJ type is truncated (`── QUEUE  DJ:… ──`)
- [ ] Look into some sort of floating HUD if possible (menu bar maybe?)
- [ ] Bug: App always opens (or maybe switches) to Album Artists tab, but correct tab remains highlighted
- [ ] Center the dancing DJ
- [ ] Kagura should reset the window's title when it quits
- [ ] Mention that I don't know any Go and Claude wrote all of this in the readme
- [x] Artist page — albums preview (first 5 + view all), top songs, similar artists
- [x] Favorites tab — starred artists, albums, songs with view-all drill-down
- [x] Vaporwave window title decoration (`▓▓▒▒░░ KAGURA 神楽 ░░▒▒▓▓`)
- [x] "lyrics unavailable" shown when a song has no lyrics from any source
- [x] Beat-pulse decay bars visualizer (peaks on beat, smooth decay)
- [x] `[` / `]` seek back / forward 10 seconds
- [x] Multiline key hints bar (2 rows)
- [x] 6 tabs: Album Artists, Albums, Songs, Playlists, Favorites, Search
- [x] Fix: Next/Prev track keys (`.` `>` `,` `<`) — switched to set_property playlist-pos
- [x] UI state persistence — last tab, page, and scroll row saved across launches
- [x] Auto DJ state persists across launches
- [x] lrclib.net as third lyrics fallback (no API key required)
- [x] BPM detection via aubiotrack + ffmpeg when file has no BPM tag
- [x] Dedicated beat ticker goroutine — all animation frames equal length, no drift
- [x] On-screen key hints bar (shown by default, toggle off/on with `?`)
- [x] BPM-driven dancing DJ animation
- [x] Left/right arrow key pagination through browser lists
- [x] Synced lyrics display in right pane (`getLyricsBySongId` + `getLyrics` fallback)
- [x] Vertical bars visualizer, switchable with `v`
- [x] Enter on song auto-unpauses
- [x] `a`/`n` enqueue only the highlighted item
- [x] Auto DJ mode (`r`) — fills queue from `getSimilarSongs` / `getRandomSongs`
- [x] Play queue persistence via `savePlayQueue` / `getPlayQueue` (syncs with other Subsonic clients)
- [x] Clear queue with `c`
- [x] Dynamic progress bar width (fills terminal width)
- [x] Login screen with ASCII bonsai sakura tree and Japanese subtitle 神楽
- [x] App renamed to Kagura
