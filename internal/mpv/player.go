// Package mpv wraps mpv's JSON IPC interface, launching mpv as a headless
// subprocess and communicating with it over a Unix socket.
package mpv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const socketPath = "/tmp/kagura-mpv.sock"

// State holds a snapshot of current playback state.
type State struct {
	Playing     bool
	Position    float64 // seconds elapsed
	Duration    float64 // total seconds
	Title       string
	Volume      int
	PlaylistPos int // current index in mpv's playlist (-1 = none)
	BPM         int // from file metadata tags (0 if not present)
}

// Player controls mpv over IPC.
type Player struct {
	cmd     *exec.Cmd
	conn    net.Conn
	mu      sync.Mutex
	reqID   atomic.Int64
	state   State
	stateMu sync.RWMutex

	// pending maps request_id → property name for in-flight get_property calls.
	pendingMu sync.Mutex
	pending   map[int64]string
}

// New launches mpv in headless/IPC mode and connects to its socket.
// Callers must call Close() when done.
func New() (*Player, error) {
	// Remove stale socket if present.
	_ = os.Remove(socketPath)

	cmd := exec.Command("mpv",
		"--no-video",
		"--no-terminal", // prevent mpv from reading/writing the controlling TTY
		"--idle=yes",
		"--really-quiet",
		fmt.Sprintf("--input-ipc-server=%s", socketPath),
	)
	// Fully detach mpv from the terminal:
	//   • Redirect stdin/stdout/stderr to /dev/null
	//   • Setsid=true creates a new session, removing the controlling TTY entirely
	// Without this mpv writes escape sequences to the TTY even with --really-quiet,
	// which scrolls the display and corrupts tview's rendering.
	devNull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching mpv: %w", err)
	}

	// Give mpv a moment to create the socket.
	var conn net.Conn
	var err error
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}
	}
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("connecting to mpv socket: %w", err)
	}

	p := &Player{
		cmd:     cmd,
		conn:    conn,
		pending: make(map[int64]string),
	}
	p.state.PlaylistPos = -1
	go p.readEvents()
	go p.pollState()
	return p, nil
}

// --- Playback commands ---

// Load appends a URL to the playlist and starts playing if queue was empty.
func (p *Player) Load(streamURL string) error {
	return p.command("loadfile", streamURL, "append-play")
}

// LoadAndPlay clears the current queue, loads a single URL, and plays it.
func (p *Player) LoadAndPlay(streamURL string) error {
	return p.command("loadfile", streamURL, "replace")
}

// LoadPlaylistAt loads all URLs into a fresh playlist and begins playing from index.
// All songs are present in mpv's playlist so Prev/Next work across the whole list.
// mpv processes commands serially through the socket, so set_property playlist-pos
// executes only after all loadfile commands have been registered.
func (p *Player) LoadPlaylistAt(urls []string, index int) error {
	if len(urls) == 0 {
		return nil
	}
	// Replace with first URL — mpv starts loading track 0 immediately.
	if err := p.command("loadfile", urls[0], "replace"); err != nil {
		return err
	}
	// Append remaining tracks.
	for _, u := range urls[1:] {
		if err := p.command("loadfile", u, "append"); err != nil {
			return err
		}
	}
	// Jump to target position (playlist is fully registered by the time mpv
	// processes this command, because the socket is a serial command queue).
	if index > 0 {
		return p.setProperty("playlist-pos", index)
	}
	return nil
}

// PlayPause toggles play/pause.
func (p *Player) PlayPause() error {
	return p.command("cycle", "pause")
}

// Pause pauses playback.
func (p *Player) Pause() error {
	return p.setProperty("pause", true)
}

// Play resumes playback.
func (p *Player) Play() error {
	return p.setProperty("pause", false)
}

// Next skips to the next track in the playlist.
func (p *Player) Next() error {
	p.stateMu.RLock()
	pos := p.state.PlaylistPos
	p.stateMu.RUnlock()
	return p.setProperty("playlist-pos", pos+1)
}

// Prev skips to the previous track.
func (p *Player) Prev() error {
	p.stateMu.RLock()
	pos := p.state.PlaylistPos
	p.stateMu.RUnlock()
	if pos <= 0 {
		return nil
	}
	return p.setProperty("playlist-pos", pos-1)
}

// Seek seeks to an absolute position in seconds.
func (p *Player) Seek(seconds float64) error {
	return p.command("seek", seconds, "absolute")
}

// SeekRelative seeks forward or backward by the given number of seconds.
func (p *Player) SeekRelative(seconds float64) error {
	return p.command("seek", seconds, "relative")
}

// SetVolume sets volume 0–100.
func (p *Player) SetVolume(vol int) error {
	return p.setProperty("volume", vol)
}

// ClearPlaylist removes all items from the internal playlist.
func (p *Player) ClearPlaylist() error {
	return p.command("playlist-clear")
}

// State returns a snapshot of current playback state (thread-safe).
func (p *Player) State() State {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.state
}

// Close stops mpv and cleans up the socket.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	_ = os.Remove(socketPath)
}

// --- IPC internals ---

type ipcRequest struct {
	Command   []any `json:"command"`
	RequestID int64 `json:"request_id"`
}

func (p *Player) send(req ipcRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err = p.conn.Write(data)
	return err
}

func (p *Player) command(args ...any) error {
	return p.send(ipcRequest{
		Command:   args,
		RequestID: p.reqID.Add(1),
	})
}

func (p *Player) setProperty(name string, val any) error {
	return p.send(ipcRequest{
		Command:   []any{"set_property", name, val},
		RequestID: p.reqID.Add(1),
	})
}

func (p *Player) getProperty(name string) error {
	id := p.reqID.Add(1)
	p.pendingMu.Lock()
	p.pending[id] = name
	p.pendingMu.Unlock()
	return p.send(ipcRequest{
		Command:   []any{"get_property", name},
		RequestID: id,
	})
}

// readEvents parses mpv's mixed event/response stream and updates state.
// mpv sends both asynchronous events {"event": "..."} and responses to our
// get_property requests {"request_id": N, "error": "success", "data": <value>}
// over the same socket.
func (p *Player) readEvents() {
	scanner := bufio.NewScanner(p.conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Events have an "event" key.
		if event, ok := msg["event"].(string); ok {
			// When a new track starts, request updated playlist position and metadata.
			if event == "start-file" {
				_ = p.getProperty("playlist-pos")
				_ = p.getProperty("metadata")
			}
			continue
		}

		// Responses to get_property calls have "request_id" and "data".
		reqIDf, _ := msg["request_id"].(float64)
		reqID := int64(reqIDf)

		p.pendingMu.Lock()
		propName := p.pending[reqID]
		delete(p.pending, reqID)
		p.pendingMu.Unlock()

		if propName == "" {
			continue
		}
		// Only update on success (mpv returns "success" or an error string).
		if errStr, _ := msg["error"].(string); errStr != "success" {
			continue
		}

		data := msg["data"]
		p.stateMu.Lock()
		switch propName {
		case "time-pos":
			if v, ok := data.(float64); ok {
				p.state.Position = v
			}
		case "duration":
			if v, ok := data.(float64); ok {
				p.state.Duration = v
			}
		case "pause":
			if v, ok := data.(bool); ok {
				p.state.Playing = !v
			}
		case "volume":
			if v, ok := data.(float64); ok {
				p.state.Volume = int(v)
			}
		case "media-title":
			if v, ok := data.(string); ok {
				p.state.Title = v
			}
		case "playlist-pos":
			if v, ok := data.(float64); ok {
				p.state.PlaylistPos = int(v)
			}
		case "metadata":
			// Extract BPM from file tags. Common keys: BPM, TBPM (ID3), bpm.
			p.state.BPM = 0
			if meta, ok := data.(map[string]any); ok {
				for _, key := range []string{"BPM", "TBPM", "bpm", "tbpm"} {
					if val, ok := meta[key].(string); ok && val != "" {
						val = strings.TrimSpace(val)
						// Handle integer or float BPM strings (e.g. "128" or "128.0")
						if dot := strings.IndexByte(val, '.'); dot >= 0 {
							val = val[:dot]
						}
						if bpm, err := strconv.Atoi(val); err == nil && bpm > 0 {
							p.state.BPM = bpm
							break
						}
					}
				}
			}
		}
		p.stateMu.Unlock()
	}
}

// pollState periodically queries mpv for position, duration, pause state,
// volume, title, and playlist position so State() always returns fresh data.
func (p *Player) pollState() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		_ = p.getProperty("time-pos")
		_ = p.getProperty("duration")
		_ = p.getProperty("pause")
		_ = p.getProperty("volume")
		_ = p.getProperty("media-title")
		_ = p.getProperty("playlist-pos")
	}
}
