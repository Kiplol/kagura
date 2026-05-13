package subsonic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const lrclibURL = "https://lrclib.net/api/get"

// lrcRe matches a single LRC timestamp line: [MM:SS.xx] text
var lrcRe = regexp.MustCompile(`^\[(\d+):(\d+)\.(\d+)\]\s*(.*)$`)

// GetLyricsFromLrcLib fetches lyrics from lrclib.net using track metadata.
// Returns synced LyricLines (with timestamps) if available, plain lines
// otherwise. Returns nil (no error) when the track simply isn't found.
// No API key required.
func GetLyricsFromLrcLib(artist, track, album string, durationSecs int) ([]LyricLine, bool, error) {
	params := url.Values{
		"track_name":  {track},
		"artist_name": {artist},
	}
	if album != "" {
		params.Set("album_name", album)
	}
	if durationSecs > 0 {
		params.Set("duration", strconv.Itoa(durationSecs))
	}

	req, err := http.NewRequest("GET", lrclibURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "Kagura/1.0 (https://github.com/kip/kagura)")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, false, nil // track not found — not an error
	}
	if resp.StatusCode != 200 {
		return nil, false, fmt.Errorf("lrclib: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	var result struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
		Instrumental bool   `json:"instrumental"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, false, err
	}

	if result.Instrumental {
		return nil, false, nil
	}

	// Prefer synced (LRC) lyrics.
	if result.SyncedLyrics != "" {
		lines := parseLRC(result.SyncedLyrics)
		if len(lines) > 0 {
			return lines, true, nil
		}
	}

	// Fall back to plain text.
	if result.PlainLyrics != "" {
		var lines []LyricLine
		for _, l := range strings.Split(strings.TrimSpace(result.PlainLyrics), "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				lines = append(lines, LyricLine{Text: l})
			}
		}
		return lines, false, nil
	}

	return nil, false, nil
}

// parseLRC converts an LRC-format string into timestamped LyricLines.
// Handles both [MM:SS.xx] (centiseconds) and [MM:SS.xxx] (milliseconds).
func parseLRC(lrc string) []LyricLine {
	var lines []LyricLine
	for _, raw := range strings.Split(lrc, "\n") {
		m := lrcRe.FindStringSubmatch(strings.TrimSpace(raw))
		if m == nil {
			continue
		}
		mins, _ := strconv.Atoi(m[1])
		secs, _ := strconv.Atoi(m[2])
		fracStr := m[3]
		frac, _ := strconv.Atoi(fracStr)
		var fracMs int
		switch len(fracStr) {
		case 1:
			fracMs = frac * 100
		case 2:
			fracMs = frac * 10
		case 3:
			fracMs = frac
		default:
			fracMs = frac * 10
		}
		timeMs := (mins*60+secs)*1000 + fracMs
		text := strings.TrimSpace(m[4])
		lines = append(lines, LyricLine{TimeMs: timeMs, Text: text})
	}
	return lines
}
