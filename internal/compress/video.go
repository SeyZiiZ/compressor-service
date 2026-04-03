package compress

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type VideoCompressor struct{}

func NewVideoCompressor() *VideoCompressor {
	return &VideoCompressor{}
}

type VideoResult struct {
	OutputPath string
	OutputSize int64
}

type ProgressCallback func(percent int)

func (vc *VideoCompressor) Compress(ctx context.Context, inputPath, outputPath string, opts VideoPreset, onProgress ProgressCallback) (*VideoResult, error) {
	duration, err := probeDuration(ctx, inputPath)
	if err != nil {
		duration = 0
	}

	args := buildFFmpegArgs(inputPath, outputPath, opts)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Parse progress from stderr
	if duration > 0 && onProgress != nil {
		scanner := bufio.NewScanner(stderr)
		scanner.Split(scanFFmpegOutput)
		for scanner.Scan() {
			line := scanner.Text()
			if p := parseProgress(line, duration); p >= 0 {
				onProgress(p)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	return &VideoResult{OutputPath: outputPath}, nil
}

func buildFFmpegArgs(input, output string, opts VideoPreset) []string {
	args := []string{
		"-y",
		"-i", input,
		"-c:v", opts.Codec,
		"-crf", strconv.Itoa(opts.CRF),
		"-c:a", "aac",
		"-b:a", "128k",
	}

	if opts.MaxWidth > 0 {
		// Scale down to max width, preserve aspect ratio, ensure even dimensions
		args = append(args, "-vf",
			fmt.Sprintf("scale='min(%d,iw)':'-2'", opts.MaxWidth))
	}

	args = append(args, opts.Extra...)
	args = append(args, output)
	return args
}

func probeDuration(ctx context.Context, inputPath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

var timeRegex = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

func parseProgress(line string, totalDuration float64) int {
	matches := timeRegex.FindStringSubmatch(line)
	if matches == nil {
		return -1
	}

	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	seconds, _ := strconv.Atoi(matches[3])
	ms, _ := strconv.Atoi(matches[4])

	current := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(ms*10)*time.Millisecond

	percent := int(float64(current.Milliseconds()) / (totalDuration * 1000) * 100)
	if percent > 100 {
		percent = 100
	}
	if percent < 0 {
		percent = 0
	}
	return percent
}

// scanFFmpegOutput splits FFmpeg's stderr on \r or \n
func scanFFmpegOutput(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' || data[i] == '\n' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
