package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

type JobType string

const (
	TypeVideo JobType = "video"
	TypeImage JobType = "image"
)

type CompressionOptions struct {
	// Video options
	Codec    string `json:"codec,omitempty"`
	CRF      int    `json:"crf,omitempty"`
	MaxWidth int    `json:"max_width,omitempty"`
	Format   string `json:"format,omitempty"`

	// Image options
	Quality int    `json:"quality,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	ImgFmt  string `json:"img_format,omitempty"`

	// Common
	Preset string `json:"preset,omitempty"`
}

type Job struct {
	ID               string             `json:"id" db:"id"`
	Status           JobStatus          `json:"status" db:"status"`
	Type             JobType            `json:"type" db:"type"`
	OriginalFilename string             `json:"original_filename" db:"original_filename"`
	// NOTE: serialized into the RabbitMQ job message (the worker needs the input
	// path). Not exposed by the public API, which uses JobResponse, not this model.
	InputPath        string             `json:"input_path" db:"input_path"`
	OutputPath       string             `json:"output_path" db:"output_path"`
	Progress         int                `json:"progress" db:"progress"`
	InputSize        int64              `json:"input_size" db:"input_size"`
	OutputSize       int64              `json:"output_size" db:"output_size"`
	CompressionRatio float64            `json:"compression_ratio" db:"compression_ratio"`
	Options          CompressionOptions `json:"options" db:"-"`
	OptionsJSON      string             `json:"-" db:"options_json"`
	Error            sql.NullString     `json:"error,omitempty" db:"error"`
	WebhookURL       string             `json:"webhook_url,omitempty" db:"webhook_url"`
	CreatedAt        time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" db:"updated_at"`
	CompletedAt      *time.Time         `json:"completed_at,omitempty" db:"completed_at"`
}

func (j *Job) MarshalOptions() error {
	b, err := json.Marshal(j.Options)
	if err != nil {
		return err
	}
	j.OptionsJSON = string(b)
	return nil
}

func (j *Job) UnmarshalOptions() error {
	if j.OptionsJSON == "" {
		return nil
	}
	return json.Unmarshal([]byte(j.OptionsJSON), &j.Options)
}
