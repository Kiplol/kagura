//go:build darwin

package hotkey

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework MediaPlayer -framework Foundation

#include <stdlib.h>

// Forward declarations for functions defined in media_keys_darwin.m
extern void setupRemoteCommands(void);
extern void teardownRemoteCommands(void);
extern void updateNowPlayingInfo(const char *title, const char *artist,
                                  double duration, double position, int playing);
extern void startObjCRunLoop(void);
*/
import "C"
import (
	"sync"
	"unsafe"
)

var (
	mediaHandler   func(Command)
	mediaHandlerMu sync.Mutex
)

// goMediaKeyCallback is called from Objective-C when a media key fires.
// It must be exported so the linker can find it from the .m file.
//
//export goMediaKeyCallback
func goMediaKeyCallback(cmd C.int) {
	mediaHandlerMu.Lock()
	h := mediaHandler
	mediaHandlerMu.Unlock()
	if h != nil {
		h(Command(cmd))
	}
}

func startMediaKeys(h func(Command)) {
	mediaHandlerMu.Lock()
	mediaHandler = h
	mediaHandlerMu.Unlock()

	// MPRemoteCommandCenter requires a run loop — spin one up on a dedicated
	// goroutine so it never blocks the bubbletea event loop.
	go C.startObjCRunLoop()
	C.setupRemoteCommands()
}

func stopMediaKeys() {
	mediaHandlerMu.Lock()
	mediaHandler = nil
	mediaHandlerMu.Unlock()
	C.teardownRemoteCommands()
}

// UpdateNowPlaying pushes metadata to the macOS Now Playing widget
// (Control Center, lock screen, AirPods HUD).
func UpdateNowPlaying(title, artist string, duration, position float64, playing bool) {
	ct := C.CString(title)
	ca := C.CString(artist)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(ca))
	var p C.int
	if playing {
		p = 1
	}
	C.updateNowPlayingInfo(ct, ca, C.double(duration), C.double(position), p)
}

// Custom hotkey stubs — on macOS we rely on media keys for the defaults;
// custom hotkey registration via Carbon/CGEventTap is a future enhancement.
func registerHotkey(_ string, _ Command, _ func(Command)) {}
func unregisterCustom()                                    {}
