// Package subsonic implements a client for the Subsonic REST API,
// which is the protocol spoken by Navidrome.
package subsonic

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const clientName = "kagura"
const apiVersion = "1.16.1"

// Client is a Subsonic API client.
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

// New creates a new Client. baseURL should be the root of the Navidrome server
// (e.g. "http://localhost:4533").
func New(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// --- Auth helpers ---

// authParams builds the token-based auth query params required by Subsonic.
func (c *Client) authParams() url.Values {
	salt := strconv.FormatInt(rand.Int63(), 36)
	token := fmt.Sprintf("%x", md5.Sum([]byte(c.password+salt)))
	return url.Values{
		"u":   {c.username},
		"t":   {token},
		"s":   {salt},
		"v":   {apiVersion},
		"c":   {clientName},
		"f":   {"json"},
	}
}

// get performs a GET request to the given Subsonic endpoint with extra params merged in.
func (c *Client) get(endpoint string, params url.Values) ([]byte, error) {
	p := c.authParams()
	for k, vs := range params {
		p[k] = vs
	}
	u := fmt.Sprintf("%s/rest/%s?%s", c.baseURL, endpoint, p.Encode())
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// --- Domain types ---

// Artist represents a library artist.
type Artist struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
}

// Album represents a library album.
type Album struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId"`
	Year     int    `json:"year"`
	Duration int    `json:"duration"`
	SongCount int   `json:"songCount"`
}

// Song represents a single track.
type Song struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	AlbumID     string `json:"albumId"`
	ArtistID    string `json:"artistId"`
	Duration    int    `json:"duration"`
	Track       int    `json:"track"`
	BitRate     int    `json:"bitRate"`
	ContentType string `json:"contentType"`
	Suffix      string `json:"suffix"`
	BPM         int    `json:"bpm"`
}

// Playlist represents a server-side playlist.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
	Owner     string `json:"owner"`
}

// --- API methods ---

// Ping checks connectivity and authentication. Returns an error if the server
// is unreachable or credentials are wrong.
func (c *Client) Ping() error {
	data, err := c.get("ping", nil)
	if err != nil {
		return err
	}
	var wrapper struct {
		Response struct {
			Status string `json:"status"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	r := wrapper.Response
	if r.Status != "ok" && r.Error != nil {
		return fmt.Errorf("subsonic error %d: %s", r.Error.Code, r.Error.Message)
	}
	return nil
}

// GetArtists returns all artists in the library.
func (c *Client) GetArtists() ([]Artist, error) {
	data, err := c.get("getArtists", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Artists struct {
				Index []struct {
					Artist []Artist `json:"artist"`
				} `json:"index"`
			} `json:"artists"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	var all []Artist
	for _, idx := range wrapper.Response.Artists.Index {
		all = append(all, idx.Artist...)
	}
	return all, nil
}

// GetAlbums returns albums for a given artist ID.
func (c *Client) GetAlbums(artistID string) ([]Album, error) {
	data, err := c.get("getArtist", url.Values{"id": {artistID}})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Artist struct {
				Album []Album `json:"album"`
			} `json:"artist"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.Artist.Album, nil
}

// GetSongs returns the tracklist for an album.
func (c *Client) GetSongs(albumID string) ([]Song, error) {
	data, err := c.get("getAlbum", url.Values{"id": {albumID}})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Album struct {
				Song []Song `json:"song"`
			} `json:"album"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.Album.Song, nil
}

// GetPlaylists returns all server-side playlists.
func (c *Client) GetPlaylists() ([]Playlist, error) {
	data, err := c.get("getPlaylists", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Playlists struct {
				Playlist []Playlist `json:"playlist"`
			} `json:"playlists"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.Playlists.Playlist, nil
}

// GetPlaylistSongs returns the tracks in a playlist.
func (c *Client) GetPlaylistSongs(playlistID string) ([]Song, error) {
	data, err := c.get("getPlaylist", url.Values{"id": {playlistID}})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Playlist struct {
				Entry []Song `json:"entry"`
			} `json:"playlist"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.Playlist.Entry, nil
}

// Search queries across artists, albums, and songs.
func (c *Client) Search(query string) ([]Artist, []Album, []Song, error) {
	params := url.Values{
		"query":         {query},
		"artistCount":   {"20"},
		"albumCount":    {"20"},
		"songCount":     {"50"},
	}
	data, err := c.get("search3", params)
	if err != nil {
		return nil, nil, nil, err
	}
	var wrapper struct {
		Response struct {
			SearchResult3 struct {
				Artist []Artist `json:"artist"`
				Album  []Album  `json:"album"`
				Song   []Song   `json:"song"`
			} `json:"searchResult3"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, nil, nil, err
	}
	r := wrapper.Response.SearchResult3
	return r.Artist, r.Album, r.Song, nil
}

// GetNewestAlbums returns up to size albums sorted by date added (newest first).
func (c *Client) GetNewestAlbums(size int) ([]Album, error) {
	params := url.Values{
		"type": {"newest"},
		"size": {strconv.Itoa(size)},
	}
	data, err := c.get("getAlbumList2", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			AlbumList2 struct {
				Album []Album `json:"album"`
			} `json:"albumList2"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.AlbumList2.Album, nil
}

// GetAlbumList returns up to size albums sorted alphabetically by name.
func (c *Client) GetAlbumList(size int) ([]Album, error) {
	params := url.Values{
		"type": {"alphabeticalByName"},
		"size": {strconv.Itoa(size)},
	}
	data, err := c.get("getAlbumList2", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			AlbumList2 struct {
				Album []Album `json:"album"`
			} `json:"albumList2"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.AlbumList2.Album, nil
}

// GetRandomSongs returns size randomly selected songs from the library.
func (c *Client) GetRandomSongs(size int) ([]Song, error) {
	params := url.Values{
		"size": {strconv.Itoa(size)},
	}
	data, err := c.get("getRandomSongs", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			RandomSongs struct {
				Song []Song `json:"song"`
			} `json:"randomSongs"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.RandomSongs.Song, nil
}

// GetTopSongs returns up to count top songs for the given artist name.
func (c *Client) GetTopSongs(artistName string, count int) ([]Song, error) {
	params := url.Values{
		"artist": {artistName},
		"count":  {strconv.Itoa(count)},
	}
	data, err := c.get("getTopSongs", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			TopSongs struct {
				Song []Song `json:"song"`
			} `json:"topSongs"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.TopSongs.Song, nil
}

// SimilarArtist is a trimmed artist record returned by getArtistInfo2.
type SimilarArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetSimilarArtists returns similar artists for the given artist ID,
// using Navidrome's getArtistInfo2 endpoint (backed by Last.fm).
func (c *Client) GetSimilarArtists(artistID string, count int) ([]SimilarArtist, error) {
	params := url.Values{
		"id":    {artistID},
		"count": {strconv.Itoa(count)},
	}
	data, err := c.get("getArtistInfo2", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			ArtistInfo2 struct {
				SimilarArtist []SimilarArtist `json:"similarArtist"`
			} `json:"artistInfo2"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.ArtistInfo2.SimilarArtist, nil
}

// GetSimilarSongs returns up to count songs similar to the given song.
// Uses the Subsonic getSimilarSongs endpoint (backed by last.fm / ListenBrainz on Navidrome).
// Falls back gracefully — callers should try GetRandomSongs if this returns nothing.
func (c *Client) GetSimilarSongs(songID string, count int) ([]Song, error) {
	params := url.Values{
		"id":    {songID},
		"count": {strconv.Itoa(count)},
	}
	data, err := c.get("getSimilarSongs", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			SimilarSongs struct {
				Song []Song `json:"song"`
			} `json:"similarSongs"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Response.SimilarSongs.Song, nil
}

// PlayQueue represents a play queue saved on the server.
type PlayQueue struct {
	Entries    []Song
	CurrentID  string // ID of the currently playing song
	PositionMs int64  // playback position in milliseconds
}

// GetPlayQueue fetches the user's saved play queue from the server.
// Returns nil (no error) when no queue has been saved yet.
func (c *Client) GetPlayQueue() (*PlayQueue, error) {
	data, err := c.get("getPlayQueue", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			PlayQueue *struct {
				Entry    []Song `json:"entry"`
				Current  string `json:"current"`
				Position int64  `json:"position"`
			} `json:"playQueue"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	pq := wrapper.Response.PlayQueue
	if pq == nil || len(pq.Entry) == 0 {
		return nil, nil
	}
	return &PlayQueue{
		Entries:    pq.Entry,
		CurrentID:  pq.Current,
		PositionMs: pq.Position,
	}, nil
}

// SavePlayQueue persists the current play queue to the server so it can be
// restored on next launch or from another Subsonic client. songIDs must be in
// play order. currentID is the ID of the currently playing song; positionMs is
// the playback position in milliseconds. Pass an empty songIDs slice to clear.
func (c *Client) SavePlayQueue(songIDs []string, currentID string, positionMs int64) error {
	params := url.Values{}
	for _, id := range songIDs {
		params.Add("id", id)
	}
	if currentID != "" {
		params.Set("current", currentID)
	}
	if positionMs > 0 {
		params.Set("position", strconv.FormatInt(positionMs, 10))
	}
	_, err := c.get("savePlayQueue", params)
	return err
}

// StarredResult holds all starred (favourited) items.
type StarredResult struct {
	Artists []Artist
	Albums  []Album
	Songs   []Song
}

// GetStarred returns all items the user has starred in Navidrome.
func (c *Client) GetStarred() (*StarredResult, error) {
	data, err := c.get("getStarred2", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Starred2 struct {
				Artist []Artist `json:"artist"`
				Album  []Album  `json:"album"`
				Song   []Song   `json:"song"`
			} `json:"starred2"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	s := wrapper.Response.Starred2
	return &StarredResult{
		Artists: s.Artist,
		Albums:  s.Album,
		Songs:   s.Song,
	}, nil
}

// StreamURL returns the URL to stream a given song. This can be passed directly
// to mpv.
func (c *Client) StreamURL(songID string) string {
	p := c.authParams()
	p.Set("id", songID)
	return fmt.Sprintf("%s/rest/stream?%s", c.baseURL, p.Encode())
}

// --- Lyrics ---

// LyricLine is a single lyric line, optionally timestamped.
// TimeMs is 0 for plain (unsynced) lyrics.
type LyricLine struct {
	TimeMs int
	Text   string
}

// GetLyricsBySongId returns synced (LRC) lyrics for a song using the
// OpenSubsonic getLyricsBySongId endpoint (Navidrome 0.49+).
// Returns (lines, synced, error). Lines are sorted by TimeMs.
func (c *Client) GetLyricsBySongId(id string) ([]LyricLine, bool, error) {
	data, err := c.get("getLyricsBySongId", url.Values{"id": {id}})
	if err != nil {
		return nil, false, err
	}
	type lyricLine struct {
		Start int    `json:"start"`
		Value string `json:"value"`
	}
	type structuredLyric struct {
		Synced bool        `json:"synced"`
		Line   []lyricLine `json:"line"`
	}
	var wrapper struct {
		Response struct {
			LyricsList struct {
				StructuredLyrics []structuredLyric `json:"structuredLyrics"`
			} `json:"lyricsList"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, false, err
	}
	all := wrapper.Response.LyricsList.StructuredLyrics
	if len(all) == 0 {
		return nil, false, nil
	}
	// Prefer a synced entry; fall back to first entry.
	chosen := all[0]
	for _, sl := range all {
		if sl.Synced {
			chosen = sl
			break
		}
	}
	lines := make([]LyricLine, len(chosen.Line))
	for i, l := range chosen.Line {
		lines[i] = LyricLine{TimeMs: l.Start, Text: l.Value}
	}
	return lines, chosen.Synced, nil
}

// GetLyrics fetches plain-text lyrics via the legacy getLyrics endpoint.
// Returns one LyricLine per non-empty line; TimeMs is always 0.
func (c *Client) GetLyrics(artist, title string) ([]LyricLine, error) {
	data, err := c.get("getLyrics", url.Values{"artist": {artist}, "title": {title}})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Response struct {
			Lyrics struct {
				Value string `json:"value"`
			} `json:"lyrics"`
		} `json:"subsonic-response"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(wrapper.Response.Lyrics.Value)
	if text == "" {
		return nil, nil
	}
	rawLines := strings.Split(text, "\n")
	lines := make([]LyricLine, 0, len(rawLines))
	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, LyricLine{Text: l})
		}
	}
	return lines, nil
}
