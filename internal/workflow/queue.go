// Package workflow provides job queues, retry, delayed jobs, DLQ, and cron.
package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dbx/dbx/internal/util"
)

// Job represents a unit of work.
type Job struct {
	ID          string
	Payload     []byte
	MaxRetries  int
	Retries     int
	CreatedAt   time.Time
	RunAt       time.Time
	LastError   string
}

// JobQueue is an in-memory job queue.
type JobQueue struct {
	mu      sync.Mutex
	jobs    []*Job
	maxSize int
}

// NewJobQueue creates a job queue.
func NewJobQueue(maxSize int) *JobQueue {
	return &JobQueue{maxSize: maxSize}
}

// Enqueue adds a job to the queue.
func (q *JobQueue) Enqueue(job *Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.maxSize > 0 && len(q.jobs) >= q.maxSize {
		return fmt.Errorf("queue is full")
	}
	q.jobs = append(q.jobs, job)
	return nil
}

// Dequeue removes and returns the next ready job.
func (q *JobQueue) Dequeue() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	for i, j := range q.jobs {
		if j.RunAt.IsZero() || now.After(j.RunAt) {
			q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
			return j
		}
	}
	return nil
}

// Len returns queue length.
func (q *JobQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}

// RetryQueue handles failed jobs with exponential backoff.
type RetryQueue struct {
	mu      sync.Mutex
	jobs    []*Job
	backoff *util.Backoff
}

// NewRetryQueue creates a retry queue.
func NewRetryQueue() *RetryQueue {
	return &RetryQueue{backoff: util.NewBackoff()}
}

// Retry schedules a job for retry.
func (r *RetryQueue) Retry(job *Job, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job.Retries++
	job.LastError = err.Error()
	job.RunAt = time.Now().Add(r.backoff.Next())
	r.jobs = append(r.jobs, job)
}

// Ready returns jobs ready to retry.
func (r *RetryQueue) Ready() []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var ready, remaining []*Job
	for _, j := range r.jobs {
		if now.After(j.RunAt) {
			ready = append(ready, j)
		} else {
			remaining = append(remaining, j)
		}
	}
	r.jobs = remaining
	return ready
}

// DeadLetterQueue stores permanently failed jobs.
type DeadLetterQueue struct {
	mu      sync.Mutex
	jobs    []*Job
	maxSize int
}

// NewDLQ creates a dead letter queue.
func NewDLQ(maxSize int) *DeadLetterQueue {
	return &DeadLetterQueue{maxSize: maxSize}
}

// Enqueue adds a permanently failed job to the DLQ.
func (d *DeadLetterQueue) Enqueue(job *Job) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.maxSize > 0 && len(d.jobs) >= d.maxSize {
		// Drop oldest
		d.jobs = d.jobs[1:]
	}
	d.jobs = append(d.jobs, job)
}

// List returns all DLQ jobs.
func (d *DeadLetterQueue) List() []*Job {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]*Job, len(d.jobs))
	copy(result, d.jobs)
	return result
}

// DelayedScheduler schedules jobs to run at a future time.
type DelayedScheduler struct {
	queue *JobQueue
	ctx   context.Context
	cancel context.CancelFunc
}

// NewDelayedScheduler creates a delayed scheduler.
func NewDelayedScheduler(queue *JobQueue) *DelayedScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &DelayedScheduler{queue: queue, ctx: ctx, cancel: cancel}
}

// Schedule enqueues a job to run at runAt.
func (s *DelayedScheduler) Schedule(job *Job, runAt time.Time) error {
	job.RunAt = runAt
	return s.queue.Enqueue(job)
}

// Stop stops the scheduler.
func (s *DelayedScheduler) Stop() { s.cancel() }
