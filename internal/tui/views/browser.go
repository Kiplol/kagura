//go:build ignore

package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kip/kagura/internal/subsonic"
)

// ---------------------------------------------------------------------------
// Messages produced by BrowserModel (handled by the root app model)
// ---------------------------------------------------------------------------

// EnqueueMsg asks the root model to add songs to the playback queue.
type EnqueueMsg struct {
	Songs      []subsonic.Song
	InsertNext bool
}

// TabChangedMsg fires whenever the user switches tabs.
// The root model uses this to trigger the appropriate API call.
type TabChangedMsg struct{ Tab BrowserTab }

// SearchSubmittedMsg fires when the user submits a search query.
type SearchSubmittedMsg struct{ Query string }

// DrillArtistMsg asks the root model to load albums for an artist.
type DrillArtistMsg struct{ Artist subsonic.Artist }

// DrillAlbumMsg asks the root model to load songs for an album.
type DrillAlbumMsg struct{ Album subsonic.Album }

// DrillPlaylistMsg asks the root model to load songs for a playlist.
type DrillPlaylistMsg struct{ Playlist subsonic.Playlist }

// ---------------------------------------------------------------------------
// Messages consumed by BrowserModel (sent by the root app model)
// ---------------------------------------------------------------------------

// ArtistsLoadedMsg delivers artist data to the browser.
type ArtistsLoadedMsg []subsonic.Artist

// AlbumsLoadedMsg delivers albums for a drilled-in artist.
type AlbumsLoadedMsg struct {
	Artist subsonic.Artist
	Albums []subsonic.Album
}

// SongsLoadedMsg delivers tracks for a drilled-in album.
type SongsLoadedMsg struct {
	Album subsonic.Album
	Songs []subsonic.Song
}

// PlaylistsLoadedMsg delivers the playlist index.
type PlaylistsLoadedMsg []subsonic.Playlist

// PlaylistSongsLoadedMsg delivers tracks for a playlist.
type PlaylistSongsLoadedMsg struct {
	Playlist subsonic.Playlist
	Songs    []subsonic.Song
}

// SearchResultsMsg delivers search results across all categories.
type SearchResultsMsg struct {
	Artists []subsonic.Artist
	Albums  []subsonic.Album
	Songs   []subsonic.Song
}

// LoadErrMsg is sent when an API call fails.
type LoadErrMsg struct{ Err error }

// ---------------------------------------------------------------------------
// BrowserTab
// ---------------------------------------------------------------------------

// BrowserTab identifies which tab is active.
type BrowserTab int

const (
	TabArtists BrowserTab = iota
	TabAlbums
	TabSongs
	TabPlaylists
	TabSearch
	tabCount
)

var tabNames = [tabCount]string{"Artists", "Albums", "Songs", "Playlists", "Search"}

// ---------------------------------------------------------------------------
// Internal list item
// ---------------------------------------------------------------------------

type listItem struct {
	title string
	desc  string
	data  any
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.desc }
func (i listItem) FilterValue() string { return i.title }

// ---------------------------------------------------------------------------
// BrowserModel
// ---------------------------------------------------------------------------

// BrowserModel is the library browser panel.
type BrowserModel struct {
	tab       BrowserTab
	list      list.Model
	search    textinput.Model
	searching bool
	loading   bool

	artists   []subsonic.Artist
	albums    []subsonic.Album
	songs     []subsonic.Song
	playlists []subsonic.Playlist

	selectedArtist   *subsonic.Artist
	selectedAlbum    *subsonic.Album
	selectedPlaylist *subsonic.Playlist

	width  int
	height int
}

// browserChrome is the number of rows consumed by tab bar + breadcrumb + help bar.
const browserChrome = 3

// listHeight returns the number of rows available for list content.
func listHeight(totalHeight int) int {
	h := totalHeight - browserChrome
	if h < 2 {
		h = 2
	}
	return h
}

// NewBrowser creates an empty BrowserModel.
func NewBrowser(width, height int) BrowserModel {
	// We keep the bubbles list only for cursor/selection state tracking.
	// Rendering is handled by our own renderItems() so we have exact control
	// over line count and can guarantee consistent frame height.
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(nil, delegate, width, listHeight(height))
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	search := textinput.New()
	search.Placeholder = "Type to search…"
	search.CharLimit = 128

	return BrowserModel{list: l, search: search, width: width, height: height, loading: true}
}

func (m BrowserModel) Init() tea.Cmd { return nil }

func (m BrowserModel) Update(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(m.width, listHeight(m.height))

	// --- Data delivered by root model ---
	case ArtistsLoadedMsg:
		m.loading = false
		m.artists = msg
		m.populateArtists()
	case AlbumsLoadedMsg:
		m.loading = false
		m.selectedArtist = &msg.Artist
		m.albums = msg.Albums
		m.populateAlbums()
	case SongsLoadedMsg:
		m.loading = false
		m.selectedAlbum = &msg.Album
		m.songs = msg.Songs
		m.populateSongs()
	case PlaylistsLoadedMsg:
		m.loading = false
		m.playlists = msg
		m.populatePlaylists()
	case PlaylistSongsLoadedMsg:
		m.loading = false
		m.selectedPlaylist = &msg.Playlist
		m.songs = msg.Songs
		m.populateSongs()
	case SearchResultsMsg:
		m.loading = false
		m.populateSearchResults(msg)
	case LoadErrMsg:
		m.loading = false

	// --- Keyboard ---
	case tea.KeyMsg:
		if m.searching {
			return m.handleSearchInput(msg)
		}
		switch msg.String() {
		case "1", "2", "3", "4", "5":
			idx := BrowserTab(msg.String()[0] - '1')
			if idx == m.tab {
				break
			}
			m.tab = idx
			m.selectedArtist = nil
			m.selectedAlbum = nil
			m.selectedPlaylist = nil
			m.loading = true
			m.list.SetItems(nil)
			return m, func() tea.Msg { return TabChangedMsg{Tab: idx} }
		case "/":
			m.tab = TabSearch
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink
		case "enter":
			return m.handleEnter()
		case "a":
			return m.handleEnqueue(false)
		case "n":
			return m.handleEnqueue(true)
		case "backspace":
			return m.handleBack()
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m BrowserModel) handleSearchInput(msg tea.KeyMsg) (BrowserModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		q := m.search.Value()
		m.searching = false
		m.search.Blur()
		m.loading = true
		return m, func() tea.Msg { return SearchSubmittedMsg{Query: q} }
	case "esc":
		m.searching = false
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

func (m BrowserModel) handleEnter() (BrowserModel, tea.Cmd) {
	selected, ok := m.list.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}
	switch v := selected.data.(type) {
	case subsonic.Song:
		return m, func() tea.Msg {
			return EnqueueMsg{Songs: []subsonic.Song{v}}
		}
	case subsonic.Artist:
		m.loading = true
		m.list.SetItems(nil)
		return m, func() tea.Msg { return DrillArtistMsg{Artist: v} }
	case subsonic.Album:
		m.loading = true
		m.list.SetItems(nil)
		return m, func() tea.Msg { return DrillAlbumMsg{Album: v} }
	case subsonic.Playlist:
		m.loading = true
		m.list.SetItems(nil)
		return m, func() tea.Msg { return DrillPlaylistMsg{Playlist: v} }
	}
	return m, nil
}

func (m BrowserModel) handleEnqueue(insertNext bool) (BrowserModel, tea.Cmd) {
	var songs []subsonic.Song
	for _, item := range m.list.Items() {
		if li, ok := item.(listItem); ok {
			if s, ok := li.data.(subsonic.Song); ok {
				songs = append(songs, s)
			}
		}
	}
	if len(songs) == 0 {
		return m, nil
	}
	return m, func() tea.Msg { return EnqueueMsg{Songs: songs, InsertNext: insertNext} }
}

func (m BrowserModel) handleBack() (BrowserModel, tea.Cmd) {
	if m.selectedAlbum != nil {
		m.selectedAlbum = nil
		m.populateAlbums()
	} else if m.selectedPlaylist != nil {
		m.selectedPlaylist = nil
		m.populatePlaylists()
	} else if m.selectedArtist != nil {
		m.selectedArtist = nil
		m.populateArtists()
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// List population helpers
// ---------------------------------------------------------------------------

func (m *BrowserModel) populateArtists() {
	items := make([]list.Item, len(m.artists))
	for i, a := range m.artists {
		items[i] = listItem{
			title: a.Name,
			desc:  fmt.Sprintf("%d albums", a.AlbumCount),
			data:  a,
		}
	}
	m.list.SetItems(items)
	m.list.ResetSelected()
}

func (m *BrowserModel) populateAlbums() {
	items := make([]list.Item, len(m.albums))
	for i, a := range m.albums {
		year := ""
		if a.Year > 0 {
			year = fmt.Sprintf(" (%d)", a.Year)
		}
		items[i] = listItem{
			title: a.Name + year,
			desc:  fmt.Sprintf("%d tracks", a.SongCount),
			data:  a,
		}
	}
	m.list.SetItems(items)
	m.list.ResetSelected()
}

func (m *BrowserModel) populateSongs() {
	items := make([]list.Item, len(m.songs))
	for i, s := range m.songs {
		track := ""
		if s.Track > 0 {
			track = fmt.Sprintf("%02d. ", s.Track)
		}
		items[i] = listItem{
			title: track + s.Title,
			desc:  formatSongDesc(s),
			data:  s,
		}
	}
	m.list.SetItems(items)
	m.list.ResetSelected()
}

func (m *BrowserModel) populatePlaylists() {
	items := make([]list.Item, len(m.playlists))
	for i, p := range m.playlists {
		items[i] = listItem{
			title: p.Name,
			desc:  fmt.Sprintf("%d tracks • %s", p.SongCount, fmtDur(p.Duration)),
			data:  p,
		}
	}
	m.list.SetItems(items)
	m.list.ResetSelected()
}

func (m *BrowserModel) populateSearchResults(r SearchResultsMsg) {
	var items []list.Item
	for _, a := range r.Artists {
		items = append(items, listItem{title: a.Name, desc: "Artist", data: a})
	}
	for _, a := range r.Albums {
		items = append(items, listItem{title: a.Name, desc: "Album — " + a.Artist, data: a})
	}
	for _, s := range r.Songs {
		items = append(items, listItem{title: s.Title, desc: s.Artist + " — " + s.Album, data: s})
	}
	m.list.SetItems(items)
	m.list.ResetSelected()
}

func formatSongDesc(s subsonic.Song) string {
	parts := []string{fmtDur(s.Duration)}
	if s.BitRate > 0 {
		parts = append(parts, fmt.Sprintf("%d kbps", s.BitRate))
	}
	if s.Suffix != "" {
		parts = append(parts, strings.ToUpper(s.Suffix))
	}
	return strings.Join(parts, " • ")
}

func fmtDur(secs int) string {
	m := secs / 60
	s := secs % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

var (
	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).
			Foreground(lipgloss.Color("205")).Underline(true)
	inactiveTabStyle = lipgloss.NewStyle().Padding(0, 1).
				Foreground(lipgloss.Color("241"))
	breadcrumbStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1)
	helpStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	loadingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Italic(true)

	cursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	descStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// renderItems renders exactly `height` lines of the item list.
// It never calls m.list.View(), so frame height is always constant.
func (m BrowserModel) renderItems(height int) string {
	items := m.list.Items()
	cursor := m.list.Index()

	// Build a window of items centred on the cursor so the selected item
	// is always visible even in long lists.
	// Each item takes 2 lines (title + desc), so capacity = height/2.
	lineCapacity := height
	itemCapacity := lineCapacity / 2
	if itemCapacity < 1 {
		itemCapacity = 1
	}

	start := cursor - itemCapacity/2
	if start < 0 {
		start = 0
	}
	end := start + itemCapacity
	if end > len(items) {
		end = len(items)
		start = end - itemCapacity
		if start < 0 {
			start = 0
		}
	}

	var lines []string
	for i := start; i < end; i++ {
		li, ok := items[i].(listItem)
		if !ok {
			continue
		}
		if i == cursor {
			lines = append(lines, cursorStyle.Render("▶ "+li.title))
			lines = append(lines, descStyle.Render("  "+li.desc))
		} else {
			lines = append(lines, normalStyle.Render("  "+li.title))
			lines = append(lines, descStyle.Render("  "+li.desc))
		}
	}

	// Pad to exactly `height` lines so frame height never varies.
	for len(lines) < height {
		lines = append(lines, "")
	}
	// Crop if somehow over (shouldn't happen but be safe).
	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

func (m BrowserModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// chromeRows consumed by this view:
	//   tab bar (1) + breadcrumb/search (1) + help bar (1) = 3
	const chrome = 3
	listH := m.height - chrome
	if listH < 1 {
		listH = 1
	}

	// Row 1: tab bar
	var tabs []string
	for i := BrowserTab(0); i < tabCount; i++ {
		label := fmt.Sprintf("%d:%s", i+1, tabNames[i])
		if i == m.tab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := strings.Join(tabs, "")

	// Row 2: breadcrumb OR search input — always exactly one row.
	var row2 string
	if m.tab == TabSearch {
		row2 = "  " + m.search.View()
	} else {
		row2 = m.breadcrumb()
	}

	// Middle: fixed-height item list (or loading/hint).
	var contentLines string
	if m.loading {
		// Fill with loading message + blank lines to hit listH.
		first := loadingStyle.Render("  Loading…")
		rest := make([]string, listH-1)
		contentLines = strings.Join(append([]string{first}, rest...), "\n")
	} else if m.tab == TabSearch && len(m.list.Items()) == 0 && !m.searching {
		first := helpStyle.Render("  Press / or Enter to search")
		rest := make([]string, listH-1)
		contentLines = strings.Join(append([]string{first}, rest...), "\n")
	} else {
		contentLines = m.renderItems(listH)
	}

	// Row last: help bar (always 1 line).
	help := helpStyle.Render("  j/k move   enter select   a add all   n insert next   backspace back   s settings   v bongo   q quit")

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, row2, contentLines, help)
}

func (m BrowserModel) breadcrumb() string {
	parts := []string{tabNames[m.tab]}
	if m.selectedArtist != nil {
		parts = append(parts, m.selectedArtist.Name)
	}
	if m.selectedAlbum != nil {
		parts = append(parts, m.selectedAlbum.Name)
	}
	if m.selectedPlaylist != nil {
		parts = append(parts, m.selectedPlaylist.Name)
	}
	return breadcrumbStyle.Render(strings.Join(parts, " › "))
}

// SearchQuery returns the current search input value.
func (m BrowserModel) SearchQuery() string { return m.search.Value() }
