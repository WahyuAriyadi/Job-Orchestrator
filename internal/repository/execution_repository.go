package repository

import (
	"database/sql"

	"github.com/zepyhr/job-orchestrator/internal/models"
)

type ExecutionRepository struct {
	db *sql.DB
}

func NewExecutionRepository(db *sql.DB) *ExecutionRepository {
	return &ExecutionRepository{db: db}
}

// Start inserts a new execution row in "running" state and returns its ID.
func (r *ExecutionRepository) Start(jobID string, attempt int) (string, error) {
	var id string
	err := r.db.QueryRow(`
		INSERT INTO executions (job_id, status, attempt, started_at)
		VALUES ($1, $2, $3, now())
		RETURNING id
	`, jobID, models.StatusRunning, attempt).Scan(&id)
	return id, err
}

// Finish records the outcome of an execution attempt.
func (r *ExecutionRepository) Finish(id string, status models.ExecutionStatus, httpStatus *int, body, errMsg *string, durationMs int) error {
	_, err := r.db.Exec(`
		UPDATE executions
		SET status=$1, http_status=$2, response_body=$3, error_message=$4,
		    finished_at=now(), duration_ms=$5
		WHERE id=$6
	`, status, httpStatus, body, errMsg, durationMs, id)
	return err
}

func (r *ExecutionRepository) ListByJob(jobID string, limit int) ([]models.Execution, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(`
		SELECT id, job_id, status, attempt, http_status, response_body, error_message,
		       started_at, finished_at, duration_ms
		FROM executions WHERE job_id=$1 ORDER BY started_at DESC LIMIT $2
	`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Execution
	for rows.Next() {
		var e models.Execution
		if err := rows.Scan(&e.ID, &e.JobID, &e.Status, &e.Attempt, &e.HTTPStatus,
			&e.ResponseBody, &e.ErrorMessage, &e.StartedAt, &e.FinishedAt, &e.DurationMs); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
