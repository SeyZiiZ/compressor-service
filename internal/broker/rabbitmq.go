package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
}

func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	rmq := &RabbitMQ{conn: conn, channel: ch, url: url}

	if err := rmq.declareQueues(); err != nil {
		rmq.Close()
		return nil, err
	}

	return rmq, nil
}

func (r *RabbitMQ) declareQueues() error {
	// Dead letter exchange
	if err := r.channel.ExchangeDeclare(
		"compression.dlx", "direct", true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("failed to declare DLX: %w", err)
	}

	// Dead letter queue
	if _, err := r.channel.QueueDeclare(
		"compression.jobs.dlq", true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("failed to declare DLQ: %w", err)
	}
	if err := r.channel.QueueBind(
		"compression.jobs.dlq", "#", "compression.dlx", false, nil,
	); err != nil {
		return fmt.Errorf("failed to bind DLQ: %w", err)
	}

	// Job queues with dead letter config
	queueArgs := amqp.Table{
		"x-dead-letter-exchange": "compression.dlx",
	}

	for _, q := range []string{QueueVideoJobs, QueueImageJobs} {
		if _, err := r.channel.QueueDeclare(
			q, true, false, false, false, queueArgs,
		); err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", q, err)
		}
	}

	return nil
}

func (r *RabbitMQ) Publish(ctx context.Context, queue string, msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.channel.PublishWithContext(ctx,
		"",    // default exchange
		queue, // routing key = queue name
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	)
}

func (r *RabbitMQ) Consume(ctx context.Context, queue string, handler Handler) error {
	if err := r.channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	deliveries, err := r.channel.Consume(
		queue,
		"",    // auto-generated consumer tag
		false, // manual ack
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to consume: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				var msg Message
				if err := json.Unmarshal(d.Body, &msg); err != nil {
					log.Printf("failed to unmarshal message: %v", err)
					d.Nack(false, false) // send to DLQ
					continue
				}
				if err := handler(ctx, msg); err != nil {
					log.Printf("failed to process message %s: %v", msg.JobID, err)
					d.Nack(false, false)
				} else {
					d.Ack(false)
				}
			}
		}
	}()

	return nil
}

func (r *RabbitMQ) Close() error {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
