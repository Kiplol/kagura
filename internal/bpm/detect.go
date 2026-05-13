// Package bpm provides optional BPM detection via aubiotrack + ffmpeg.
// Both tools must be installed (e.g. `brew install aubio ffmpeg`).
// All activity is logged to /tmp/kagura.log for debugging.
package bpm

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// logf appends a timestamped line to /tmp/kagura.log.
func logf(format string, args ...any) {
	f, err := os.OpenFile("/tmp/kagura.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(f, "[bpm %s] "+format+"\n", append([]any{ts}, args...)...)
}

// findBin searches PATH then common Homebrew prefixes for a binary.
func findBin(names ...string) (string, error) {
	brewPrefixes := []string{"/opt/homebrew/bin/", "/usr/local/bin/", "/opt/local/bin/"}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
		for _, prefix := range brewPrefixes {
			candidate := prefix + name
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found in PATH or Homebrew", names[0])
}

// Detect analyzes the audio stream at streamURL and returns the detected BPM.
// It requires aubiotempo (part of the aubio suite, e.g. `brew install aubio`).
// Returns 0 if aubiotempo is not found, the stream is unreachable, or fewer
// than 4 beats are detected within the timeout.
// Detect analyzes the audio stream at streamURL and returns the detected BPM.
// Requires aubiotrack and ffmpeg (e.g. `brew install aubio ffmpeg`).
// aubiotrack's Homebrew build lacks HTTP support, so ffmpeg downloads the
// first 30 seconds into a temp WAV file that aubiotrack reads locally.
// Returns 0 if either tool is missing or detection fails.
func Detect(ctx context.Context, streamURL string) int {
	aubio, err := findBin("aubiotrack", "aubiotempo")
	if err != nil {
		logf("aubiotrack not found: %v", err)
		return 0
	}
	ffmpeg, err := findBin("ffmpeg")
	if err != nil {
		logf("ffmpeg not found: %v", err)
		return 0
	}
	logf("using %s and %s", aubio, ffmpeg)
	logf("analyzing stream: %.80s...", streamURL)

	// Create a temp WAV file for ffmpeg to write into.
	tmp, err := os.CreateTemp("", "kagura-bpm-*.wav")
	if err != nil {
		logf("CreateTemp error: %v", err)
		return 0
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	// Give the whole pipeline up to 40 seconds.
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	// Step 1: download first 30 seconds of the stream as mono 44.1 kHz WAV.
	logf("running ffmpeg → %s", tmpPath)
	ffmpegCmd := exec.CommandContext(ctx, ffmpeg,
		"-y",             // overwrite temp file
		"-i", streamURL,
		"-t", "30",       // first 30 seconds only
		"-ar", "44100",   // 44.1 kHz
		"-ac", "1",       // mono
		"-f", "wav",
		tmpPath,
	)
	ffmpegStderr := &strings.Builder{}
	ffmpegCmd.Stderr = ffmpegStderr
	if err := ffmpegCmd.Run(); err != nil {
		logf("ffmpeg error: %v", err)
		if s := strings.TrimSpace(ffmpegStderr.String()); s != "" {
			// Only log last 3 lines of ffmpeg stderr (it's very verbose).
			lines := strings.Split(s, "\n")
			if len(lines) > 3 {
				lines = lines[len(lines)-3:]
			}
			logf("ffmpeg stderr (tail): %s", strings.Join(lines, " | "))
		}
		return 0
	}
	logf("ffmpeg done, running aubiotrack on %s", tmpPath)

	// Step 2: run aubiotrack on the temp WAV file.
	trackCmd := exec.CommandContext(ctx, aubio, "-i", tmpPath)
	stdout, err := trackCmd.StdoutPipe()
	if err != nil {
		logf("StdoutPipe error: %v", err)
		return 0
	}
	trackStderr := &strings.Builder{}
	trackCmd.Stderr = trackStderr
	if err := trackCmd.Start(); err != nil {
		logf("aubiotrack start error: %v", err)
		return 0
	}
	defer func() {
		if trackCmd.Process != nil {
			_ = trackCmd.Process.Kill()
		}
		_ = trackCmd.Wait()
		if s := strings.TrimSpace(trackStderr.String()); s != "" {
			logf("aubiotrack stderr: %s", s)
		}
	}()

	// aubiotrack outputs one beat timestamp (seconds) per line.
	// Collect them and derive BPM from median inter-beat interval.
	var beats []float64
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Take the first field (handles both "t" and "t bpm" formats).
		field := strings.Fields(line)
		if len(field) == 0 {
			continue
		}
		if t, err := strconv.ParseFloat(field[0], 64); err == nil && t > 0 {
			beats = append(beats, t)
		}
	}
	logf("collected %d beat timestamps", len(beats))

	if len(beats) < 4 {
		logf("not enough beats — giving up")
		return 0
	}

	// Compute inter-beat intervals.
	intervals := make([]float64, len(beats)-1)
	for i := 1; i < len(beats); i++ {
		intervals[i-1] = beats[i] - beats[i-1]
	}
	sort.Float64s(intervals)
	median := intervals[len(intervals)/2]
	if median <= 0 {
		logf("median interval is zero — giving up")
		return 0
	}
	result := int(60.0/median + 0.5)
	logf("median interval=%.4fs → %d BPM", median, result)

	if result < 40 || result > 300 {
		logf("BPM %d out of plausible range — discarding", result)
		return 0
	}
	return result
}
