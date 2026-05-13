"""mpv player controller via python-mpv (libmpv bindings).

Requires mpv to be installed: brew install mpv
python-mpv links against the libmpv shared library that ships with mpv.
"""

from __future__ import annotations

import threading
from dataclasses import dataclass
from typing import Callable, Optional


@dataclass
class PlayerState:
    playing: bool = False
    position: float = 0.0
    duration: float = 0.0
    title: str = ""
    volume: int = 100


class Player:
    """Thin wrapper around python-mpv exposing the same interface the TUI needs."""

    def __init__(self) -> None:
        import mpv  # deferred so import errors are catchable by caller

        self._mpv = mpv.MPV(
            video=False,
            terminal=False,
            input_terminal=False,
            really_quiet=True,
        )
        self._state = PlayerState()
        self._lock = threading.RLock()
        self._on_track_end: Optional[Callable] = None

        # Observe playback properties so _state stays fresh without polling.
        self._mpv.observe_property("time-pos", self._on_time_pos)
        self._mpv.observe_property("duration", self._on_duration)
        self._mpv.observe_property("pause", self._on_pause)
        self._mpv.observe_property("volume", self._on_volume)
        self._mpv.observe_property("media-title", self._on_title)

        @self._mpv.event_callback("end-file")
        def _end_file(event):
            if self._on_track_end:
                self._on_track_end()

    # ------------------------------------------------------------------
    # Property observers (called from mpv's internal thread)
    # ------------------------------------------------------------------

    def _on_time_pos(self, _name: str, value) -> None:
        with self._lock:
            self._state.position = float(value) if value is not None else 0.0

    def _on_duration(self, _name: str, value) -> None:
        with self._lock:
            self._state.duration = float(value) if value is not None else 0.0

    def _on_pause(self, _name: str, value) -> None:
        with self._lock:
            self._state.playing = not bool(value) if value is not None else False

    def _on_volume(self, _name: str, value) -> None:
        with self._lock:
            self._state.volume = int(value) if value is not None else 100

    def _on_title(self, _name: str, value) -> None:
        with self._lock:
            self._state.title = str(value) if value else ""

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    @property
    def state(self) -> PlayerState:
        with self._lock:
            s = self._state
            return PlayerState(
                playing=s.playing,
                position=s.position,
                duration=s.duration,
                title=s.title,
                volume=s.volume,
            )

    def load(self, url: str) -> None:
        """Append a URL to the playlist and start playing immediately."""
        # append-play: add to end, start playing if idle
        self._mpv.command("loadfile", url, "append-play")

    def load_and_play(self, url: str) -> None:
        """Replace the current playlist with a single URL and play it."""
        self._mpv.command("loadfile", url, "replace")

    def play(self) -> None:
        self._mpv.pause = False

    def pause(self) -> None:
        self._mpv.pause = True

    def toggle(self) -> None:
        self._mpv.pause = not self._mpv.pause

    def next(self) -> None:
        try:
            self._mpv.command("playlist-next", "soft")
        except Exception:
            pass

    def prev(self) -> None:
        try:
            self._mpv.command("playlist-prev", "soft")
        except Exception:
            pass

    def seek(self, seconds: float) -> None:
        try:
            self._mpv.seek(seconds, reference="absolute")
        except Exception:
            pass

    def set_volume(self, vol: int) -> None:
        self._mpv.volume = max(0, min(100, vol))

    def clear_playlist(self) -> None:
        try:
            self._mpv.command("playlist-clear")
        except Exception:
            pass

    def close(self) -> None:
        try:
            self._mpv.terminate()
        except Exception:
            pass
