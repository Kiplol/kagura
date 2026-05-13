"""Navidrome TUI — textual application.

Screens
-------
LoginScreen  — first-run credential entry
MainScreen   — library browser + queue + playback

Key bindings (main screen)
--------------------------
1-5          Switch tab (Artists / Albums / Songs / Playlists / Search)
j / ↓        Move cursor down
k / ↑        Move cursor up
Enter        Select / drill into item / play song
Backspace    Go back one level
a            Enqueue all visible tracks
n            Insert all visible tracks next
Space        Play / pause
.  or  >     Next track
,  or  <     Previous track
+  or  =     Volume up
-            Volume down
v            Toggle bongo cat
q            Quit
"""

from __future__ import annotations

import threading
from dataclasses import dataclass
from typing import Optional, Union

from rich.text import Text
from textual import on, work
from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.containers import Horizontal, Vertical, VerticalScroll
from textual.message import Message
from textual.reactive import reactive
from textual.screen import Screen
from textual.widget import Widget
from textual.widgets import Input, Label, ListItem, ListView, Static

import config
from config import Config
from subsonic import Album, Artist, Playlist, Song, SubsonicClient

# ---------------------------------------------------------------------------
# Bongo cat ASCII frames (ported from bongocat.go)
# ---------------------------------------------------------------------------

_BONGO_IDLE = """\
                       /\\  /\\
  ___________________ /  \\/  \\
 / ●                           \\
|  ~~  .          (oo)          |
 \\_______________________________/
 ii
  i"""

_BONGO_BEAT = """\
 (oo)
  |    _________________  /\\  /\\
  \\___/ ●              _ /  \\/  \\
       |  ~~  .       (oo)       |
        \\_________________________/
                              ii
                               i"""


def _bongo_frame(phase: int) -> str:
    return _BONGO_IDLE if phase % 2 == 0 else _BONGO_BEAT


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _fmt_dur(secs: int) -> str:
    m, s = divmod(int(secs), 60)
    return f"{m}:{s:02d}"


def _progress_bar(pos: float, dur: float, width: int) -> str:
    """Return a fixed-width ASCII progress bar."""
    if width < 4 or dur <= 0:
        return " " * max(width, 0)
    inner = width - 2
    filled = int(inner * pos / dur)
    filled = max(0, min(filled, inner))
    bar = "=" * filled + (">" if filled < inner else "=") + " " * (inner - filled - (1 if filled < inner else 0))
    return f"[{bar[:inner]}]"


# ---------------------------------------------------------------------------
# ListItem data carrier
# ---------------------------------------------------------------------------


@dataclass
class _Item:
    label: str
    sub: str
    data: Union[Artist, Album, Song, Playlist]


# ---------------------------------------------------------------------------
# NowPlayingBar widget
# ---------------------------------------------------------------------------


class NowPlayingBar(Static):
    """Single-line bar docked at the bottom of the screen."""

    DEFAULT_CSS = """
    NowPlayingBar {
        dock: bottom;
        height: 1;
        background: #181825;
        color: #a6adc8;
        padding: 0 1;
    }
    """

    def update_state(
        self,
        title: str,
        pos: float,
        dur: float,
        vol: int,
        playing: bool,
    ) -> None:
        icon = "♪" if playing else "⏸"
        track = title if title else "no track loaded"

        # Reserve space: icon(1) + space(1) + track(variable) + space(2) +
        # bar(16) + space(1) + time(11) + space(1) + vol(7) = ~40 fixed chars
        bar = _progress_bar(pos, dur, 16)
        elapsed = _fmt_dur(pos)
        total = _fmt_dur(dur)
        vol_str = f"vol:{vol:3d}%"

        self.update(f"{icon} {track}  {bar} {elapsed}/{total}  {vol_str}")


# ---------------------------------------------------------------------------
# BongoCatPanel widget
# ---------------------------------------------------------------------------


class BongoCatPanel(Static):
    DEFAULT_CSS = """
    BongoCatPanel {
        width: 36;
        border-left: solid #313244;
        padding: 1 1;
        background: #1e1e2e;
        color: #585b70;
        display: none;
    }
    BongoCatPanel.visible {
        display: block;
    }
    """

    def update_phase(self, phase: int) -> None:
        self.update(_bongo_frame(phase))


# ---------------------------------------------------------------------------
# Browser widget
# ---------------------------------------------------------------------------

TABS = ["Artists", "Albums", "Songs", "Playlists", "Search"]

TAB_ARTISTS = 0
TAB_ALBUMS = 1
TAB_SONGS = 2
TAB_PLAYLISTS = 3
TAB_SEARCH = 4


class TabBar(Static):
    DEFAULT_CSS = """
    TabBar {
        height: 1;
        background: #1e1e2e;
    }
    """

    current: reactive[int] = reactive(0)

    def render(self) -> Text:
        t = Text()
        for i, name in enumerate(TABS):
            label = f" {i+1}:{name} "
            if i == self.current:
                t.append(label, "#cba6f7 bold underline")
            else:
                t.append(label, "#6c7086")
        return t


class BreadcrumbBar(Static):
    DEFAULT_CSS = """
    BreadcrumbBar {
        height: 1;
        background: #1e1e2e;
        color: #6c7086;
        padding: 0 1;
    }
    """


class BrowserWidget(Widget):
    """Library browser: tab bar + breadcrumb + scrollable list."""

    DEFAULT_CSS = """
    BrowserWidget {
        layout: vertical;
        height: 1fr;
    }
    ListView {
        height: 1fr;
        background: #1e1e2e;
        scrollbar-background: #1e1e2e;
        scrollbar-color: #313244;
        scrollbar-color-hover: #45475a;
        scrollbar-size-vertical: 1;
    }
    ListView > ListItem {
        padding: 0 1;
        background: #1e1e2e;
        color: #cdd6f4;
    }
    ListView:focus > ListItem.--highlight {
        background: #313244;
        color: #89b4fa;
    }
    """

    # Current items shown (used for "enqueue all")
    _items: list[_Item] = []

    def compose(self) -> ComposeResult:
        yield TabBar(id="tab-bar")
        yield BreadcrumbBar("", id="breadcrumb")
        yield ListView(id="item-list")
        yield Input(placeholder="Search…", id="search-input")

    def on_mount(self) -> None:
        self.query_one("#search-input", Input).display = False

    # ------------------------------------------------------------------
    # Public helpers called by MainScreen
    # ------------------------------------------------------------------

    def set_tab(self, idx: int) -> None:
        self.query_one("#tab-bar", TabBar).current = idx
        self.query_one("#search-input", Input).display = (idx == TAB_SEARCH)
        if idx == TAB_SEARCH:
            self.query_one("#search-input", Input).focus()
        else:
            self.query_one("#item-list", ListView).focus()

    def set_breadcrumb(self, text: str) -> None:
        self.query_one("#breadcrumb", BreadcrumbBar).update(text)

    def set_loading(self, msg: str = "  Loading…") -> None:
        lv = self.query_one("#item-list", ListView)
        lv.clear()
        self._items = []
        # Show a placeholder while data loads
        lv.append(ListItem(Label(Text(msg, style="italic magenta"))))

    def populate(self, items: list[_Item]) -> None:
        self._items = items
        lv = self.query_one("#item-list", ListView)
        lv.clear()
        for item in items:
            t = Text()
            t.append(item.label, "white")
            if item.sub:
                t.append(f"  {item.sub}", "dim")
            lv.append(ListItem(Label(t)))
        if items:
            lv.index = 0

    def selected_item(self) -> Optional[_Item]:
        lv = self.query_one("#item-list", ListView)
        idx = lv.index
        if idx is not None and 0 <= idx < len(self._items):
            return self._items[idx]
        return None

    def all_songs(self) -> list[Song]:
        return [i.data for i in self._items if isinstance(i.data, Song)]

    def focus_list(self) -> None:
        self.query_one("#item-list", ListView).focus()

    # ------------------------------------------------------------------
    # Key handling — j/k navigation, list shortcuts
    # ------------------------------------------------------------------

    def on_key(self, event) -> None:
        key = event.key
        lv = self.query_one("#item-list", ListView)
        search = self.query_one("#search-input", Input)

        # If the search input is focused, let it handle most keys
        if search.has_focus:
            if key == "escape":
                search.value = ""
                lv.focus()
                event.stop()
            return

        if key in ("j", "down"):
            lv.action_cursor_down()
            event.stop()
        elif key in ("k", "up"):
            lv.action_cursor_up()
            event.stop()


# ---------------------------------------------------------------------------
# Login screen
# ---------------------------------------------------------------------------


class LoginScreen(Screen):
    DEFAULT_CSS = """
    LoginScreen {
        align: center middle;
        background: #1e1e2e;
    }
    #login-box {
        width: 52;
        border: round #45475a;
        padding: 1 2;
        background: #181825;
    }
    #login-box Label {
        margin-bottom: 1;
        color: #cdd6f4;
    }
    #login-box Input {
        margin-bottom: 1;
        background: #313244;
        border: tall #585b70;
        color: #cdd6f4;
    }
    #login-box Input:focus {
        border: tall #89b4fa;
    }
    #error-msg {
        color: #f38ba8;
        height: 1;
    }
    """

    BINDINGS = [Binding("escape", "app.quit", "Quit")]

    def compose(self) -> ComposeResult:
        with Vertical(id="login-box"):
            yield Label("[bold #cba6f7]Navidrome TUI[/bold #cba6f7]\n[#6c7086]Enter your server details[/#6c7086]")
            yield Input(placeholder="Server URL  (e.g. https://music.example.com)", id="url")
            yield Input(placeholder="Username", id="username")
            yield Input(placeholder="Password", password=True, id="password")
            yield Label("", id="error-msg")

    def on_mount(self) -> None:
        self.query_one("#url", Input).focus()

    def on_input_submitted(self, event: Input.Submitted) -> None:
        # Tab between fields on Enter, submit on the last field
        url_w = self.query_one("#url", Input)
        user_w = self.query_one("#username", Input)
        pass_w = self.query_one("#password", Input)

        if event.input is url_w:
            user_w.focus()
        elif event.input is user_w:
            pass_w.focus()
        elif event.input is pass_w:
            self._do_login()

    def _do_login(self) -> None:
        url = self.query_one("#url", Input).value.strip()
        username = self.query_one("#username", Input).value.strip()
        password = self.query_one("#password", Input).value

        if not url or not username or not password:
            self.query_one("#error-msg", Label).update("All fields required.")
            return

        self.query_one("#error-msg", Label).update("[italic]Connecting…[/italic]")
        self._login_async(url, username, password)

    @work(thread=True)
    def _login_async(self, url: str, username: str, password: str) -> None:
        client = SubsonicClient(url, username, password)
        ok = client.ping()

        def _done():
            if ok:
                cfg = config.Config(
                    server=config.ServerConfig(url=url, username=username, password=password)
                )
                config.save(cfg)
                self.app.push_screen(MainScreen(cfg))
            else:
                self.query_one("#error-msg", Label).update(
                    "[red]Could not connect — check URL and credentials.[/red]"
                )

        self.app.call_from_thread(_done)


# ---------------------------------------------------------------------------
# Main screen
# ---------------------------------------------------------------------------


class MainScreen(Screen):
    DEFAULT_CSS = """
    MainScreen {
        layout: vertical;
        background: #1e1e2e;
    }
    #main-row {
        layout: horizontal;
        height: 1fr;
    }
    #separator {
        height: 1;
        background: #181825;
        color: #313244;
    }
    """

    BINDINGS = [
        Binding("q", "quit", "Quit", show=False),
        Binding("space", "toggle_play", "Play/Pause", show=False),
        Binding("v", "toggle_bongo", "Bongo", show=False),
        Binding("1", "tab_1", "Artists", show=False),
        Binding("2", "tab_2", "Albums", show=False),
        Binding("3", "tab_3", "Songs", show=False),
        Binding("4", "tab_4", "Playlists", show=False),
        Binding("5", "tab_5", "Search", show=False),
        Binding("enter", "select_item", "Select", show=False),
        Binding("backspace", "go_back", "Back", show=False),
        Binding("a", "enqueue_all", "Add all", show=False),
        Binding("n", "insert_next", "Insert next", show=False),
    ]

    def on_key(self, event) -> None:
        """Handle symbol keys that can't go in BINDINGS (textual parses , as separator)."""
        k = event.key
        if k in ("full_stop", "greater_than_sign"):    # . >
            self.action_next_track(); event.stop()
        elif k in ("comma", "less_than_sign"):          # , <
            self.action_prev_track(); event.stop()
        elif k in ("plus", "equals_sign"):              # + =
            self.action_vol_up(); event.stop()
        elif k == "minus":                              # -
            self.action_vol_down(); event.stop()
        elif k == "slash":                              # /
            self.action_focus_search(); event.stop()

    def __init__(self, cfg: Config) -> None:
        super().__init__()
        self._cfg = cfg
        self._client = SubsonicClient(
            cfg.server.url, cfg.server.username, cfg.server.password
        )
        self._player: Optional[object] = None  # type: ignore[type-arg]

        # Browser state
        self._tab = TAB_ARTISTS
        self._selected_artist: Optional[Artist] = None
        self._selected_album: Optional[Album] = None
        self._selected_playlist: Optional[Playlist] = None

        # Queue
        self._queue: list[Song] = []
        self._queue_idx: int = -1

        # Bongo
        self._bongo_phase: int = 0
        self._show_bongo: bool = False

    # ------------------------------------------------------------------
    # Compose & mount
    # ------------------------------------------------------------------

    def compose(self) -> ComposeResult:
        with Horizontal(id="main-row"):
            yield BrowserWidget(id="browser")
            yield BongoCatPanel(id="bongo")
        yield Static(id="separator")
        yield NowPlayingBar("♪  no track loaded", id="nowplaying")

    def on_mount(self) -> None:
        # Wire separator
        self.query_one("#separator", Static).update(
            "─" * self.app.console.width
        )

        # Start player
        try:
            from player import Player
            self._player = Player()
        except Exception:
            self._player = None

        # Media keys
        import hotkeys
        hotkeys.setup(self._media_key_handler)

        # Kick off initial data load
        self._load_tab(TAB_ARTISTS)

        # Ticker for now-playing bar + bongo animation
        self.set_interval(0.5, self._tick)

        # Focus the list
        self.query_one("#browser", BrowserWidget).focus_list()

    # ------------------------------------------------------------------
    # Tick — updates now-playing bar and bongo animation
    # ------------------------------------------------------------------

    def _tick(self) -> None:
        if self._player is None:
            return
        st = self._player.state
        self._bongo_phase = int(st.position * 2)

        self.query_one("#nowplaying", NowPlayingBar).update_state(
            title=st.title,
            pos=st.position,
            dur=st.duration,
            vol=st.volume,
            playing=st.playing,
        )

        if self._show_bongo:
            self.query_one("#bongo", BongoCatPanel).update_phase(self._bongo_phase)

        import hotkeys
        hotkeys.update_now_playing(
            title=st.title,
            artist="",
            duration=st.duration,
            position=st.position,
            playing=st.playing,
        )

    # ------------------------------------------------------------------
    # Media key handler (called from background thread)
    # ------------------------------------------------------------------

    def _media_key_handler(self, cmd: str) -> None:
        def _dispatch():
            if self._player is None:
                return
            if cmd == "play":
                self._player.play()
            elif cmd == "pause":
                self._player.pause()
            elif cmd == "toggle":
                self._player.toggle()
            elif cmd == "next":
                self._player.next()
            elif cmd == "prev":
                self._player.prev()

        self.app.call_from_thread(_dispatch)

    # ------------------------------------------------------------------
    # Tab loading
    # ------------------------------------------------------------------

    def _load_tab(self, tab: int) -> None:
        self._tab = tab
        self._selected_artist = None
        self._selected_album = None
        self._selected_playlist = None
        b = self.query_one("#browser", BrowserWidget)
        b.set_tab(tab)
        b.set_loading()
        self._update_breadcrumb()

        if tab == TAB_ARTISTS:
            self._fetch_artists()
        elif tab == TAB_ALBUMS:
            self._fetch_all_albums()
        elif tab == TAB_SONGS:
            self._fetch_random_songs()
        elif tab == TAB_PLAYLISTS:
            self._fetch_playlists()
        elif tab == TAB_SEARCH:
            b.set_loading("  Type and press Enter to search…")

    def _update_breadcrumb(self) -> None:
        parts = [TABS[self._tab]]
        if self._selected_artist:
            parts.append(self._selected_artist.name)
        if self._selected_album:
            parts.append(self._selected_album.name)
        if self._selected_playlist:
            parts.append(self._selected_playlist.name)
        self.query_one("#browser", BrowserWidget).set_breadcrumb(
            "  " + " › ".join(parts)
        )

    # ------------------------------------------------------------------
    # Data fetchers (run in background threads)
    # ------------------------------------------------------------------

    @work(thread=True)
    def _fetch_artists(self) -> None:
        try:
            artists = self._client.get_artists()
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(
                    f"  Error: {e}"
                )
            )
            return
        items = [
            _Item(a.name, f"{a.album_count} album{'s' if a.album_count != 1 else ''}", a)
            for a in artists
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_all_albums(self) -> None:
        try:
            albums = self._client.get_all_albums()
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(f"  Error: {e}")
            )
            return
        items = [
            _Item(
                f"{a.name}{f'  ({a.year})' if a.year else ''}",
                a.artist,
                a,
            )
            for a in albums
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_random_songs(self) -> None:
        try:
            songs = self._client.get_random_songs()
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(f"  Error: {e}")
            )
            return
        items = [
            _Item(
                s.title,
                f"{s.artist}  •  {_fmt_dur(s.duration)}" if s.artist else _fmt_dur(s.duration),
                s,
            )
            for s in songs
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_albums(self, artist: Artist) -> None:
        try:
            albums = self._client.get_albums(artist.id)
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(
                    f"  Error: {e}"
                )
            )
            return
        items = [
            _Item(
                f"{a.name}{f'  ({a.year})' if a.year else ''}",
                f"{a.song_count} track{'s' if a.song_count != 1 else ''}",
                a,
            )
            for a in albums
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_songs(self, album: Album) -> None:
        try:
            songs = self._client.get_songs(album.id)
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(
                    f"  Error: {e}"
                )
            )
            return
        items = [
            _Item(
                f"{'%02d. ' % s.track if s.track else ''}{s.title}",
                "  •  ".join(
                    filter(
                        None,
                        [_fmt_dur(s.duration), f"{s.bit_rate} kbps" if s.bit_rate else "", s.suffix.upper() if s.suffix else ""],
                    )
                ),
                s,
            )
            for s in songs
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_playlists(self) -> None:
        try:
            playlists = self._client.get_playlists()
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(
                    f"  Error: {e}"
                )
            )
            return
        items = [
            _Item(p.name, f"{p.song_count} tracks  •  {_fmt_dur(p.duration)}", p)
            for p in playlists
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_playlist_songs(self, playlist: Playlist) -> None:
        try:
            songs = self._client.get_playlist_songs(playlist.id)
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(
                    f"  Error: {e}"
                )
            )
            return
        items = [
            _Item(
                f"{'%02d. ' % s.track if s.track else ''}{s.title}",
                f"{s.artist}  •  {_fmt_dur(s.duration)}" if s.artist else _fmt_dur(s.duration),
                s,
            )
            for s in songs
        ]
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    @work(thread=True)
    def _fetch_search(self, query: str) -> None:
        try:
            artists, albums, songs = self._client.search(query)
        except Exception as e:
            self.app.call_from_thread(
                lambda: self.query_one("#browser", BrowserWidget).set_loading(
                    f"  Error: {e}"
                )
            )
            return
        items: list[_Item] = []
        for a in artists:
            items.append(_Item(a.name, "Artist", a))
        for a in albums:
            items.append(_Item(a.name, f"Album  •  {a.artist}", a))
        for s in songs:
            items.append(_Item(s.title, f"{s.artist}  •  {s.album}", s))
        self.app.call_from_thread(
            lambda: self.query_one("#browser", BrowserWidget).populate(items)
        )

    # ------------------------------------------------------------------
    # Queue helpers
    # ------------------------------------------------------------------

    def _enqueue(self, songs: list[Song], insert_next: bool = False) -> None:
        if not songs:
            return
        if insert_next and self._queue_idx + 1 <= len(self._queue):
            tail = self._queue[self._queue_idx + 1 :]
            self._queue = self._queue[: self._queue_idx + 1] + songs + tail
        else:
            self._queue.extend(songs)

        if self._player:
            for s in songs:
                self._player.load(self._client.stream_url(s.id))

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    def action_quit(self) -> None:
        import hotkeys
        hotkeys.teardown()
        if self._player:
            self._player.close()
        self.app.exit()

    def action_toggle_play(self) -> None:
        if self._player:
            self._player.toggle()

    def action_next_track(self) -> None:
        if self._player:
            self._player.next()

    def action_prev_track(self) -> None:
        if self._player:
            self._player.prev()

    def action_vol_up(self) -> None:
        if self._player:
            st = self._player.state
            self._player.set_volume(min(st.volume + 5, 100))

    def action_vol_down(self) -> None:
        if self._player:
            st = self._player.state
            self._player.set_volume(max(st.volume - 5, 0))

    def action_toggle_bongo(self) -> None:
        self._show_bongo = not self._show_bongo
        panel = self.query_one("#bongo", BongoCatPanel)
        if self._show_bongo:
            panel.add_class("visible")
        else:
            panel.remove_class("visible")

    def action_tab_1(self) -> None:
        self._load_tab(TAB_ARTISTS)

    def action_tab_2(self) -> None:
        self._load_tab(TAB_ALBUMS)

    def action_tab_3(self) -> None:
        self._load_tab(TAB_SONGS)

    def action_tab_4(self) -> None:
        self._load_tab(TAB_PLAYLISTS)

    def action_tab_5(self) -> None:
        self._load_tab(TAB_SEARCH)

    def action_focus_search(self) -> None:
        self._load_tab(TAB_SEARCH)

    def action_select_item(self) -> None:
        browser = self.query_one("#browser", BrowserWidget)
        search = browser.query_one("#search-input", Input)

        # If search input is focused and has content, run the search
        if search.has_focus:
            query = search.value.strip()
            if query:
                browser.set_loading("  Searching…")
                self._fetch_search(query)
            return

        item = browser.selected_item()
        if item is None:
            return

        if isinstance(item.data, Song):
            self._enqueue([item.data])
        elif isinstance(item.data, Artist):
            self._selected_artist = item.data
            self._selected_album = None
            self._update_breadcrumb()
            browser.set_loading()
            self._fetch_albums(item.data)
        elif isinstance(item.data, Album):
            self._selected_album = item.data
            self._update_breadcrumb()
            browser.set_loading()
            self._fetch_songs(item.data)
        elif isinstance(item.data, Playlist):
            self._selected_playlist = item.data
            self._update_breadcrumb()
            browser.set_loading()
            self._fetch_playlist_songs(item.data)

    def action_go_back(self) -> None:
        browser = self.query_one("#browser", BrowserWidget)
        if self._selected_album is not None:
            self._selected_album = None
            self._update_breadcrumb()
            if self._selected_artist:
                browser.set_loading()
                self._fetch_albums(self._selected_artist)
        elif self._selected_playlist is not None:
            self._selected_playlist = None
            self._update_breadcrumb()
            browser.set_loading()
            self._fetch_playlists()
        elif self._selected_artist is not None:
            self._selected_artist = None
            self._update_breadcrumb()
            browser.set_loading()
            self._fetch_artists()

    def action_enqueue_all(self) -> None:
        songs = self.query_one("#browser", BrowserWidget).all_songs()
        self._enqueue(songs)

    def action_insert_next(self) -> None:
        songs = self.query_one("#browser", BrowserWidget).all_songs()
        self._enqueue(songs, insert_next=True)

    # ------------------------------------------------------------------
    # Handle ListView selection via mouse click
    # ------------------------------------------------------------------

    @on(ListView.Selected)
    def on_list_view_selected(self, event: ListView.Selected) -> None:
        self.action_select_item()


# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------


class NavidromeApp(App):
    TITLE = "Navidrome TUI"
    CSS = """
    Screen {
        background: #1e1e2e;
        color: #cdd6f4;
    }
    """

    def on_mount(self) -> None:
        cfg = config.load()
        if cfg.server.url:
            self.push_screen(MainScreen(cfg))
        else:
            self.push_screen(LoginScreen())
