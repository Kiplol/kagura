//go:build linux

package hotkey

// Linux media key integration via MPRIS (D-Bus) is planned.
// For now these are no-ops so the package compiles on Linux.
// TODO: implement org.mpris.MediaPlayer2.Player over github.com/godbus/dbus/v5

func startMediaKeys(_ func(Command)) {}
func stopMediaKeys()                 {}

// UpdateNowPlaying is a no-op on Linux until MPRIS is implemented.
func UpdateNowPlaying(_, _ string, _, _ float64, _ bool) {}

func registerHotkey(_ string, _ Command, _ func(Command)) {}
func unregisterCustom()                                    {}
