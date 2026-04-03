package compress

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

type ImageCompressor struct{}

func NewImageCompressor() *ImageCompressor {
	return &ImageCompressor{}
}

type ImageResult struct {
	OutputPath string
	OutputSize int64
}

// Compress uses FFmpeg to process images, avoiding the need for libvips/CGO.
func (ic *ImageCompressor) Compress(ctx context.Context, inputPath, outputPath string, opts ImagePreset) (*ImageResult, error) {
	args := buildImageFFmpegArgs(inputPath, outputPath, opts)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg image processing failed: %w, output: %s", err, string(output))
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output: %w", err)
	}

	return &ImageResult{
		OutputPath: outputPath,
		OutputSize: info.Size(),
	}, nil
}

func buildImageFFmpegArgs(input, output string, opts ImagePreset) []string {
	args := []string{"-y", "-i", input}

	// Build filter chain
	var filters []string

	if opts.Width > 0 && opts.Height > 0 {
		filters = append(filters, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", opts.Width, opts.Height))
	} else if opts.Width > 0 {
		filters = append(filters, fmt.Sprintf("scale=%d:-1", opts.Width))
	} else if opts.Height > 0 {
		filters = append(filters, fmt.Sprintf("scale=-1:%d", opts.Height))
	}

	if len(filters) > 0 {
		filterStr := filters[0]
		for i := 1; i < len(filters); i++ {
			filterStr += "," + filters[i]
		}
		args = append(args, "-vf", filterStr)
	}

	// Quality settings based on output format
	switch opts.Format {
	case "webp":
		args = append(args, "-quality", strconv.Itoa(opts.Quality))
	case "jpeg", "jpg":
		args = append(args, "-q:v", strconv.Itoa(qualityToFFmpegJPEG(opts.Quality)))
	case "png":
		// PNG is lossless, compression_level 0-9
		args = append(args, "-compression_level", "6")
	case "avif":
		args = append(args, "-crf", strconv.Itoa(qualityToCRF(opts.Quality)))
	}

	args = append(args, output)
	return args
}

// qualityToFFmpegJPEG converts a 0-100 quality to FFmpeg's JPEG q:v scale (2-31, lower is better)
func qualityToFFmpegJPEG(quality int) int {
	if quality <= 0 {
		quality = 80
	}
	// Map 100->2, 0->31
	return 2 + (100-quality)*29/100
}

// qualityToCRF converts a 0-100 quality to CRF (0-63, lower is better)
func qualityToCRF(quality int) int {
	if quality <= 0 {
		quality = 80
	}
	return (100 - quality) * 63 / 100
}
