package ffmpeg

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ZipDirectory walks dir (non-recursive — it's a flat directory of
// frame_NNNNN.jpg files) and writes every regular file into a new zip
// archive at destZipPath, using only the stdlib archive/zip (no external
// `zip` binary dependency).
func ZipDirectory(dir, destZipPath string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return fmt.Errorf("create zip file: %w", err)
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)
	defer zw.Close()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if err := addFileToZip(zw, filepath.Join(dir, entry.Name()), entry.Name()); err != nil {
			return fmt.Errorf("add %s to zip: %w", entry.Name(), err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}
	return nil
}

func addFileToZip(zw *zip.Writer, srcPath, name string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	w, err := zw.Create(name)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, src)
	return err
}
