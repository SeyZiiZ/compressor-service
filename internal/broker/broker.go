package broker

import "context"

type Message struct {
	JobID   string
	JobType string
	Body    []byte
}

type Handler func(ctx context.Context, msg Message) error

type Broker interface {
	Publish(ctx context.Context, queue string, msg Message) error
	Consume(ctx context.Context, queue string, handler Handler) error
	Close() error
}

const (
	QueueVideoJobs = "compression.jobs.video"
	QueueImageJobs = "compression.jobs.image"
)
