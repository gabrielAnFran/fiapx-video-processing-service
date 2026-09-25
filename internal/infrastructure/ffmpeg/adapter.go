package ffmpeg

import "context"

// Adapter adapts this package's free functions to the usecases.Extractor
// interface, so the use case layer depends on a small interface instead of
// this package's exec.CommandContext-based free functions directly.
type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (a *Adapter) ExtractFrames(ctx context.Context, inputPath, outputDir string, intervalSeconds int) error {
	return ExtractFrames(ctx, inputPath, outputDir, intervalSeconds)
}

func (a *Adapter) CountFrames(outputDir string) (int, error) {
	return CountFrames(outputDir)
}

func (a *Adapter) ZipDirectory(dir, destZipPath string) error {
	return ZipDirectory(dir, destZipPath)
}
