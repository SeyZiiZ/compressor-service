package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/video-compressor/internal/broker"
	"github.com/video-compressor/internal/model"
)

type Handler struct {
	db          *sql.DB
	rdb         *redis.Client
	broker      broker.Broker
	storagePath string
}

func NewHandler(db *sql.DB, rdb *redis.Client, b broker.Broker, storagePath string) *Handler {
	return &Handler{
		db:          db,
		rdb:         rdb,
		broker:      b,
		storagePath: storagePath,
	}
}

// CreateJob godoc
// @Summary      Submit a compression job
// @Description  Upload a file or provide a URL to compress. Returns a job ID for tracking.
// @Tags         jobs
// @Accept       multipart/form-data
// @Produce      json
// @Param        file       formData  file    false  "File to compress"
// @Param        type       formData  string  true   "Job type: video or image" Enums(video, image)
// @Param        preset     formData  string  false  "Compression preset" Enums(web-optimized, mobile, archive, h265-efficient, thumbnail)
// @Param        source_url formData  string  false  "URL of file to compress (alternative to file upload)"
// @Param        webhook_url formData string  false  "Webhook URL for completion callback"
// @Param        options    formData  string  false  "JSON compression options"
// @Success      201  {object}  JobResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/jobs [post]
func (h *Handler) CreateJob(c *gin.Context) {
	jobID := uuid.New().String()
	jobType := c.PostForm("type")
	if jobType == "" {
		// Try JSON body
		var req CreateJobRequest
		if err := c.ShouldBindJSON(&req); err == nil {
			jobType = req.Type
		}
	}

	if jobType != "video" && jobType != "image" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "type must be 'video' or 'image'"})
		return
	}

	// Create job directory
	jobDir := filepath.Join(h.storagePath, jobID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create job directory"})
		return
	}

	var inputPath string
	var inputSize int64
	var originalFilename string

	// Handle file upload
	file, header, err := c.Request.FormFile("file")
	if err == nil {
		defer file.Close()
		originalFilename = header.Filename
		inputPath = filepath.Join(jobDir, "input"+filepath.Ext(header.Filename))
		dst, err := os.Create(inputPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to save uploaded file"})
			return
		}
		defer dst.Close()
		written, err := io.Copy(dst, file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to write file"})
			return
		}
		inputSize = written
	} else {
		// Check for source_url
		sourceURL := c.PostForm("source_url")
		if sourceURL == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "file upload or source_url is required"})
			return
		}
		// Download from URL
		downloaded, size, fname, dlErr := downloadFile(sourceURL, jobDir)
		if dlErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to download source: " + dlErr.Error()})
			return
		}
		inputPath = downloaded
		inputSize = size
		originalFilename = fname
	}

	// Parse options
	var opts model.CompressionOptions
	if optStr := c.PostForm("options"); optStr != "" {
		json.Unmarshal([]byte(optStr), &opts)
	}
	if preset := c.PostForm("preset"); preset != "" {
		opts.Preset = preset
	}
	webhookURL := c.PostForm("webhook_url")

	// Create job in DB
	job := model.Job{
		ID:               jobID,
		Status:           model.StatusPending,
		Type:             model.JobType(jobType),
		OriginalFilename: originalFilename,
		InputPath:        inputPath,
		InputSize:        inputSize,
		Options:          opts,
		WebhookURL:       webhookURL,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := job.MarshalOptions(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to serialize options"})
		return
	}

	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO jobs (id, status, type, original_filename, input_path, input_size, options_json, webhook_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		job.ID, job.Status, job.Type, job.OriginalFilename, job.InputPath,
		job.InputSize, job.OptionsJSON, job.WebhookURL, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create job: " + err.Error()})
		return
	}

	// Publish to broker
	jobJSON, _ := json.Marshal(job)
	queue := broker.QueueVideoJobs
	if jobType == "image" {
		queue = broker.QueueImageJobs
	}
	if err := h.broker.Publish(c.Request.Context(), queue, broker.Message{
		JobID:   jobID,
		JobType: jobType,
		Body:    jobJSON,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to queue job"})
		return
	}

	c.JSON(http.StatusCreated, toJobResponse(&job))
}

// GetJob godoc
// @Summary      Get job status
// @Description  Returns the current status and metadata of a compression job
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  JobResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/v1/jobs/{id} [get]
func (h *Handler) GetJob(c *gin.Context) {
	job, err := h.getJobByID(c, c.Param("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to fetch job"})
		return
	}
	c.JSON(http.StatusOK, toJobResponse(job))
}

// ListJobs godoc
// @Summary      List jobs
// @Description  Returns a paginated list of compression jobs
// @Tags         jobs
// @Produce      json
// @Param        page   query     int     false  "Page number"  default(1)
// @Param        limit  query     int     false  "Items per page"  default(20)
// @Param        status query     string  false  "Filter by status" Enums(pending, processing, completed, failed)
// @Success      200    {object}  ListJobsResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /api/v1/jobs [get]
func (h *Handler) ListJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := "SELECT id, status, type, original_filename, progress, input_size, output_size, compression_ratio, error, created_at, completed_at FROM jobs"
	countQuery := "SELECT COUNT(*) FROM jobs"
	args := []interface{}{}
	countArgs := []interface{}{}

	if status != "" {
		query += " WHERE status=$1"
		countQuery += " WHERE status=$1"
		args = append(args, status)
		countArgs = append(countArgs, status)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)

	rows, err := h.db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to query jobs"})
		return
	}
	defer rows.Close()

	var jobs []JobResponse
	for rows.Next() {
		var j model.Job
		if err := rows.Scan(&j.ID, &j.Status, &j.Type, &j.OriginalFilename, &j.Progress,
			&j.InputSize, &j.OutputSize, &j.CompressionRatio, &j.Error, &j.CreatedAt, &j.CompletedAt); err != nil {
			continue
		}
		jobs = append(jobs, *toJobResponse(&j))
	}
	if jobs == nil {
		jobs = []JobResponse{}
	}

	var total int
	h.db.QueryRowContext(c.Request.Context(), countQuery, countArgs...).Scan(&total)

	c.JSON(http.StatusOK, ListJobsResponse{
		Jobs:  jobs,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// DownloadJob godoc
// @Summary      Download compressed file
// @Description  Downloads the compressed output file
// @Tags         jobs
// @Produce      octet-stream
// @Param        id   path      string  true  "Job ID"
// @Success      200  {file}    binary
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /api/v1/jobs/{id}/download [get]
func (h *Handler) DownloadJob(c *gin.Context) {
	job, err := h.getJobByID(c, c.Param("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to fetch job"})
		return
	}

	if job.Status != model.StatusCompleted {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "job is not completed yet", Details: "status: " + string(job.Status)})
		return
	}

	if _, err := os.Stat(job.OutputPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "output file not found"})
		return
	}

	filename := "compressed_" + job.OriginalFilename
	if ext := filepath.Ext(job.OutputPath); ext != "" {
		base := strings.TrimSuffix(job.OriginalFilename, filepath.Ext(job.OriginalFilename))
		filename = "compressed_" + base + ext
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.File(job.OutputPath)
}

// DeleteJob godoc
// @Summary      Delete a job
// @Description  Deletes a job and its associated files
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      204
// @Failure      404  {object}  ErrorResponse
// @Router       /api/v1/jobs/{id} [delete]
func (h *Handler) DeleteJob(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.getJobByID(c, jobID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to fetch job"})
		return
	}

	// Clean up files
	jobDir := filepath.Join(h.storagePath, job.ID)
	os.RemoveAll(jobDir)

	// Delete from DB
	h.db.ExecContext(c.Request.Context(), "DELETE FROM jobs WHERE id=$1", jobID)
	c.Status(http.StatusNoContent)
}

// Health godoc
// @Summary      Health check
// @Description  Returns the health status of the service and its dependencies
// @Tags         system
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /api/v1/health [get]
func (h *Handler) Health(c *gin.Context) {
	services := map[string]string{}

	// Check DB
	if err := h.db.PingContext(c.Request.Context()); err != nil {
		services["database"] = "down"
	} else {
		services["database"] = "up"
	}

	// Check Redis
	if err := h.rdb.Ping(c.Request.Context()).Err(); err != nil {
		services["redis"] = "down"
	} else {
		services["redis"] = "up"
	}

	status := "ok"
	for _, v := range services {
		if v == "down" {
			status = "degraded"
			break
		}
	}

	c.JSON(http.StatusOK, HealthResponse{Status: status, Services: services})
}

func (h *Handler) getJobByID(c *gin.Context, id string) (*model.Job, error) {
	var job model.Job
	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, status, type, original_filename, input_path, output_path, progress,
		input_size, output_size, compression_ratio, options_json, error, webhook_url, created_at, updated_at, completed_at
		FROM jobs WHERE id=$1`, id).Scan(
		&job.ID, &job.Status, &job.Type, &job.OriginalFilename, &job.InputPath, &job.OutputPath,
		&job.Progress, &job.InputSize, &job.OutputSize, &job.CompressionRatio,
		&job.OptionsJSON, &job.Error, &job.WebhookURL, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	if err != nil {
		return nil, err
	}
	job.UnmarshalOptions()
	return &job, nil
}

func toJobResponse(j *model.Job) *JobResponse {
	resp := &JobResponse{
		ID:               j.ID,
		Status:           string(j.Status),
		Type:             string(j.Type),
		OriginalFilename: j.OriginalFilename,
		Progress:         j.Progress,
		InputSize:        j.InputSize,
		OutputSize:       j.OutputSize,
		CompressionRatio: j.CompressionRatio,
		CreatedAt:        j.CreatedAt.Format(time.RFC3339),
	}
	if j.Error.Valid {
		resp.Error = &j.Error.String
	}
	if j.Status == model.StatusCompleted {
		url := fmt.Sprintf("/api/v1/jobs/%s/download", j.ID)
		resp.DownloadURL = &url
	}
	if j.CompletedAt != nil {
		t := j.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	return resp
}

func downloadFile(url string, destDir string) (path string, size int64, filename string, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", 0, "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Extract filename from URL
	parts := strings.Split(url, "/")
	filename = parts[len(parts)-1]
	if idx := strings.Index(filename, "?"); idx != -1 {
		filename = filename[:idx]
	}
	if filename == "" {
		filename = "input"
	}

	path = filepath.Join(destDir, "input"+filepath.Ext(filename))
	f, err := os.Create(path)
	if err != nil {
		return "", 0, filename, err
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return "", 0, filename, err
	}
	return path, written, filename, nil
}
