// Package hotkey manages both OS media key integration and optional
// user-configured global hotkeys. Media keys are always active; custom
// hotkeys are only registered when the user has set a binding.
package hotkey

import (
	"sync"

	"github.com/kip/kagura/internal/config"
)

// Command is a playback action triggered by a hotkey or media key.
type Command int

const (
	CmdPlay Command = iota
	CmdPause
	CmdTogglePlayPause
	CmdNext
	CmdPrev
	CmdVolumeUp
	CmdVolumeDown
	CmdToggleMute
)

// Handler is called whenever a hotkey or media key fires.
type Handler func(Command)

// Daemon manages media key registration and custom hotkey bindings.
type Daemon struct {
	handler Handler
	cfg     config.Hotkeys
	mu      sync.Mutex
	started bool
}

// New creates a Daemon but does not start it yet.
func New(h Handler) *Daemon {
	return &Daemon{handler: h}
}

// Start activates media key integration and registers any configured custom
// hotkeys. Safe to call multiple times (re-registers on config change).
func (d *Daemon) Start(cfg config.Hotkeys) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg = cfg
	d.started = true

	// Platform-specific media key setup (see media_keys_darwin.go / _linux.go).
	startMediaKeys(d.dispatch)

	// Register custom hotkeys (only those with a non-empty binding).
	d.registerCustom()
}

// UpdateConfig re-registers custom hotkeys after the user changes bindings.
func (d *Daemon) UpdateConfig(cfg config.Hotkeys) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg = cfg
	unregisterCustom()
	d.registerCustom()
}

// Stop deregisters everything.
func (d *Daemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	stopMediaKeys()
	unregisterCustom()
}

// dispatch routes a Command to the handler (thread-safe).
func (d *Daemon) dispatch(cmd Command) {
	if d.handler != nil {
		d.handler(cmd)
	}
}

// registerCustom registers all non-empty custom bindings.
// Must be called with d.mu held.
func (d *Daemon) registerCustom() {
	bindings := map[string]Command{
		d.cfg.PlayPause:  CmdTogglePlayPause,
		d.cfg.Next:       CmdNext,
		d.cfg.Prev:       CmdPrev,
		d.cfg.VolumeUp:   CmdVolumeUp,
		d.cfg.VolumeDown: CmdVolumeDown,
		d.cfg.ToggleMute: CmdToggleMute,
	}
	for combo, cmd := range bindings {
		if combo == "" {
			continue
		}
		registerHotkey(combo, cmd, d.dispatch)
	}
}
