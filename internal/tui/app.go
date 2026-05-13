// Package tui is the tview-based terminal UI for Navidrome TUI.
// Uses tcell.ColorDefault for backgrounds (transparent to terminal theme) and
// ANSI 0-15 indices for accent colors, which the terminal theme remaps automatically.
package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	bpmdetect "github.com/kip/kagura/internal/bpm"
	"github.com/kip/kagura/internal/config"
	"github.com/kip/kagura/internal/hotkey"
	"github.com/kip/kagura/internal/mpv"
	"github.com/kip/kagura/internal/subsonic"
)

const (
	tabArtists   = 0
	tabAlbums    = 1
	tabSongs     = 2
	tabPlaylists = 3
	tabSearch    = 4
)

var tabNames = [5]string{"Artists", "Albums", "Songs", "Playlists", "Search"}

// queuePaneWidth is the total character width of the right-side queue+cat panel.
const queuePaneWidth = 28

// pageSize is the number of items shown per page in the browser list.
const pageSize = 50

// lyricsPanelHeight is the number of rows reserved for lyrics (prev/cur/next).
// Extra rows give wrapped lines room without clipping.
const lyricsPanelHeight = 5

// numBars is the number of bars in the vertical-bars visualizer (2 chars each).
const numBars = queuePaneWidth / 2

// catPanelHeight is the number of rows reserved for the bongo cat visualizer.
const catPanelHeight = 7

// djFrame builds a DJ dancer kaomoji frame.
// pose 0 = left arm up ┏...┛, pose 1 = right arm up ┗...┓, pose -1 = idle.
// bpm is displayed on the bottom line; pass 0 to show "---".
func djFrame(pose, bpm int) string {
	var bpmStr string
	if bpm > 0 {
		bpmStr = fmt.Sprintf("[gray]♩ %d bpm[-]", bpm)
	} else {
		bpmStr = "[gray]♩ ---[-]"
	}
	switch pose {
	case 0:
		return "  [yellow]♪[-]  [yellow]♫[-]  [yellow]♪[-]\n" +
			" [white]┏[gray](･o･)[white]┛[-]\n" +
			"  [gray]( DJ~ )[-]\n" +
			"  [gray]└─────┘[-]\n" +
			"  " + bpmStr
	case 1:
		return "  [yellow]♪[-]  [yellow]♫[-]  [yellow]♪[-]\n" +
			" [white]┗[gray](･o･)[white]┓[-]\n" +
			"  [gray]( ~DJ )[-]\n" +
			"  [gray]└─────┘[-]\n" +
			"  " + bpmStr
	default: // idle
		return "\n" +
			"  [gray](･o･)[-]\n" +
			"  [gray]( DJ  )[-]\n" +
			"  [gray]└─────┘[-]\n" +
			"  " + bpmStr
	}
}

// loginArt is the bonsai sakura tree shown on the login screen.
// Uses tview dynamic color tags; hex colors are decorative-only (login art, not UI chrome).
const loginArt = "" +
	"         [#f9a8c9]✿  ✿✿✿  ✿[-]\n" +
	"       [#f472b6]✿✿✿ ✿ ✿✿✿✿[-]\n" +
	"     [#f9a8c9]✿✿[-]  [#a78fa8]\\[-]  [#f472b6]✿[-]  [#a78fa8]/[-]  [#f9a8c9]✿✿[-]\n" +
	"      [#f9a8c9]✿[-]   [#a78fa8]\\[-]     [#a78fa8]/[-]   [#f472b6]✿[-]\n" +
	"    [#f472b6]✿✿[-]     [#a78fa8]\\[-]   [#a78fa8]/[-]     [#f9a8c9]✿✿[-]\n" +
	"     [#f9a8c9]✿[-]  [#a78fa8]\\   \\ /   /[-]  [#f472b6]✿[-]\n" +
	"    [#f472b6]✿✿[-] [#a78fa8]\\ \\   |   / /[-] [#f9a8c9]✿✿[-]\n" +
	"            [#a78fa8]\\  \\|/ /[-]\n" +
	"             [#a78fa8]\\  |/[-]\n" +
	"              [#a78fa8]\\ |[-]\n" +
	"           [#a78fa8]----\\|[-]\n" +
	"          [#a78fa8]/      |[-]\n" +
	"         [#a78fa8]|        |[-]\n" +
	"       [#7c6b7e]~~~~~~~~~~|~~~[-]\n" +
	"      [#5a4a5c]╰═══════════════╯[-]\n" +
	"\n" +
	"           [white::b]KAGURA[-]\n" +
	"            [#a78fa8]神　楽[-]"

// ---------------------------------------------------------------------------

type listItem struct {
	label string
	data  interface{} // subsonic.Artist | Album | Song | Playlist | nil (header)
}

type navFrame struct {
	breadText string
	items     []listItem
	allItems  []listItem
	pageNum   int
	index     int
}

// App is the root tview application.
type App struct {
	tv    *tview.Application
	pages *tview.Pages
	cfg   config.Config

	client   *subsonic.Client
	player   *mpv.Player
	hkDaemon *hotkey.Daemon

	// Widgets
	tabBar      *tview.TextView
	breadcrumb  *tview.TextView
	list        *tview.Table // Table avoids tview.List secondary-text quirks
	searchInput *tview.InputField
	contentFlex *tview.Flex
	nowBar      *tview.TextView
	queuePanel  *tview.TextView // right pane: scrolling queue list
	catPanel    *tview.TextView // right pane: bongo cat visualizer
	catPhase       int             // beat counter — drives both bongo and bars
	beatInterval   time.Duration   // time between beats (from BPM)
	lastBeat       time.Time       // when the last beat fired
	visualizerMode int             // 0 = bongo cat, 1 = vertical bars
	lyricsPanel    *tview.TextView // right pane: synced lyrics
	currentSongID  string          // ID of the song whose lyrics are loaded
	lyricsLines    []subsonic.LyricLine
	lyricsSynced   bool            // true if lyricsLines have timestamps
	hintsBar       *tview.TextView // bottom key-hints bar (toggle with ?)
	rootFlex       *tview.Flex     // root layout flex, kept for hints toggle
	showHints      bool

	// Navigation state (touch only inside QueueUpdateDraw)
	tab          int
	currentItems []listItem // current page's items
	allItems     []listItem // full item list for current level
	pageNum      int        // current page (0-indexed)
	navStack     []navFrame
	breadText    string

	// Playback queue (guarded by mu)
	mu       sync.Mutex
	queue    []subsonic.Song
	queueIdx int

	// Auto DJ — appends similar songs when the queue runs low
	autoDJ         bool
	autoDJFetching bool
	autoDJSource   string // "similar" | "random" | ""

	// Play queue persistence
	saveTickCount int // incremented each ticker tick; save every ~10 s

	currentBPM int // actual BPM from tag (0 = unknown, drives display)
}

// Run builds and runs the application. Blocks until quit.
func Run(cfg config.Config) error {
	a := &App{cfg: cfg}
	return a.run()
}

func (a *App) run() error {
	// Terminal-native colors: ColorDefault = transparent to terminal theme.
	// ANSI 0-15 constants are sent as ANSI codes, which the terminal remaps.
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorBlack
	tview.Styles.BorderColor = tcell.ColorGray
	tview.Styles.TitleColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = tcell.ColorDefault
	tview.Styles.SecondaryTextColor = tcell.ColorGray

	a.tv = tview.NewApplication()
	a.pages = tview.NewPages()
	a.tv.SetRoot(a.pages, true)

	// Clear the whole screen before every draw pass so no old content bleeds
	// through transparent (ColorDefault) cells.
	a.tv.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		screen.Clear()
		return false
	})

	if a.cfg.Server.URL == "" {
		a.buildLoginPage()
	} else {
		a.initClient()
		a.buildMainPage()
		a.pages.SwitchToPage("main")
		go a.fetchTab(tabArtists)
	}

	go a.ticker()
	return a.tv.Run()
}

// ---------------------------------------------------------------------------
// Login screen
// ---------------------------------------------------------------------------

func (a *App) buildLoginPage() {
	status := tview.NewTextView().SetDynamicColors(true)
	status.SetBackgroundColor(tcell.ColorDefault)

	serverURL := a.cfg.Server.URL
	username := a.cfg.Server.Username
	password := ""

	bonsai := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	bonsai.SetBackgroundColor(tcell.ColorDefault)
	bonsai.SetText(loginArt)

	form := tview.NewForm()
	form.SetBorder(true).
		SetBorderColor(tcell.ColorGray)
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetFieldBackgroundColor(tcell.ColorBlack)
	form.SetFieldTextColor(tcell.ColorDefault)
	form.SetLabelColor(tcell.ColorGray)
	form.SetButtonBackgroundColor(tcell.ColorBlack)
	form.SetButtonTextColor(tcell.ColorDefault)

	form.AddInputField("Server URL", serverURL, 36, nil, func(v string) { serverURL = v })
	form.AddInputField("Username  ", username, 36, nil, func(v string) { username = v })
	form.AddPasswordField("Password  ", password, 36, '*', func(v string) { password = v })
	form.AddButton("Login", func() {
		if serverURL == "" || username == "" || password == "" {
			status.SetText("[red]All fields required.[-]")
			return
		}
		status.SetText("[gray]Connecting…[-]")
		go func() {
			c := subsonic.New(serverURL, username, password)
			err := c.Ping()
			a.tv.QueueUpdateDraw(func() {
				if err != nil {
					status.SetText(fmt.Sprintf("[red]Login failed: %v[-]", err))
					return
				}
				a.cfg.Server = config.Server{URL: serverURL, Username: username, Password: password}
				_ = config.Save(a.cfg)
				a.initClient()
				a.buildMainPage()
				a.pages.SwitchToPage("main")
				go a.fetchTab(tabArtists)
			})
		}()
	})
	form.SetCancelFunc(func() { a.tv.Stop() })

	newBox := func() *tview.Box {
		b := tview.NewBox()
		b.SetBackgroundColor(tcell.ColorDefault)
		return b
	}
	outer := tview.NewFlex().
		AddItem(newBox(), 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(newBox(), 0, 1, false).
			AddItem(bonsai, 18, 0, false).
			AddItem(form, 11, 0, true).
			AddItem(status, 1, 0, false).
			AddItem(newBox(), 0, 1, false),
			50, 0, true).
		AddItem(newBox(), 0, 1, false)
	outer.SetBackgroundColor(tcell.ColorDefault)

	a.pages.AddPage("login", outer, true, true)
}

// ---------------------------------------------------------------------------
// Main screen
// ---------------------------------------------------------------------------

func (a *App) initClient() {
	a.client = subsonic.New(a.cfg.Server.URL, a.cfg.Server.Username, a.cfg.Server.Password)
	var err error
	a.player, err = mpv.New()
	if err != nil {
		_ = err
	}
	a.hkDaemon = hotkey.New(func(cmd hotkey.Command) {
		a.tv.QueueUpdateDraw(func() { a.handleHotkeyCmd(cmd) })
	})
	a.hkDaemon.Start(a.cfg.Hotkeys)
}

func (a *App) buildMainPage() {
	// --- Tab bar ---
	a.tabBar = tview.NewTextView().SetDynamicColors(true)
	a.tabBar.SetBackgroundColor(tcell.ColorBlack) // ANSI 0 — subtle surface bar

	// --- Breadcrumb ---
	a.breadcrumb = tview.NewTextView().SetDynamicColors(true)
	a.breadcrumb.SetTextColor(tcell.ColorGray)
	a.breadcrumb.SetBackgroundColor(tcell.ColorDefault)

	// --- Browser list — Table avoids tview.List secondary-text rendering quirks ---
	selStyle := tcell.StyleDefault.
		Background(tcell.ColorBlack). // ANSI 0
		Foreground(tcell.ColorNavy)   // ANSI 4 → blue
	a.list = tview.NewTable().SetSelectable(true, false)
	a.list.SetSelectedStyle(selStyle)
	a.list.SetBackgroundColor(tcell.ColorDefault)
	a.list.SetSelectedFunc(func(row, col int) {
		a.handleSelect(row)
	})

	// --- Search input (visible only on search tab) ---
	a.searchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetLabelColor(tcell.ColorGray).
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetFieldTextColor(tcell.ColorDefault)
	a.searchInput.SetBackgroundColor(tcell.ColorDefault)
	a.searchInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			if q := a.searchInput.GetText(); q != "" {
				go a.fetchSearch(q)
			}
			a.tv.SetFocus(a.list)
		case tcell.KeyEscape:
			a.tv.SetFocus(a.list)
		}
	})

	// --- Browser content area (left side) ---
	a.contentFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	a.contentFlex.SetBackgroundColor(tcell.ColorDefault)
	a.contentFlex.AddItem(a.list, 0, 1, true)

	// --- Queue panel (right side, top) ---
	a.queuePanel = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.queuePanel.SetBackgroundColor(tcell.ColorDefault)

	// --- Lyrics panel (right side, middle) ---
	a.lyricsPanel = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	a.lyricsPanel.SetBackgroundColor(tcell.ColorDefault)

	// --- Visualizer panel (right side, bottom) ---
	a.catPanel = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.catPanel.SetBackgroundColor(tcell.ColorDefault)
	a.updateVisualizerPanel()

	// Dividers
	lyricsSep := tview.NewTextView().SetWrap(false)
	lyricsSep.SetText(strings.Repeat("─", 100))
	lyricsSep.SetTextColor(tcell.ColorGray)
	lyricsSep.SetBackgroundColor(tcell.ColorDefault)

	catDiv := tview.NewTextView().SetWrap(false)
	catDiv.SetText(strings.Repeat("─", 100))
	catDiv.SetTextColor(tcell.ColorGray)
	catDiv.SetBackgroundColor(tcell.ColorDefault)

	// Right pane: queue list | lyrics | visualizer
	rightPane := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.queuePanel, 0, 1, false).
		AddItem(lyricsSep, 1, 0, false).
		AddItem(a.lyricsPanel, lyricsPanelHeight, 0, false).
		AddItem(catDiv, 1, 0, false).
		AddItem(a.catPanel, catPanelHeight, 0, false)
	rightPane.SetBackgroundColor(tcell.ColorDefault)

	// Thin vertical separator between browser and right pane (1 char, gray bg)
	vSep := tview.NewBox()
	vSep.SetBackgroundColor(tcell.ColorGray)

	// Body: browser | 1-char separator | right pane
	body := tview.NewFlex().
		AddItem(a.contentFlex, 0, 1, true).
		AddItem(vSep, 1, 0, false).
		AddItem(rightPane, queuePaneWidth, 0, false)
	body.SetBackgroundColor(tcell.ColorDefault)

	// Horizontal separator above now-playing bar
	sep := tview.NewTextView().SetWrap(false)
	sep.SetText(strings.Repeat("─", 500))
	sep.SetTextColor(tcell.ColorGray)
	sep.SetBackgroundColor(tcell.ColorDefault)

	// Now-playing bar (2 rows: title+artist on line 1, progress on line 2)
	a.nowBar = tview.NewTextView().SetDynamicColors(true)
	a.nowBar.SetBackgroundColor(tcell.ColorBlack) // ANSI 0 — same subtle bar as tab

	// Key hints bar (hidden by default; toggled with ?)
	a.hintsBar = tview.NewTextView().SetDynamicColors(true)
	a.hintsBar.SetBackgroundColor(tcell.ColorBlack)
	a.hintsBar.SetText(
		"[gray]  j/k:move  Enter:play  a:add  n:insert  c:clear  Space:⏯  " +
			">.:next  <,:prev  +-:vol  r:autodj  v:vis  ←→:page  1-5:tabs  /:search  ⌫:back  q:quit  ?:close[-]")

	// Root layout (vertical flex) — hintsBar shown by default, toggled with ?
	a.showHints = true
	a.rootFlex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.tabBar, 1, 0, false).
		AddItem(a.breadcrumb, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(sep, 1, 0, false).
		AddItem(a.nowBar, 2, 0, false).
		AddItem(a.hintsBar, 1, 0, false)
	a.rootFlex.SetBackgroundColor(tcell.ColorDefault)

	a.tv.SetInputCapture(a.handleKey)

	a.pages.AddPage("main", a.rootFlex, true, true)

	a.tab = tabArtists
	a.updateTabBar()
	a.updateNowBar()
	a.updateQueuePanel()

	// Restore Auto DJ state from config.
	a.autoDJ = a.cfg.AutoDJ

	// Restore the previously saved play queue from the server (async).
	go a.loadPlayQueue()
}

// ---------------------------------------------------------------------------
// Tab fetching (goroutines → QueueUpdateDraw)
// ---------------------------------------------------------------------------

func (a *App) fetchTab(tab int) {
	switch tab {
	case tabArtists:
		a.tv.QueueUpdateDraw(func() { a.restoreListFlex(); a.setLoading() })
		artists, err := a.client.GetArtists()
		a.tv.QueueUpdateDraw(func() {
			if err != nil {
				a.showError(err)
				return
			}
			items := make([]listItem, len(artists))
			for i, ar := range artists {
				items[i] = listItem{label: ar.Name, data: ar}
			}
			a.setItems(items, "")
		})

	case tabAlbums:
		a.tv.QueueUpdateDraw(func() { a.restoreListFlex(); a.setLoading() })
		albums, err := a.client.GetAlbumList(500)
		a.tv.QueueUpdateDraw(func() {
			if err != nil {
				a.showError(err)
				return
			}
			items := make([]listItem, len(albums))
			for i, al := range albums {
				items[i] = listItem{label: dimArtistLabel(al.Name, al.Artist), data: al}
			}
			a.setItems(items, "")
		})

	case tabSongs:
		a.tv.QueueUpdateDraw(func() { a.restoreListFlex(); a.setLoading() })
		songs, err := a.client.GetRandomSongs(50)
		a.tv.QueueUpdateDraw(func() {
			if err != nil {
				a.showError(err)
				return
			}
			items := make([]listItem, len(songs))
			for i, s := range songs {
				items[i] = listItem{label: dimSongLabel(s.Title, s.Artist, 0), data: s}
			}
			a.setItems(items, "Random Songs")
		})

	case tabPlaylists:
		a.tv.QueueUpdateDraw(func() { a.restoreListFlex(); a.setLoading() })
		lists, err := a.client.GetPlaylists()
		a.tv.QueueUpdateDraw(func() {
			if err != nil {
				a.showError(err)
				return
			}
			items := make([]listItem, len(lists))
			for i, pl := range lists {
				items[i] = listItem{label: pl.Name, data: pl}
			}
			a.setItems(items, "")
		})

	case tabSearch:
		a.tv.QueueUpdateDraw(func() {
			a.contentFlex.Clear()
			a.contentFlex.AddItem(a.searchInput, 1, 0, true)
			a.contentFlex.AddItem(a.list, 0, 1, false)
			a.list.Clear()
			a.currentItems = nil
			a.breadcrumb.SetText("[gray]type and press Enter to search[-]")
			a.tv.SetFocus(a.searchInput)
		})
	}
}

func (a *App) fetchSearch(query string) {
	artists, albums, songs, err := a.client.Search(query)
	a.tv.QueueUpdateDraw(func() {
		if err != nil {
			a.showError(err)
			return
		}
		var items []listItem
		if len(artists) > 0 {
			items = append(items, listItem{label: "── Artists ──"})
			for _, ar := range artists {
				items = append(items, listItem{label: ar.Name, data: ar})
			}
		}
		if len(albums) > 0 {
			items = append(items, listItem{label: "── Albums ──"})
			for _, al := range albums {
				items = append(items, listItem{label: dimArtistLabel(al.Name, al.Artist), data: al})
			}
		}
		if len(songs) > 0 {
			items = append(items, listItem{label: "── Songs ──"})
			for _, s := range songs {
				items = append(items, listItem{label: dimSongLabel(s.Title, s.Artist, 0), data: s})
			}
		}
		if len(items) == 0 {
			items = []listItem{{label: "No results"}}
		}
		a.setItems(items, fmt.Sprintf("Results: %q", query))
		a.tv.SetFocus(a.list)
	})
}

// ---------------------------------------------------------------------------
// Selection / navigation
// ---------------------------------------------------------------------------

func (a *App) handleSelect(row int) {
	if row < 0 || row >= len(a.currentItems) {
		return
	}
	item := a.currentItems[row]

	switch data := item.data.(type) {
	case subsonic.Artist:
		a.pushNav(row)
		a.setItems([]listItem{{label: "Loading…"}}, data.Name)
		go func() {
			albums, err := a.client.GetAlbums(data.ID)
			a.tv.QueueUpdateDraw(func() {
				if err != nil {
					a.showError(err)
					return
				}
				items := make([]listItem, len(albums))
				for i, al := range albums {
					label := al.Name
					if al.Year > 0 {
						label = fmt.Sprintf("%s (%d)", al.Name, al.Year)
					}
					items[i] = listItem{label: label, data: al}
				}
				a.setItems(items, data.Name)
			})
		}()

	case subsonic.Album:
		a.pushNav(row)
		a.setItems([]listItem{{label: "Loading…"}}, data.Name)
		go func() {
			songs, err := a.client.GetSongs(data.ID)
			a.tv.QueueUpdateDraw(func() {
				if err != nil {
					a.showError(err)
					return
				}
				items := make([]listItem, len(songs))
				for i, s := range songs {
					items[i] = listItem{label: dimSongLabel(s.Title, s.Artist, s.Duration), data: s}
				}
				a.setItems(items, data.Name)
			})
		}()

	case subsonic.Playlist:
		a.pushNav(row)
		a.setItems([]listItem{{label: "Loading…"}}, data.Name)
		go func() {
			songs, err := a.client.GetPlaylistSongs(data.ID)
			a.tv.QueueUpdateDraw(func() {
				if err != nil {
					a.showError(err)
					return
				}
				items := make([]listItem, len(songs))
				for i, s := range songs {
					items[i] = listItem{label: dimSongLabel(s.Title, s.Artist, s.Duration), data: s}
				}
				a.setItems(items, data.Name)
			})
		}()

	case subsonic.Song:
		// Collect all songs visible in the current list as the context queue.
		// The whole context becomes the new queue, starting from the selected song
		// (with earlier songs accessible via Prev).
		var contextSongs []subsonic.Song
		selIdx := 0
		found := false
		for _, it := range a.allItems {
			if s, ok := it.data.(subsonic.Song); ok {
				if s.ID == data.ID && !found {
					selIdx = len(contextSongs)
					found = true
				}
				contextSongs = append(contextSongs, s)
			}
		}
		if len(contextSongs) == 0 {
			contextSongs = []subsonic.Song{data}
			selIdx = 0
		}
		a.replaceQueueFrom(contextSongs, selIdx)

	default:
		// header row — ignore
	}
}

// replaceQueueFrom clears the current queue, fills it with songs, and begins
// playback at startIndex. All songs are loaded into mpv's playlist so
// Prev/Next navigate the full context.
func (a *App) replaceQueueFrom(songs []subsonic.Song, startIndex int) {
	if len(songs) == 0 {
		return
	}
	if startIndex < 0 || startIndex >= len(songs) {
		startIndex = 0
	}

	// Build stream URLs in order.
	urls := make([]string, len(songs))
	if a.client != nil {
		for i, s := range songs {
			urls[i] = a.client.StreamURL(s.ID)
		}
	}

	a.mu.Lock()
	a.queue = make([]subsonic.Song, len(songs))
	copy(a.queue, songs)
	a.queueIdx = startIndex
	a.mu.Unlock()

	if a.player != nil {
		_ = a.player.LoadPlaylistAt(urls, startIndex)
		_ = a.player.Play()
	}

	a.savePlayQueue()
	a.updateQueuePanel()
}

func (a *App) pushNav(cursorRow int) {
	a.navStack = append(a.navStack, navFrame{
		breadText: a.breadText,
		items:     a.currentItems,
		allItems:  a.allItems,
		pageNum:   a.pageNum,
		index:     cursorRow,
	})
}

func (a *App) popNav() {
	if len(a.navStack) == 0 {
		return
	}
	frame := a.navStack[len(a.navStack)-1]
	a.navStack = a.navStack[:len(a.navStack)-1]
	a.breadText = frame.breadText
	a.allItems = frame.allItems
	a.pageNum = frame.pageNum
	a.currentItems = frame.items
	a.list.Clear()
	for i, it := range frame.items {
		a.list.SetCell(i, 0, tableCell(it.label))
	}
	totalPages := (len(a.allItems) + pageSize - 1) / pageSize
	if totalPages > 1 {
		a.breadcrumb.SetText(fmt.Sprintf("%s  [gray](page %d/%d  ←→)[-]",
			a.breadText, a.pageNum+1, totalPages))
	} else {
		a.breadcrumb.SetText(a.breadText)
	}
	if frame.index >= 0 && frame.index < a.list.GetRowCount() {
		a.list.Select(frame.index, 0)
	}
}

// ---------------------------------------------------------------------------
// Append/insert queue helpers (used by 'a' and 'n' keys)
// ---------------------------------------------------------------------------

func (a *App) enqueue(songs []subsonic.Song, insertNext bool) {
	a.mu.Lock()
	if insertNext && a.queueIdx+1 <= len(a.queue) {
		tail := make([]subsonic.Song, len(a.queue[a.queueIdx+1:]))
		copy(tail, a.queue[a.queueIdx+1:])
		a.queue = append(a.queue[:a.queueIdx+1], append(songs, tail...)...)
	} else {
		a.queue = append(a.queue, songs...)
	}
	allSongs := make([]subsonic.Song, len(a.queue))
	copy(allSongs, a.queue)
	idx := a.queueIdx
	a.mu.Unlock()

	if a.player != nil && a.client != nil {
		if insertNext {
			// Inserting in the middle means mpv's playlist order would be wrong
			// if we just appended. Rebuild the full playlist and restore position.
			st := a.player.State()
			savedPos := st.Position
			urls := make([]string, len(allSongs))
			for i, s := range allSongs {
				urls[i] = a.client.StreamURL(s.ID)
			}
			_ = a.player.LoadPlaylistAt(urls, idx)
			if savedPos > 1 {
				go func() {
					time.Sleep(300 * time.Millisecond)
					_ = a.player.Seek(savedPos)
				}()
			}
		} else {
			// Appending — just load the new songs at the end.
			for _, s := range songs {
				_ = a.player.Load(a.client.StreamURL(s.ID))
			}
		}
	}
	a.savePlayQueue()
}

// enqueueSelected enqueues (or inserts next) whatever is highlighted in the list.
// Songs are queued immediately; Albums and Playlists are fetched then queued.
func (a *App) enqueueSelected(insertNext bool) {
	row, _ := a.list.GetSelection()
	if row < 0 || row >= len(a.currentItems) {
		return
	}
	item := a.currentItems[row]
	switch data := item.data.(type) {
	case subsonic.Song:
		a.enqueue([]subsonic.Song{data}, insertNext)
	case subsonic.Album:
		if a.client == nil {
			return
		}
		go func() {
			songs, err := a.client.GetSongs(data.ID)
			if err == nil && len(songs) > 0 {
				a.tv.QueueUpdateDraw(func() {
					a.enqueue(songs, insertNext)
					a.updateQueuePanel()
				})
			}
		}()
	case subsonic.Playlist:
		if a.client == nil {
			return
		}
		go func() {
			songs, err := a.client.GetPlaylistSongs(data.ID)
			if err == nil && len(songs) > 0 {
				a.tv.QueueUpdateDraw(func() {
					a.enqueue(songs, insertNext)
					a.updateQueuePanel()
				})
			}
		}()
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (a *App) handleKey(event *tcell.EventKey) *tcell.EventKey {
	if a.tv.GetFocus() == a.searchInput {
		return event // let search input handle its own keys
	}

	switch event.Key() {
	case tcell.KeyRune:
		switch event.Rune() {
		case 'c':
			a.clearQueue()
			return nil
		case 'r':
			a.autoDJ = !a.autoDJ
			a.autoDJFetching = false
			a.cfg.AutoDJ = a.autoDJ
			go config.Save(a.cfg)
			a.updateQueuePanel()
			return nil
		case 'v':
			a.visualizerMode = (a.visualizerMode + 1) % 2
			a.updateVisualizerPanel()
			return nil
		case '?':
			if a.showHints {
				a.rootFlex.RemoveItem(a.hintsBar)
				a.showHints = false
			} else {
				a.rootFlex.AddItem(a.hintsBar, 1, 0, false)
				a.showHints = true
			}
			return nil
		case 'q':
			a.cleanup()
			a.tv.Stop()
			return nil
		case ' ':
			if a.player != nil {
				st := a.player.State()
				if st.Playing {
					_ = a.player.Pause()
				} else {
					_ = a.player.Play()
				}
			}
			return nil
		case '.', '>':
			if a.player != nil {
				_ = a.player.Next()
			}
			return nil
		case ',', '<':
			if a.player != nil {
				_ = a.player.Prev()
			}
			return nil
		case '+', '=':
			if a.player != nil {
				st := a.player.State()
				_ = a.player.SetVolume(clamp(st.Volume+5, 0, 100))
			}
			return nil
		case '-':
			if a.player != nil {
				st := a.player.State()
				_ = a.player.SetVolume(clamp(st.Volume-5, 0, 100))
			}
			return nil
		case 'a':
			a.enqueueSelected(false)
			return nil
		case 'n':
			a.enqueueSelected(true)
			return nil
		case '/':
			a.switchTab(tabSearch)
			return nil
		case '1':
			a.switchTab(tabArtists)
			return nil
		case '2':
			a.switchTab(tabAlbums)
			return nil
		case '3':
			a.switchTab(tabSongs)
			return nil
		case '4':
			a.switchTab(tabPlaylists)
			return nil
		case '5':
			a.switchTab(tabSearch)
			return nil
		case 'j':
			if a.tv.GetFocus() == a.list {
				row, _ := a.list.GetSelection()
				if row < a.list.GetRowCount()-1 {
					a.list.Select(row+1, 0)
				}
				return nil
			}
		case 'k':
			if a.tv.GetFocus() == a.list {
				row, _ := a.list.GetSelection()
				if row > 0 {
					a.list.Select(row-1, 0)
				}
				return nil
			}
		}

	case tcell.KeyLeft:
		if a.tv.GetFocus() != a.searchInput && a.pageNum > 0 {
			a.pageNum--
			a.applyPage()
			return nil
		}

	case tcell.KeyRight:
		if a.tv.GetFocus() != a.searchInput {
			totalPages := (len(a.allItems) + pageSize - 1) / pageSize
			if a.pageNum < totalPages-1 {
				a.pageNum++
				a.applyPage()
				return nil
			}
		}

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(a.navStack) > 0 {
			a.popNav()
			return nil
		}

	case tcell.KeyCtrlC:
		a.cleanup()
		a.tv.Stop()
		return nil
	}

	return event
}

func (a *App) switchTab(tab int) {
	a.tab = tab
	a.navStack = nil
	a.breadText = ""
	if tab == tabSearch && a.searchInput != nil {
		a.searchInput.SetText("")
	}
	a.updateTabBar()
	go a.fetchTab(tab)
}

// ---------------------------------------------------------------------------
// Hotkey commands
// ---------------------------------------------------------------------------

func (a *App) handleHotkeyCmd(cmd hotkey.Command) {
	if a.player == nil {
		return
	}
	switch cmd {
	case hotkey.CmdPlay:
		_ = a.player.Play()
	case hotkey.CmdPause:
		_ = a.player.Pause()
	case hotkey.CmdTogglePlayPause:
		st := a.player.State()
		if st.Playing {
			_ = a.player.Pause()
		} else {
			_ = a.player.Play()
		}
	case hotkey.CmdNext:
		_ = a.player.Next()
	case hotkey.CmdPrev:
		_ = a.player.Prev()
	case hotkey.CmdVolumeUp:
		st := a.player.State()
		_ = a.player.SetVolume(clamp(st.Volume+5, 0, 100))
	case hotkey.CmdVolumeDown:
		st := a.player.State()
		_ = a.player.SetVolume(clamp(st.Volume-5, 0, 100))
	}
}

// ---------------------------------------------------------------------------
// Ticker — refreshes UI every 500 ms
// ---------------------------------------------------------------------------

func (a *App) ticker() {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		a.tv.QueueUpdateDraw(func() {
			// Sync queueIdx from mpv's playlist position.
			if a.player != nil {
				pos := a.player.State().PlaylistPos
				if pos >= 0 {
					a.mu.Lock()
					if pos < len(a.queue) {
						a.queueIdx = pos
					}
					a.mu.Unlock()
				}
			}
			// Update beat interval from current song's BPM.
			a.mu.Lock()
			var bpm int
			var nowSong subsonic.Song
			hasSong := a.queueIdx >= 0 && a.queueIdx < len(a.queue)
			if hasSong {
				nowSong = a.queue[a.queueIdx]
				bpm = nowSong.BPM
			}
			a.mu.Unlock()
			// Prefer BPM read directly from the file's tags by mpv (most accurate),
			// fall back to Subsonic metadata, then to 120 BPM default.
			if a.player != nil {
				if mpvBPM := a.player.State().BPM; mpvBPM > 0 {
					bpm = mpvBPM
				}
			}
			a.currentBPM = bpm // 0 = no tag found; shown as "---" in visualizer
			if bpm > 0 {
				a.beatInterval = time.Duration(60000/bpm) * time.Millisecond
			} else {
				a.beatInterval = 500 * time.Millisecond // fallback ~120 BPM
			}
			// Detect song change, update window title, and fetch lyrics.
			if hasSong && nowSong.ID != a.currentSongID {
				a.currentSongID = nowSong.ID
				if nowSong.Artist != "" {
					setWindowTitle(nowSong.Title + " — " + nowSong.Artist)
				} else {
					setWindowTitle(nowSong.Title)
				}
				a.lyricsLines = nil
				a.lyricsSynced = false
				if a.client != nil {
					go func(id, artist, title, album string, duration int) {
						lines, synced, err := a.client.GetLyricsBySongId(id)
						if err != nil || len(lines) == 0 {
							plain, err2 := a.client.GetLyrics(artist, title)
							if err2 == nil && len(plain) > 0 {
								lines = plain
								synced = false
							}
						}
						// Third fallback: lrclib.net
						if len(lines) == 0 {
							lrcLines, lrcSynced, _ := subsonic.GetLyricsFromLrcLib(artist, title, album, duration)
							if len(lrcLines) > 0 {
								lines = lrcLines
								synced = lrcSynced
							}
						}
						a.tv.QueueUpdateDraw(func() {
							if a.currentSongID == id {
								a.lyricsLines = lines
								a.lyricsSynced = synced
							}
						})
					}(nowSong.ID, nowSong.Artist, nowSong.Title, nowSong.Album, nowSong.Duration)

					// Detect BPM from the audio stream via aubiotempo (optional).
					// Runs in the background; result is discarded if the song changed.
					if a.client != nil {
						streamURL := a.client.StreamURL(nowSong.ID)
						go func(id, url string) {
							detected := bpmdetect.Detect(context.Background(), url)
							if detected == 0 {
								return
							}
							appLogf("BPM callback fired: detected=%d songID=%q currentSongID=%q",
								detected, id, a.currentSongID)
							a.tv.QueueUpdateDraw(func() {
								appLogf("QueueUpdateDraw: id=%q currentSongID=%q match=%v",
									id, a.currentSongID, a.currentSongID == id)
								if a.currentSongID != id {
									return
								}
								a.currentBPM = detected
								a.beatInterval = time.Duration(60000/detected) * time.Millisecond
								a.updateVisualizerPanel()
								appLogf("BPM set to %d, visualizer updated", a.currentBPM)
							})
						}(nowSong.ID, streamURL)
					}
				}
			}
			// Auto DJ: when 2 or fewer songs remain, fetch more.
			if a.autoDJ && !a.autoDJFetching && a.client != nil {
				a.mu.Lock()
				qLen := len(a.queue)
				idx := a.queueIdx
				var seedID string
				if qLen > 0 {
					seedID = a.queue[qLen-1].ID
				}
				a.mu.Unlock()
				if qLen > 0 && idx >= qLen-2 {
					a.autoDJFetching = true
					go func(id string) {
						songs, err := a.client.GetSimilarSongs(id, 20)
						source := "similar"
						if err != nil || len(songs) == 0 {
							songs, _ = a.client.GetRandomSongs(20)
							source = "random"
						}
						a.tv.QueueUpdateDraw(func() {
							defer func() { a.autoDJFetching = false }()
							a.autoDJSource = source
							if len(songs) == 0 {
								return
							}
							// Filter out songs already in the queue to avoid repeats.
							a.mu.Lock()
							inQueue := make(map[string]bool, len(a.queue))
							for _, s := range a.queue {
								inQueue[s.ID] = true
							}
							a.mu.Unlock()
							var fresh []subsonic.Song
							for _, s := range songs {
								if !inQueue[s.ID] {
									fresh = append(fresh, s)
								}
							}
							if len(fresh) > 0 {
								a.enqueue(fresh, false)
								a.updateQueuePanel()
							}
						})
					}(seedID)
				}
			}
			a.updateNowBar()
			a.updateQueuePanel()
			a.updateLyricsPanel()
			if a.player != nil {
				st := a.player.State()
				hotkey.UpdateNowPlaying(st.Title, "", st.Duration, st.Position, st.Playing)
			}
			// Save play queue to server every ~10 seconds to keep position in sync.
			a.saveTickCount++
			if a.saveTickCount >= 20 {
				a.saveTickCount = 0
				a.savePlayQueue()
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UI helpers
// ---------------------------------------------------------------------------

func (a *App) updateTabBar() {
	var sb strings.Builder
	for i, name := range tabNames {
		if i == a.tab {
			// Active tab: bold (terminal-native — no hex color)
			fmt.Fprintf(&sb, " [::b]%d:%s[::-] ", i+1, name)
		} else {
			fmt.Fprintf(&sb, " [gray]%d:%s[-] ", i+1, name)
		}
	}
	a.tabBar.SetText(sb.String())
}

// updateNowBar renders a 2-line now-playing bar:
//
//	Line 1:  ▶  Song Title  [gray]— Artist[-]
//	Line 2:     ████████░░░░░░░░░░░░   1:23 / 4:56   vol 80%
func (a *App) updateNowBar() {
	if a.nowBar == nil {
		return
	}

	var pos, dur float64
	var mpvTitle string
	vol := 100
	playing := false
	if a.player != nil {
		st := a.player.State()
		pos, dur = st.Position, st.Duration
		mpvTitle, vol, playing = st.Title, st.Volume, st.Playing
	}

	icon := "⏸"
	if playing {
		icon = "▶"
	}

	// Line 1: icon + title + artist (from queue metadata if available)
	var line1 string
	a.mu.Lock()
	var song *subsonic.Song
	if a.queueIdx >= 0 && a.queueIdx < len(a.queue) {
		s := a.queue[a.queueIdx]
		song = &s
	}
	a.mu.Unlock()

	if song != nil {
		title := song.Title
		if title == "" {
			title = mpvTitle
		}
		if song.Artist != "" {
			line1 = fmt.Sprintf(" %s  %s  [gray]— %s[-]", icon, title, song.Artist)
		} else {
			line1 = fmt.Sprintf(" %s  %s", icon, title)
		}
	} else {
		title := mpvTitle
		if title == "" {
			title = "─"
		}
		line1 = fmt.Sprintf(" %s  %s", icon, title)
	}

	// Line 2: progress bar + time + volume
	// Reserve space for the fixed parts: 4 leading spaces + 3 separator + ~28 for time/vol.
	_, _, nowBarWidth, _ := a.nowBar.GetRect()
	barWidth := nowBarWidth - 35
	if barWidth < 10 {
		barWidth = 10
	}
	bar := progressBar(pos, dur, barWidth)
	line2 := fmt.Sprintf("    %s   [gray]%s / %s  vol %d%%[-]",
		bar, fmtDur(int(pos)), fmtDur(int(dur)), vol)

	a.nowBar.SetText(line1 + "\n" + line2)
}

// updateQueuePanel renders the right-side queue list with a ▶ marker on the
// current track, gray for past tracks, and default colour for upcoming ones.
func (a *App) updateQueuePanel() {
	if a.queuePanel == nil {
		return
	}

	a.mu.Lock()
	queue := make([]subsonic.Song, len(a.queue))
	copy(queue, a.queue)
	idx := a.queueIdx
	a.mu.Unlock()

	var sb strings.Builder
	if a.autoDJ {
		src := a.autoDJSource
		if src == "" {
			src = "…"
		}
		sb.WriteString(fmt.Sprintf("[green]── QUEUE  DJ:%s ──[-]\n", src))
	} else {
		sb.WriteString("[gray]── QUEUE ──[-]\n")
	}

	if len(queue) == 0 {
		sb.WriteString("[gray]  (empty)[-]")
	} else {
		for i, s := range queue {
			title := truncQ(s.Title, queuePaneWidth-3)
			switch {
			case i < idx:
				fmt.Fprintf(&sb, "[gray]  %s[-]\n", title)
			case i == idx:
				fmt.Fprintf(&sb, "[::b]▶ %s[::-]\n", title)
			default:
				fmt.Fprintf(&sb, "  %s\n", title)
			}
		}
	}

	a.queuePanel.SetText(sb.String())

	// Advance bongo cat on each beat; freeze at idle when paused.
	if a.player != nil && a.player.State().Playing {
		now := time.Now()
		interval := a.beatInterval
		if interval <= 0 {
			interval = 500 * time.Millisecond
		}
		if a.lastBeat.IsZero() || now.Sub(a.lastBeat) >= interval {
			a.catPhase++
			if a.lastBeat.IsZero() {
				a.lastBeat = now
			} else {
				a.lastBeat = a.lastBeat.Add(interval)
			}
		}
	} else {
		a.catPhase = 0
		a.lastBeat = time.Time{}
	}
	a.updateVisualizerPanel()
}

// updateLyricsPanel renders up to 3 lines of the current song's lyrics,
// centered on the line matching the current playback position.
func (a *App) updateLyricsPanel() {
	if a.lyricsPanel == nil {
		return
	}
	lines := a.lyricsLines
	if len(lines) == 0 {
		a.lyricsPanel.SetText("")
		return
	}
	var pos, dur float64
	if a.player != nil {
		st := a.player.State()
		pos, dur = st.Position, st.Duration
	}
	idx := 0
	if a.lyricsSynced {
		posMs := int(pos * 1000)
		for i, l := range lines {
			if l.TimeMs <= posMs {
				idx = i
			} else {
				break
			}
		}
	} else if dur > 0 {
		idx = int(float64(len(lines)) * pos / dur)
		if idx >= len(lines) {
			idx = len(lines) - 1
		}
	}
	var sb strings.Builder
	if idx > 0 {
		sb.WriteString("[gray]" + lines[idx-1].Text + "[-]")
	}
	sb.WriteString("\n")
	sb.WriteString("[::b]" + lines[idx].Text + "[::-]")
	sb.WriteString("\n")
	if idx+1 < len(lines) {
		sb.WriteString("[gray]" + lines[idx+1].Text + "[-]")
	}
	a.lyricsPanel.SetText(sb.String())
}

// updateVisualizerPanel renders the current visualizer frame (DJ dancer or bars).
func (a *App) updateVisualizerPanel() {
	if a.catPanel == nil {
		return
	}
	switch a.visualizerMode {
	case 0: // DJ dancer
		playing := a.player != nil && a.player.State().Playing
		bpm := a.currentBPM // 0 = unknown → djFrame shows "---"
		if !playing {
			a.catPanel.SetText(djFrame(-1, bpm))
		} else if a.catPhase%2 == 0 {
			a.catPanel.SetText(djFrame(0, bpm))
		} else {
			a.catPanel.SetText(djFrame(1, bpm))
		}
	case 1: // vertical bars — sin-wave fake spectrum, beat-driven
		t := float64(a.catPhase) * 0.5
		var sb strings.Builder
		for row := 0; row < catPanelHeight; row++ {
			level := catPanelHeight - row // 7 at top row, 1 at bottom row
			for bar := 0; bar < numBars; bar++ {
				p := t + float64(bar)*0.6
				h := math.Sin(p)*0.4 + math.Sin(p*1.9)*0.3 + math.Sin(p*3.1)*0.2
				normalized := (h + 0.9) / 1.8 // map -0.9…0.9 → 0…1
				height := 1 + int(normalized*float64(catPanelHeight-1))
				if height < 1 {
					height = 1
				}
				if height > catPanelHeight {
					height = catPanelHeight
				}
				if height >= level {
					sb.WriteString("██")
				} else {
					sb.WriteString("  ")
				}
			}
			if row < catPanelHeight-1 {
				sb.WriteString("\n")
			}
		}
		a.catPanel.SetText("[green]" + sb.String() + "[-]")
	}
}

func (a *App) setItems(items []listItem, bread string) {
	a.allItems = items
	a.pageNum = 0
	if bread != "" {
		a.breadText = bread
	}
	a.applyPage()
}

// applyPage slices allItems for the current page and refreshes the list and breadcrumb.
func (a *App) applyPage() {
	start := a.pageNum * pageSize
	end := start + pageSize
	if start > len(a.allItems) {
		start = len(a.allItems)
	}
	if end > len(a.allItems) {
		end = len(a.allItems)
	}
	a.currentItems = a.allItems[start:end]

	a.list.Clear()
	for i, it := range a.currentItems {
		a.list.SetCell(i, 0, tableCell(it.label))
	}
	if a.list.GetRowCount() > 0 {
		a.list.Select(0, 0)
		a.list.ScrollToBeginning()
	}

	totalPages := (len(a.allItems) + pageSize - 1) / pageSize
	if totalPages > 1 {
		a.breadcrumb.SetText(fmt.Sprintf("%s  [gray](page %d/%d  ←→)[-]",
			a.breadText, a.pageNum+1, totalPages))
	} else {
		a.breadcrumb.SetText(a.breadText)
	}
}

func (a *App) setLoading() {
	a.list.Clear()
	a.list.SetCell(0, 0, tview.NewTableCell("Loading…").
		SetTextColor(tcell.ColorGray).
		SetExpansion(1))
	a.currentItems = nil
}

func (a *App) showError(err error) {
	a.list.Clear()
	a.list.SetCell(0, 0, tview.NewTableCell(fmt.Sprintf("Error: %v", err)).
		SetTextColor(tcell.ColorMaroon). // ANSI 1
		SetExpansion(1))
	a.currentItems = nil
}

func (a *App) restoreListFlex() {
	a.contentFlex.Clear()
	a.contentFlex.AddItem(a.list, 0, 1, true)
	a.tv.SetFocus(a.list)
}

// ---------------------------------------------------------------------------
// Play queue persistence
// ---------------------------------------------------------------------------

// savePlayQueue snapshots the current queue and fires off a background save to
// the Navidrome server. Safe to call from any goroutine.
func (a *App) savePlayQueue() {
	if a.client == nil {
		return
	}
	a.mu.Lock()
	songs := make([]subsonic.Song, len(a.queue))
	copy(songs, a.queue)
	idx := a.queueIdx
	a.mu.Unlock()

	ids := make([]string, len(songs))
	for i, s := range songs {
		ids[i] = s.ID
	}
	var currentID string
	var posMs int64
	if idx >= 0 && idx < len(songs) {
		currentID = songs[idx].ID
	}
	if a.player != nil {
		posMs = int64(a.player.State().Position * 1000)
	}
	go func() {
		_ = a.client.SavePlayQueue(ids, currentID, posMs)
	}()
}

// loadPlayQueue fetches the saved play queue from the server and restores it.
// Called in a goroutine from buildMainPage. Starts playback paused at the
// saved position so the user can resume at will.
func (a *App) loadPlayQueue() {
	if a.client == nil {
		return
	}
	pq, err := a.client.GetPlayQueue()
	if err != nil || pq == nil {
		return
	}

	startIdx := 0
	for i, s := range pq.Entries {
		if s.ID == pq.CurrentID {
			startIdx = i
			break
		}
	}
	urls := make([]string, len(pq.Entries))
	for i, s := range pq.Entries {
		urls[i] = a.client.StreamURL(s.ID)
	}
	posMs := pq.PositionMs
	songs := pq.Entries

	a.tv.QueueUpdateDraw(func() {
		a.mu.Lock()
		a.queue = songs
		a.queueIdx = startIdx
		a.mu.Unlock()

		if a.player != nil {
			_ = a.player.LoadPlaylistAt(urls, startIdx)
			// Keep paused at the saved position so the user presses Space to resume.
			go func() {
				time.Sleep(400 * time.Millisecond)
				_ = a.player.Pause()
				if posMs > 0 {
					_ = a.player.Seek(float64(posMs) / 1000.0)
				}
			}()
		}
		a.updateQueuePanel()
		a.updateNowBar()
	})
}

// clearQueue stops playback and empties the queue, then saves the empty state.
func (a *App) clearQueue() {
	a.mu.Lock()
	a.queue = nil
	a.queueIdx = -1
	a.mu.Unlock()

	if a.player != nil {
		_ = a.player.ClearPlaylist()
		_ = a.player.Pause()
	}
	a.currentSongID = ""
	a.lyricsLines = nil
	a.autoDJ = false
	a.autoDJFetching = false
	a.autoDJSource = ""
	setWindowTitle("kagura")
	a.savePlayQueue()
	a.updateQueuePanel()
	a.updateNowBar()
	a.updateLyricsPanel()
	a.updateVisualizerPanel()
}

// ---------------------------------------------------------------------------

func (a *App) cleanup() {
	// Save final queue position synchronously before shutting down.
	if a.client != nil {
		a.mu.Lock()
		songs := make([]subsonic.Song, len(a.queue))
		copy(songs, a.queue)
		idx := a.queueIdx
		a.mu.Unlock()
		ids := make([]string, len(songs))
		for i, s := range songs {
			ids[i] = s.ID
		}
		var currentID string
		var posMs int64
		if idx >= 0 && idx < len(songs) {
			currentID = songs[idx].ID
		}
		if a.player != nil {
			posMs = int64(a.player.State().Position * 1000)
		}
		_ = a.client.SavePlayQueue(ids, currentID, posMs)
	}
	setWindowTitle("kagura")
	if a.player != nil {
		a.player.Close()
	}
	if a.hkDaemon != nil {
		a.hkDaemon.Stop()
	}
}

// dimSongLabel builds "Title  [gray]Artist  dur[-]" for browser lists.
// The title stays at default (bright) colour; artist and duration are dimmed.
// Either artist or duration may be absent.
func dimSongLabel(title, artist string, durSecs int) string {
	label := title
	if artist == "" && durSecs <= 0 {
		return label
	}
	label += "  [gray]"
	if artist != "" {
		label += artist
		if durSecs > 0 {
			label += "  " + fmtDur(durSecs)
		}
	} else {
		label += fmtDur(durSecs)
	}
	return label + "[-]"
}

// dimArtistLabel builds "Name  [gray]Artist[-]" for album/artist lists.
func dimArtistLabel(name, artist string) string {
	if artist == "" {
		return name
	}
	return name + "  [gray]" + artist + "[-]"
}

// tableCell returns a standard table cell for browser items.
func tableCell(text string) *tview.TableCell {
	return tview.NewTableCell(text).
		SetTextColor(tcell.ColorDefault).
		SetBackgroundColor(tcell.ColorDefault).
		SetExpansion(1)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// setWindowTitle sets the terminal window/tab title using an OSC escape sequence.
func setWindowTitle(title string) {
	fmt.Fprintf(os.Stdout, "\033]0;%s\007", title)
}

// appLogf appends a timestamped line to /tmp/kagura.log (shared with bpm package).
func appLogf(format string, args ...any) {
	f, err := os.OpenFile("/tmp/kagura.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(f, "[app %s] "+format+"\n", append([]any{ts}, args...)...)
}

func progressBar(pos, dur float64, width int) string {
	if dur <= 0 {
		return strings.Repeat("─", width)
	}
	pct := pos / dur
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func fmtDur(secs int) string {
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// truncQ truncates a string to max runes, appending "…" if needed.
func truncQ(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
