package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/zepyhr/job-orchestrator/internal/executor"
	"github.com/zepyhr/job-orchestrator/internal/models"
	"github.com/zepyhr/job-orchestrator/internal/repository"
	"github.com/zepyhr/job-orchestrator/internal/scheduler"
)

type Handler struct {
	jobRepo  *repository.JobRepository
	execRepo *repository.ExecutionRepository
	elector  *scheduler.LeaderElector
	exec     *executor.Executor
}

func NewHandler(jobRepo *repository.JobRepository, execRepo *repository.ExecutionRepository, elector *scheduler.LeaderElector, exec *executor.Executor) *Handler {
	return &Handler{jobRepo: jobRepo, execRepo: execRepo, elector: elector, exec: exec}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// POST /api/jobs
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req models.CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Name == "" || req.CronExpression == "" || req.CallbackURL == "" {
		writeError(w, http.StatusBadRequest, "name, cron_expression, and callback_url are required")
		return
	}

	nextRun, err := scheduler.InitialNextRun(req.CronExpression)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron_expression: "+err.Error())
		return
	}

	method := req.HTTPMethod
	if method == "" {
		method = http.MethodPost
	}
	maxRetries := 3
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}
	timeout := 30
	if req.TimeoutSeconds != nil {
		timeout = *req.TimeoutSeconds
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	job := &models.Job{
		Name:           req.Name,
		CronExpression: req.CronExpression,
		HTTPMethod:     method,
		CallbackURL:    req.CallbackURL,
		Payload:        req.Payload,
		MaxRetries:     maxRetries,
		TimeoutSeconds: timeout,
		Enabled:        enabled,
		NextRunAt:      &nextRun,
	}

	if err := h.jobRepo.Create(job); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create job: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

// GET /api/jobs
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.jobRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs: "+err.Error())
		return
	}
	if jobs == nil {
		jobs = []models.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// GET /api/jobs/{id}
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.jobRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get job: "+err.Error())
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// PUT /api/jobs/{id}
func (h *Handler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req models.UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	// If the cron expression changed, recompute next_run_at so the update
	// takes effect on the schedule immediately rather than at the old time.
	if req.CronExpression != nil {
		next, err := scheduler.InitialNextRun(*req.CronExpression)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cron_expression: "+err.Error())
			return
		}
		_ = next // applied via jobRepo.Update through a follow-up SetNextRun below
		job, err := h.jobRepo.Update(id, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update job: "+err.Error())
			return
		}
		if job == nil {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		if err := h.jobRepo.SetNextRun(id, time.Now().UTC(), next); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reschedule job: "+err.Error())
			return
		}
		job.NextRunAt = &next
		writeJSON(w, http.StatusOK, job)
		return
	}

	job, err := h.jobRepo.Update(id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update job: "+err.Error())
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// DELETE /api/jobs/{id}
func (h *Handler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.jobRepo.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete job: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/jobs/{id}/trigger — manual run, bypassing the schedule (handy
// for testing a callback without waiting for the cron time to arrive).
func (h *Handler) TriggerJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.jobRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get job: "+err.Error())
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	go h.exec.Run(*job)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "triggered"})
}

// GET /api/jobs/{id}/executions
func (h *Handler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	execs, err := h.execRepo.ListByJob(id, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list executions: "+err.Error())
		return
	}
	if execs == nil {
		execs = []models.Execution{}
	}
	writeJSON(w, http.StatusOK, execs)
}

// GET /api/health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"is_leader": h.elector.IsLeader(),
		"time":      time.Now().UTC(),
	})
}
