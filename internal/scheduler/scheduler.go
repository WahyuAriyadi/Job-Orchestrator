package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/zepyhr/job-orchestrator/internal/cronparser"
	"github.com/zepyhr/job-orchestrator/internal/executor"
	"github.com/zepyhr/job-orchestrator/internal/models"
	"github.com/zepyhr/job-orchestrator/internal/repository"
)

// Scheduler polls the jobs table for due work. It's safe to run this same
// code on every instance of the service: the tick body is a no-op unless
// LeaderElector says this instance currently holds the lock, so only one
// instance ever actually dispatches a given job for a given due time.
type Scheduler struct {
	jobRepo  *repository.JobRepository
	executor *executor.Executor
	elector  *LeaderElector
	interval time.Duration
}

func New(jobRepo *repository.JobRepository, exec *executor.Executor, elector *LeaderElector, interval time.Duration) *Scheduler {
	return &Scheduler{jobRepo: jobRepo, executor: exec, elector: elector, interval: interval}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.elector.IsLeader() {
				continue // another instance is in charge; stay idle
			}
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	now := time.Now().UTC()
	due, err := s.jobRepo.DueJobs(now)
	if err != nil {
		log.Printf("[scheduler] failed to query due jobs: %v", err)
		return
	}
	for _, job := range due {
		s.dispatch(job, now)
	}
}

func (s *Scheduler) dispatch(job models.Job, now time.Time) {
	schedule, err := cronparser.Parse(job.CronExpression)
	if err != nil {
		log.Printf("[scheduler] job=%s has invalid cron expression %q, skipping: %v", job.ID, job.CronExpression, err)
		return
	}
	next := schedule.Next(now)

	// Advance next_run_at BEFORE executing so a slow/hanging callback can't
	// cause the same due job to be picked up again on the following tick.
	if err := s.jobRepo.SetNextRun(job.ID, now, next); err != nil {
		log.Printf("[scheduler] job=%s failed to advance schedule: %v", job.ID, err)
		return
	}

	log.Printf("[scheduler] dispatching job=%s (%s), next run at %s", job.ID, job.Name, next.Format(time.RFC3339))
	go s.executor.Run(job) // fire-and-forget: one slow callback must not block the tick
}

// InitialNextRun computes the first next_run_at for a freshly created job.
func InitialNextRun(cronExpr string) (time.Time, error) {
	schedule, err := cronparser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(time.Now().UTC()), nil
}
