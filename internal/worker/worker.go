package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/video-compressor/internal/broker"
	"github.com/video-compressor/internal/compress"
	"github.com/video-compressor/internal/model"
)

type Worker struct {
	db              *sql.DB
	rdb             *redis.Client
	videoCompressor *compress.VideoCompressor
	imageCompressor *compress.ImageCompressor
	storagePath     string
}

func New(db *sql.DB, rdb *redis.Client, storagePath string) *Worker {
	return &Worker{
		db:              db,
		rdb:             rdb,
		videoCompressor: compress.NewVideoCompressor(),
		imageCompressor: compress.NewImageCompressor(),
		storagePath:     storagePath,
	}
}

func (w *Worker) ProcessJob(ctx context.Context, msg broker.Message) error {
	var job model.Job
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		return fmt.Errorf("failed to unmarshal job: %w", err)
	}

	log.Printf("processing job %s (type=%s)", job.ID, job.Type)

	// Update status to processing
	if err := w.updateJobStatus(ctx, job.ID, model.StatusProcessing, 0); err != nil {
		return err
	}
	w.publishSSEEvent(ctx, job.ID, "status", map[string]interface{}{
		"job_id": job.ID, "status": "processing", "progress": 0,
	})

	var outputSize int64
	var outputPath string

	switch job.Type {
	case model.TypeVideo:
		result, err := w.processVideo(ctx, &job)
		if err != nil {
			w.failJob(ctx, job.ID, err)
			return nil // don't requeue
		}
		outputSize = getFileSize(result.OutputPath)
		outputPath = result.OutputPath

	case model.TypeImage:
		result, err := w.processImage(ctx, &job)
		if err != nil {
			w.failJob(ctx, job.ID, err)
			return nil
		}
		outputSize = result.OutputSize
		outputPath = result.OutputPath

	default:
		w.failJob(ctx, job.ID, fmt.Errorf("unknown job type: %s", job.Type))
		return nil
	}

	// Complete the job
	now := time.Now()
	var ratio float64
	if job.InputSize > 0 {
		ratio = float64(outputSize) / float64(job.InputSize)
	}

	_, err := w.db.ExecContext(ctx, `
		UPDATE jobs SET status=$1, progress=100, output_path=$2, output_size=$3,
		compression_ratio=$4, completed_at=$5, updated_at=$6 WHERE id=$7`,
		model.StatusCompleted, outputPath, outputSize, ratio, now, now, job.ID)
	if err != nil {
		return fmt.Errorf("failed to update completed job: %w", err)
	}

	w.publishSSEEvent(ctx, job.ID, "completed", map[string]interface{}{
		"job_id":            job.ID,
		"status":            "completed",
		"download_url":      fmt.Sprintf("/api/v1/jobs/%s/download", job.ID),
		"compression_ratio": ratio,
		"output_size":       outputSize,
	})

	log.Printf("job %s completed (ratio=%.2f)", job.ID, ratio)
	return nil
}

func (w *Worker) processVideo(ctx context.Context, job *model.Job) (*compress.VideoResult, error) {
	if err := job.UnmarshalOptions(); err != nil {
		return nil, err
	}

	preset := resolveVideoPreset(job.Options)
	outputExt := "." + preset.Format
	outputPath := filepath.Join(filepath.Dir(job.InputPath), "output"+outputExt)

	onProgress := func(percent int) {
		w.updateJobStatus(ctx, job.ID, model.StatusProcessing, percent)
		w.publishSSEEvent(ctx, job.ID, "progress", map[string]interface{}{
			"job_id": job.ID, "progress": percent, "status": "processing",
		})
	}

	return w.videoCompressor.Compress(ctx, job.InputPath, outputPath, preset, onProgress)
}

func (w *Worker) processImage(ctx context.Context, job *model.Job) (*compress.ImageResult, error) {
	if err := job.UnmarshalOptions(); err != nil {
		return nil, err
	}

	preset := resolveImagePreset(job.Options)
	outputExt := "." + preset.Format
	outputPath := filepath.Join(filepath.Dir(job.InputPath), "output"+outputExt)

	return w.imageCompressor.Compress(ctx, job.InputPath, outputPath, preset)
}

func resolveVideoPreset(opts model.CompressionOptions) compress.VideoPreset {
	if opts.Preset != "" {
		if p, ok := compress.VideoPresets[opts.Preset]; ok {
			return p
		}
	}
	// Build from individual options, falling back to web-optimized defaults
	p := compress.VideoPresets["web-optimized"]
	if opts.Codec != "" {
		p.Codec = opts.Codec
	}
	if opts.CRF > 0 {
		p.CRF = opts.CRF
	}
	if opts.MaxWidth > 0 {
		p.MaxWidth = opts.MaxWidth
	}
	if opts.Format != "" {
		p.Format = opts.Format
	}
	return p
}

func resolveImagePreset(opts model.CompressionOptions) compress.ImagePreset {
	if opts.Preset != "" {
		if p, ok := compress.ImagePresets[opts.Preset]; ok {
			return p
		}
	}
	p := compress.ImagePresets["web-optimized"]
	if opts.Quality > 0 {
		p.Quality = opts.Quality
	}
	if opts.Width > 0 {
		p.Width = opts.Width
	}
	if opts.Height > 0 {
		p.Height = opts.Height
	}
	if opts.ImgFmt != "" {
		p.Format = opts.ImgFmt
	}
	return p
}

func (w *Worker) updateJobStatus(ctx context.Context, jobID string, status model.JobStatus, progress int) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE jobs SET status=$1, progress=$2, updated_at=$3 WHERE id=$4`,
		status, progress, time.Now(), jobID)
	return err
}

func (w *Worker) failJob(ctx context.Context, jobID string, jobErr error) {
	log.Printf("job %s failed: %v", jobID, jobErr)
	now := time.Now()
	w.db.ExecContext(ctx, `
		UPDATE jobs SET status=$1, error=$2, completed_at=$3, updated_at=$4 WHERE id=$5`,
		model.StatusFailed, jobErr.Error(), now, now, jobID)

	w.publishSSEEvent(ctx, jobID, "failed", map[string]interface{}{
		"job_id": jobID, "status": "failed", "error": jobErr.Error(),
	})
}

func (w *Worker) publishSSEEvent(ctx context.Context, jobID, eventType string, data map[string]interface{}) {
	payload, _ := json.Marshal(data)
	channel := fmt.Sprintf("job:%s:events", jobID)
	msg := fmt.Sprintf("%s:%s", eventType, string(payload))
	w.rdb.Publish(ctx, channel, msg)
}

func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
