// Package executor calls a job's HTTP callback and retries transient
// failures with exponential backoff + jitter.
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/zepyhr/job-orchestrator/internal/models"
	"github.com/zepyhr/job-orchestrator/internal/repository"
)

type Executor struct {
	execRepo *repository.ExecutionRepository
	client   *http.Client
}

func New(execRepo *repository.ExecutionRepository) *Executor {
	return &Executor{
		execRepo: execRepo,
		client:   &http.Client{}, // per-request timeout is set via context below
	}
}

// Run executes a job's callback, retrying up to job.MaxRetries times on
// failure (non-2xx response, timeout, or network error). Each attempt is
// recorded as its own execution row for a full audit trail. Intended to be
// called in its own goroutine per due job so one slow callback never blocks
// the scheduler tick.
func (e *Executor) Run(job models.Job) {
	maxAttempts := job.MaxRetries + 1 // MaxRetries=3 => up to 4 total attempts

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		execID, err := e.execRepo.Start(job.ID, attempt)
		if err != nil {
			log.Printf("[executor] job=%s failed to record execution start: %v", job.ID, err)
			return
		}

		status, body, callErr, durationMs := e.call(job)

		if callErr == nil && status >= 200 && status < 300 {
			bodySnippet := truncate(body, 2000)
			_ = e.execRepo.Finish(execID, models.StatusSuccess, &status, &bodySnippet, nil, durationMs)
			log.Printf("[executor] job=%s attempt=%d SUCCESS status=%d", job.ID, attempt, status)
			return
		}

		errMsg := describeFailure(status, callErr)
		isLastAttempt := attempt == maxAttempts
		finalStatus := models.StatusRetrying
		if isLastAttempt {
			finalStatus = models.StatusFailed
		}

		var httpStatusPtr *int
		if status > 0 {
			httpStatusPtr = &status
		}
		bodySnippet := truncate(body, 2000)
		_ = e.execRepo.Finish(execID, finalStatus, httpStatusPtr, &bodySnippet, &errMsg, durationMs)

		if isLastAttempt {
			log.Printf("[executor] job=%s attempt=%d FAILED (giving up): %s", job.ID, attempt, errMsg)
			return
		}

		wait := backoff(attempt)
		log.Printf("[executor] job=%s attempt=%d failed: %s — retrying in %s", job.ID, attempt, errMsg, wait)
		time.Sleep(wait)
	}
}

func (e *Executor) call(job models.Job) (status int, body string, err error, durationMs int) {
	timeout := time.Duration(job.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	method := job.HTTPMethod
	if method == "" {
		method = http.MethodPost
	}

	var reqBody io.Reader
	if job.Payload != nil {
		b, marshalErr := json.Marshal(job.Payload)
		if marshalErr == nil {
			reqBody = bytes.NewReader(b)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, job.CallbackURL, reqBody)
	if err != nil {
		return 0, "", err, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Orchestrator-Job-Id", job.ID)

	start := time.Now()
	resp, err := e.client.Do(req)
	durationMs = int(time.Since(start).Milliseconds())
	if err != nil {
		return 0, "", err, durationMs
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap read at 1MB
	return resp.StatusCode, string(respBody), nil, durationMs
}

// backoff computes exponential backoff with jitter: base 1s, doubling each
// attempt, capped at 30s, plus up to 20% random jitter so many failing jobs
// don't all retry in lockstep (thundering herd).
func backoff(attempt int) time.Duration {
	base := math.Min(math.Pow(2, float64(attempt-1)), 30) // 1, 2, 4, 8, 16, 30, 30...
	jitter := base * 0.2 * rand.Float64()
	return time.Duration((base + jitter) * float64(time.Second))
}

func describeFailure(status int, err error) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("non-2xx response: HTTP %d", status)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
