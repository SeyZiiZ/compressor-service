package api

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// JobEvents godoc
// @Summary      Stream job events (SSE)
// @Description  Opens a Server-Sent Events stream for real-time job progress updates.
// @Description  Events: progress, status, completed, failed.
// @Description  The stream auto-closes after a completed or failed event.
// @Tags         jobs
// @Produce      text/event-stream
// @Param        id   path      string  true  "Job ID"
// @Success      200  {string}  string  "SSE stream"
// @Failure      404  {object}  ErrorResponse
// @Router       /api/v1/jobs/{id}/events [get]
func (h *Handler) JobEvents(c *gin.Context) {
	jobID := c.Param("id")

	// Verify job exists
	_, err := h.getJobByID(c, jobID)
	if err != nil {
		c.JSON(404, ErrorResponse{Error: "job not found"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ctx := c.Request.Context()
	channel := fmt.Sprintf("job:%s:events", jobID)

	// Subscribe to Redis Pub/Sub channel
	sub := h.rdb.Subscribe(ctx, channel)
	defer sub.Close()

	ch := sub.Channel()

	// Send initial connected event
	c.SSEvent("connected", fmt.Sprintf(`{"job_id":"%s"}`, jobID))
	c.Writer.Flush()

	// Heartbeat ticker
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-ch:
			if !ok {
				return
			}
			// Message format: "eventType:jsonPayload"
			eventType, payload := parseSSEMessage(msg.Payload)
			c.SSEvent(eventType, payload)
			c.Writer.Flush()

			// Auto-close on terminal events
			if eventType == "completed" || eventType == "failed" {
				return
			}

		case <-heartbeat.C:
			// Send heartbeat to keep connection alive
			io.WriteString(c.Writer, ": heartbeat\n\n")
			c.Writer.Flush()
		}
	}
}

// CheckJobAndStreamEvents checks if the job is already completed and either returns the
// final state or opens an SSE stream.
func (h *Handler) CheckJobAndStreamEvents(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.getJobByID(c, jobID)
	if err != nil {
		c.JSON(404, ErrorResponse{Error: "job not found"})
		return
	}

	// If job is already in a terminal state, send the final event immediately
	if job.Status == "completed" || job.Status == "failed" {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		resp := toJobResponse(job)
		if job.Status == "completed" {
			c.SSEvent("completed", resp)
		} else {
			c.SSEvent("failed", resp)
		}
		c.Writer.Flush()
		return
	}

	// Otherwise, open the SSE stream
	h.JobEvents(c)
}

func parseSSEMessage(payload string) (eventType string, data string) {
	idx := strings.Index(payload, ":")
	if idx == -1 {
		log.Printf("malformed SSE message: %s", payload)
		return "message", payload
	}
	return payload[:idx], payload[idx+1:]
}

// SetupSSE initializes Redis subscription for a specific job.
// Used internally by workers to ensure the channel exists before publishing.
func SetupSSE(rdb *redis.Client) {
	// No-op: Redis Pub/Sub channels are created on first subscribe/publish
	_ = rdb
}
