package worker

import (
	"context"
	"log"
	"sync"

	"github.com/video-compressor/internal/broker"
)

type Pool struct {
	worker *Worker
	broker broker.Broker
}

func NewPool(w *Worker, b broker.Broker) *Pool {
	return &Pool{worker: w, broker: b}
}

func (p *Pool) Start(ctx context.Context, videoWorkers, imageWorkers int) error {
	var wg sync.WaitGroup

	// Start video workers
	for i := 0; i < videoWorkers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			log.Printf("video worker %d started", workerID)
			if err := p.broker.Consume(ctx, broker.QueueVideoJobs, p.worker.ProcessJob); err != nil {
				log.Printf("video worker %d error: %v", workerID, err)
			}
		}()
	}

	// Start image workers
	for i := 0; i < imageWorkers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			log.Printf("image worker %d started", workerID)
			if err := p.broker.Consume(ctx, broker.QueueImageJobs, p.worker.ProcessJob); err != nil {
				log.Printf("image worker %d error: %v", workerID, err)
			}
		}()
	}

	// Wait for context cancellation
	<-ctx.Done()
	wg.Wait()
	log.Println("all workers stopped")
	return nil
}
