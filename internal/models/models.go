package models

import "time"

type ExecutionStatus string

const (
	StatusPending  ExecutionStatus = "pending"
	StatusRunning  ExecutionStatus = "running"
	StatusSuccess  ExecutionStatus = "success"
	StatusFailed   ExecutionStatus = "failed"
	StatusRetrying ExecutionStatus = "retrying"
)

// Job is a scheduled unit of work: "call this URL on this cron schedule".
type Job struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	CronExpression string         `json:"cron_expression"`
	HTTPMethod     string         `json:"http_method"`
	CallbackURL    string         `json:"callback_url"`
	Payload        map[string]any `json:"payload"`
	MaxRetries     int            `json:"max_retries"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	Enabled        bool           `json:"enabled"`
	NextRunAt      *time.Time     `json:"next_run_at"`
	LastRunAt      *time.Time     `json:"last_run_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// CreateJobRequest is the payload accepted by POST /api/jobs.
type CreateJobRequest struct {
	Name           string         `json:"name"`
	CronExpression string         `json:"cron_expression"`
	HTTPMethod     string         `json:"http_method"`
	CallbackURL    string         `json:"callback_url"`
	Payload        map[string]any `json:"payload"`
	MaxRetries     *int           `json:"max_retries"`
	TimeoutSeconds *int           `json:"timeout_seconds"`
	Enabled        *bool          `json:"enabled"`
}

// UpdateJobRequest mirrors CreateJobRequest; all fields optional (partial update).
type UpdateJobRequest struct {
	Name           *string        `json:"name"`
	CronExpression *string        `json:"cron_expression"`
	HTTPMethod     *string        `json:"http_method"`
	CallbackURL    *string        `json:"callback_url"`
	Payload        map[string]any `json:"payload"`
	MaxRetries     *int           `json:"max_retries"`
	TimeoutSeconds *int           `json:"timeout_seconds"`
	Enabled        *bool          `json:"enabled"`
}

// Execution records a single attempt (or retry) of running a job's callback.
type Execution struct {
	ID           string          `json:"id"`
	JobID        string          `json:"job_id"`
	Status       ExecutionStatus `json:"status"`
	Attempt      int             `json:"attempt"`
	HTTPStatus   *int            `json:"http_status"`
	ResponseBody *string         `json:"response_body"`
	ErrorMessage *string         `json:"error_message"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
	DurationMs   *int            `json:"duration_ms"`
}
