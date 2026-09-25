package ffmpeg

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func osWriteFile(path string) error {
	return os.WriteFile(path, []byte("fake content for "+filepath.Base(path)), 0o644)
}

func TestZipDirectory(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"frame_00001.jpg": "content-1",
		"frame_00002.jpg": "content-2",
		"frame_00003.jpg": "content-3",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	destZip := filepath.Join(t.TempDir(), "frames.zip")
	require.NoError(t, ZipDirectory(dir, destZip))

	r, err := zip.OpenReader(destZip)
	require.NoError(t, err)
	defer r.Close()

	var gotNames []string
	gotContents := map[string]string{}
	for _, f := range r.File {
		gotNames = append(gotNames, f.Name)
		rc, err := f.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		gotContents[f.Name] = string(b)
	}

	var wantNames []string
	for name := range files {
		wantNames = append(wantNames, name)
	}
	sort.Strings(gotNames)
	sort.Strings(wantNames)

	assert.Equal(t, wantNames, gotNames)
	for name, content := range files {
		assert.Equal(t, content, gotContents[name])
	}
}

func TestZipDirectory_NonExistentDir(t *testing.T) {
	err := ZipDirectory(filepath.Join(t.TempDir(), "does-not-exist"), filepath.Join(t.TempDir(), "out.zip"))
	assert.Error(t, err)
}
