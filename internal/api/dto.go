package api

import "github.com/video-compressor/internal/model"

type CreateJobRequest struct {
	Type       string                    `json:"type" binding:"required,oneof=video image" example:"video"`
	SourceURL  string                    `json:"source_url,omitempty" example:"https://example.com/video.mp4"`
	Preset     string                    `json:"preset,omitempty" example:"web-optimized"`
	Options    model.CompressionOptions  `json:"options,omitempty"`
	WebhookURL string                    `json:"webhook_url,omitempty" example:"https://callback.example.com/done"`
}

type JobResponse struct {
	ID               string   `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Status           string   `json:"status" example:"pending"`
	Type             string   `json:"type" example:"video"`
	OriginalFilename string   `json:"original_filename" example:"video.mp4"`
	Progress         int      `json:"progress" example:"0"`
	InputSize        int64    `json:"input_size" example:"104857600"`
	OutputSize       int64    `json:"output_size" example:"0"`
	CompressionRatio float64  `json:"compression_ratio" example:"0"`
	Error            *string  `json:"error,omitempty"`
	DownloadURL      *string  `json:"download_url,omitempty" example:"/api/v1/jobs/550e8400/download"`
	CreatedAt        string   `json:"created_at" example:"2026-04-03T12:00:00Z"`
	CompletedAt      *string  `json:"completed_at,omitempty"`
}

type ListJobsResponse struct {
	Jobs  []JobResponse `json:"jobs"`
	Total int           `json:"total" example:"42"`
	Page  int           `json:"page" example:"1"`
	Limit int           `json:"limit" example:"20"`
}

type ErrorResponse struct {
	Error   string `json:"error" example:"job not found"`
	Details string `json:"details,omitempty"`
}

type HealthResponse struct {
	Status   string            `json:"status" example:"ok"`
	Services map[string]string `json:"services"`
}
