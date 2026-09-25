// Package ffmpeg shells out to the ffmpeg binary to extract frames from a
// video file at a fixed interval, and zips the resulting frames.
package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// BuildExtractArgs is a pure function (no exec) that builds the ffmpeg CLI
// argument list for extracting one frame every intervalSeconds from
// inputPath into outputDir as frame_00001.jpg, frame_00002.jpg, ...
// intervalSeconds <= 0 defaults to 1 to guarantee a sane, non-zero fps filter.
func BuildExtractArgs(inputPath, outputDir string, intervalSeconds int) []string {
	if intervalSeconds <= 0 {
		intervalSeconds = 1
	}
	return []string{
		"-i", inputPath,
		"-vf", fmt.Sprintf("fps=1/%d", intervalSeconds),
		"-q:v", "2",
		filepath.Join(outputDir, "frame_%05d.jpg"),
	}
}

// ExtractFrames shells out to the real ffmpeg binary, wrapping
// BuildExtractArgs' output. ffmpeg writes its diagnostic output to stderr;
// that is exactly what the caller wants surfaced as a job's error_message on
// failure, so combined stdout+stderr is captured into the returned error.
// The context governs timeout/cancellation of the subprocess.
func ExtractFrames(ctx context.Context, inputPath, outputDir string, intervalSeconds int) error {
	args := BuildExtractArgs(inputPath, outputDir, intervalSeconds)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, out.String())
	}
	return nil
}

// CountFrames globs outputDir for frame_*.jpg files and returns the count,
// used to populate processing_jobs.frame_count and the
// video.processing.completed event payload.
func CountFrames(outputDir string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(outputDir, "frame_*.jpg"))
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}
