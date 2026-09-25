package ffmpeg

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildExtractArgs(t *testing.T) {
	tests := []struct {
		name            string
		inputPath       string
		outputDir       string
		intervalSeconds int
		want            []string
	}{
		{
			name:            "one second interval",
			inputPath:       "/tmp/in.mp4",
			outputDir:       "/tmp/out",
			intervalSeconds: 1,
			want: []string{
				"-i", "/tmp/in.mp4",
				"-vf", "fps=1/1",
				"-q:v", "2",
				filepath.Join("/tmp/out", "frame_%05d.jpg"),
			},
		},
		{
			name:            "five second interval",
			inputPath:       "/tmp/in.mp4",
			outputDir:       "/tmp/out",
			intervalSeconds: 5,
			want: []string{
				"-i", "/tmp/in.mp4",
				"-vf", "fps=1/5",
				"-q:v", "2",
				filepath.Join("/tmp/out", "frame_%05d.jpg"),
			},
		},
		{
			name:            "zero interval defaults to 1",
			inputPath:       "/tmp/in.mp4",
			outputDir:       "/tmp/out",
			intervalSeconds: 0,
			want: []string{
				"-i", "/tmp/in.mp4",
				"-vf", "fps=1/1",
				"-q:v", "2",
				filepath.Join("/tmp/out", "frame_%05d.jpg"),
			},
		},
		{
			name:            "negative interval defaults to 1",
			inputPath:       "/tmp/in.mp4",
			outputDir:       "/tmp/out",
			intervalSeconds: -3,
			want: []string{
				"-i", "/tmp/in.mp4",
				"-vf", "fps=1/1",
				"-q:v", "2",
				filepath.Join("/tmp/out", "frame_%05d.jpg"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildExtractArgs(tt.inputPath, tt.outputDir, tt.intervalSeconds)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountFrames(t *testing.T) {
	dir := t.TempDir()

	count, err := CountFrames(dir)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	for _, name := range []string{"frame_00001.jpg", "frame_00002.jpg", "frame_00003.jpg", "not-a-frame.txt"} {
		p := filepath.Join(dir, name)
		assert.NoError(t, writeEmptyFile(p))
	}

	count, err = CountFrames(dir)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func writeEmptyFile(path string) error {
	return osWriteFile(path)
}
