package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/zepyhr/job-orchestrator/internal/models"
)

type JobRepository struct {
	db *sql.DB
}

func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) Create(j *models.Job) error {
	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return err
	}
	return r.db.QueryRow(`
		INSERT INTO jobs (name, cron_expression, http_method, callback_url, payload,
		                   max_retries, timeout_seconds, enabled, next_run_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, j.Name, j.CronExpression, j.HTTPMethod, j.CallbackURL, payload,
		j.MaxRetries, j.TimeoutSeconds, j.Enabled, j.NextRunAt,
	).Scan(&j.ID, &j.CreatedAt, &j.UpdatedAt)
}

func (r *JobRepository) List() ([]models.Job, error) {
	rows, err := r.db.Query(`
		SELECT id, name, cron_expression, http_method, callback_url, payload,
		       max_retries, timeout_seconds, enabled, next_run_at, last_run_at,
		       created_at, updated_at
		FROM jobs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (r *JobRepository) Get(id string) (*models.Job, error) {
	row := r.db.QueryRow(`
		SELECT id, name, cron_expression, http_method, callback_url, payload,
		       max_retries, timeout_seconds, enabled, next_run_at, last_run_at,
		       created_at, updated_at
		FROM jobs WHERE id = $1
	`, id)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *JobRepository) Update(id string, req models.UpdateJobRequest) (*models.Job, error) {
	current, err := r.Get(id)
	if err != nil || current == nil {
		return current, err
	}

	if req.Name != nil {
		current.Name = *req.Name
	}
	if req.CronExpression != nil {
		current.CronExpression = *req.CronExpression
	}
	if req.HTTPMethod != nil {
		current.HTTPMethod = *req.HTTPMethod
	}
	if req.CallbackURL != nil {
		current.CallbackURL = *req.CallbackURL
	}
	if req.Payload != nil {
		current.Payload = req.Payload
	}
	if req.MaxRetries != nil {
		current.MaxRetries = *req.MaxRetries
	}
	if req.TimeoutSeconds != nil {
		current.TimeoutSeconds = *req.TimeoutSeconds
	}
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}

	payload, err := json.Marshal(current.Payload)
	if err != nil {
		return nil, err
	}

	_, err = r.db.Exec(`
		UPDATE jobs SET name=$1, cron_expression=$2, http_method=$3, callback_url=$4,
		                payload=$5, max_retries=$6, timeout_seconds=$7, enabled=$8,
		                next_run_at=$9, updated_at=now()
		WHERE id=$10
	`, current.Name, current.CronExpression, current.HTTPMethod, current.CallbackURL,
		payload, current.MaxRetries, current.TimeoutSeconds, current.Enabled,
		current.NextRunAt, id)
	if err != nil {
		return nil, err
	}
	return r.Get(id)
}

func (r *JobRepository) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM jobs WHERE id=$1`, id)
	return err
}

// DueJobs returns enabled jobs whose next_run_at has arrived. Called only
// by the current leader (see scheduler.Scheduler.tick).
func (r *JobRepository) DueJobs(now time.Time) ([]models.Job, error) {
	rows, err := r.db.Query(`
		SELECT id, name, cron_expression, http_method, callback_url, payload,
		       max_retries, timeout_seconds, enabled, next_run_at, last_run_at,
		       created_at, updated_at
		FROM jobs
		WHERE enabled = true AND next_run_at IS NOT NULL AND next_run_at <= $1
		ORDER BY next_run_at ASC
		LIMIT 100
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// SetNextRun updates a job's schedule bookkeeping after the scheduler has
// dispatched it for the current tick.
func (r *JobRepository) SetNextRun(id string, lastRun, nextRun time.Time) error {
	_, err := r.db.Exec(`UPDATE jobs SET last_run_at=$1, next_run_at=$2 WHERE id=$3`, lastRun, nextRun, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (models.Job, error) {
	var j models.Job
	var payload []byte
	err := row.Scan(&j.ID, &j.Name, &j.CronExpression, &j.HTTPMethod, &j.CallbackURL,
		&payload, &j.MaxRetries, &j.TimeoutSeconds, &j.Enabled, &j.NextRunAt, &j.LastRunAt,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return j, err
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &j.Payload)
	}
	return j, nil
}
