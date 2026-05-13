"""macOS media key integration via MPRemoteCommandCenter (pyobjc).

Install the optional dependency to enable:
    pip install pyobjc-framework-MediaPlayer

If pyobjc is not installed the app runs normally — media keys just won't
register with Control Center / AirPods / lock screen.
"""

from __future__ import annotations

import sys
import threading
from typing import Callable, Optional

_cleanup: Optional[Callable] = None


def setup(handler: Callable[[str], None]) -> None:
    """Register media key handlers. handler receives one of:
    'play', 'pause', 'toggle', 'next', 'prev'
    """
    global _cleanup
    if sys.platform == "darwin":
        _cleanup = _setup_darwin(handler)


def teardown() -> None:
    if _cleanup:
        _cleanup()


def update_now_playing(
    title: str,
    artist: str,
    duration: float,
    position: float,
    playing: bool,
) -> None:
    if sys.platform != "darwin":
        return
    try:
        from MediaPlayer import (  # type: ignore[import]
            MPMediaItemPropertyArtist,
            MPMediaItemPropertyPlaybackDuration,
            MPMediaItemPropertyTitle,
            MPNowPlayingInfoCenter,
            MPNowPlayingInfoPropertyElapsedPlaybackTime,
            MPNowPlayingInfoPropertyMediaType,
            MPNowPlayingInfoPropertyPlaybackRate,
        )

        info: dict = {}
        if title:
            info[MPMediaItemPropertyTitle] = title
        if artist:
            info[MPMediaItemPropertyArtist] = artist
        if duration > 0:
            info[MPMediaItemPropertyPlaybackDuration] = duration
        info[MPNowPlayingInfoPropertyElapsedPlaybackTime] = position
        info[MPNowPlayingInfoPropertyPlaybackRate] = 1.0 if playing else 0.0
        # 1 = audio
        info[MPNowPlayingInfoPropertyMediaType] = 1
        MPNowPlayingInfoCenter.defaultCenter().setNowPlayingInfo_(info)
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Darwin implementation
# ---------------------------------------------------------------------------


def _setup_darwin(handler: Callable[[str], None]) -> Optional[Callable]:
    try:
        from MediaPlayer import (  # type: ignore[import]
            MPRemoteCommandCenter,
            MPRemoteCommandHandlerStatusSuccess,
        )
        from Foundation import NSRunLoop  # type: ignore[import]
    except ImportError:
        return None  # pyobjc not available

    cc = MPRemoteCommandCenter.sharedCommandCenter()

    def _h(cmd: str):
        def _inner(_event):
            handler(cmd)
            return MPRemoteCommandHandlerStatusSuccess
        return _inner

    play_target = cc.playCommand().addTargetWithHandler_(_h("play"))
    pause_target = cc.pauseCommand().addTargetWithHandler_(_h("pause"))
    toggle_target = cc.togglePlayPauseCommand().addTargetWithHandler_(_h("toggle"))
    next_target = cc.nextTrackCommand().addTargetWithHandler_(_h("next"))
    prev_target = cc.previousTrackCommand().addTargetWithHandler_(_h("prev"))

    # MPRemoteCommandCenter requires an ObjC run loop to dispatch events.
    loop_thread = threading.Thread(
        target=lambda: NSRunLoop.currentRunLoop().run(),
        daemon=True,
    )
    loop_thread.start()

    def _teardown():
        cc.playCommand().removeTarget_(play_target)
        cc.pauseCommand().removeTarget_(pause_target)
        cc.togglePlayPauseCommand().removeTarget_(toggle_target)
        cc.nextTrackCommand().removeTarget_(next_target)
        cc.previousTrackCommand().removeTarget_(prev_target)

    return _teardown
