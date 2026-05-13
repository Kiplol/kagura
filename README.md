# Kagura 神楽

A terminal music player for [Navidrome](https://www.navidrome.org/) — browse your library, manage a play queue, and watch a bongo cat dance to every song, all without leaving the terminal.

```
┌─ 1:Artists  2:Albums  3:Songs  4:Playlists  5:Search ─────────────┐
│ ▸ Jóhann Jóhannsson                        │  ▶ Now Playing       │
│   Jon Hopkins                              │  1. The Sun's Gone   │
│   Joep Beving                              │  2. Circles          │
│   Nils Frahm                               │     Dim              │
│   Ólafur Arnalds                           │  3. Midnight         │
│                                            │                      │
│                                            │  ♪ And I knew then   │
│                                            │  ♪ the light would   │
│                                            │  ♪ never fade away   │
│                                            │  ──────────────────  │
│                                            │    ∿∿  ∿∿            │
│                                            │   \(^‿^)/  ♪♫        │
├────────────────────────────────────────────┴──────────────────────┤
│ ▶  The Sun's Gone Dim   ████████████░░░░░░░   2:14 / 5:38  80%   │
└── space:play  j/k:move  enter:play  a:queue  r:dj  v:vis  ?:help ─┘
```

## What it does

Kagura connects to your Navidrome server and gives you a full music browser in the terminal. You can explore your library by artist, album, or playlist; build up a queue on the fly; and let Auto DJ keep the music going when you run out of songs.

A dancing DJ grooves in the corner at the song's actual BPM. Synced lyrics scroll in real time when they're available.

Everything works with the keyboard. If you're on macOS, your media keys (including AirPods gestures and the lock screen) control playback too.

## Requirements

- A running [Navidrome](https://www.navidrome.org/) server
- [mpv](https://mpv.io/) for audio playback (`brew install mpv`)
- macOS or Linux

**Optional** — for BPM detection when your files don't have BPM tags:
```
brew install aubio ffmpeg
```

## Installation

### Homebrew (easiest)

Coming soon.

### Build from source

You'll need [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/Kiplol/kagura.git
cd kagura
go build ./cmd/kagura
./kagura
```

To install it system-wide so you can just type `kagura`:

```bash
go build -o /usr/local/bin/kagura ./cmd/kagura
```

## Getting started

Run `kagura` and you'll see a login screen. Enter your Navidrome server URL, username, and password — these are saved, so you only do this once.

After that, you land straight in your music library. Your last session is restored: same tab, same position in the list, same queue.

## Keyboard shortcuts

| Key | What it does |
|---|---|
| `j` / `k` or `↑` / `↓` | Move through the list |
| `Enter` | Open artist/album, or play a song |
| `a` | Add to end of queue |
| `n` | Play next (insert after current) |
| `Space` | Play / pause |
| `.` or `>` | Next track |
| `,` or `<` | Previous track |
| `+` / `-` | Volume up / down |
| `r` | Toggle Auto DJ |
| `v` | Switch visualizer (dancing DJ ↔ bars) |
| `←` / `→` | Page through long lists |
| `1`–`5` | Jump to tab (Artists / Albums / Songs / Playlists / Search) |
| `/` | Search |
| `Backspace` | Go back |
| `?` | Show / hide key hints |
| `q` | Quit |

## Features

**Browse your library** across Artists, Albums, Songs, and Playlists. Long lists are paginated and your position is remembered.

**Queue management** — add songs one at a time or drop a whole album into the queue at once. Use `n` to cut in line.

**Auto DJ** — press `r` and Kagura will keep the music going automatically, using Navidrome's "similar songs" feature if you have Last.fm integration set up, or random songs otherwise. Stays on until you turn it off, and the setting is remembered between sessions.

**Synced lyrics** — when available, lyrics scroll in time with the music. Kagura tries your Navidrome server first, then falls back to [lrclib.net](https://lrclib.net/) (a free community lyrics database).

**BPM-driven dancing DJ** — the animation runs at the actual tempo of the song, read from the file's metadata. If your files don't have BPM tags, Kagura can detect it automatically using aubio (optional install).

**Media keys on macOS** — play/pause, skip forward, and skip back all work from the keyboard media keys, AirPods double/triple tap, Control Center, and the lock screen.

**Queue sync** — your queue is saved to Navidrome's play queue, so it's visible from other Subsonic clients like Feishin, and survives app restarts.

## Troubleshooting

**BPM always shows `---`** — your audio files probably don't have BPM tags embedded. Install `aubio` and `ffmpeg` (`brew install aubio ffmpeg`) to enable automatic BPM detection from the audio stream. Detection runs in the background and the DJ will start dancing at the right tempo after about 30 seconds.

**App looks garbled** — make sure your terminal uses a font that supports Unicode. Any modern terminal (iTerm2, Ghostty, Kitty, Alacritty, macOS Terminal) works fine.

**Media keys not working** — this uses macOS system APIs. Make sure Kagura is the frontmost app and hasn't been blocked in System Settings → Privacy.

**Debug log** — if something isn't working, check `/tmp/kagura.log` for detailed output.

## License

MIT
