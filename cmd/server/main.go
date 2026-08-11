// Command server runs the job orchestrator: an HTTP API for managing
// scheduled jobs, plus a background scheduler loop that dispatches due jobs
// to their HTTP callbacks. Multiple copies of this binary can be run at
// once (e.g. scaled on Cloud Run); leader election ensures only one of them
// dispatches jobs at any moment.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zepyhr/job-orchestrator/internal/api"
	"github.com/zepyhr/job-orchestrator/internal/config"
	"github.com/zepyhr/job-orchestrator/internal/db"
	"github.com/zepyhr/job-orchestrator/internal/executor"
	"github.com/zepyhr/job-orchestrator/internal/repository"
	"github.com/zepyhr/job-orchestrator/internal/scheduler"
)

func main() {
	cfg := config.Load()

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer conn.Close()
	log.Println("connected to database")

	jobRepo := repository.NewJobRepository(conn)
	execRepo := repository.NewExecutionRepository(conn)

	elector := scheduler.NewLeaderElector(conn, cfg.LeaderLockID, time.Duration(cfg.LeaderRetryEvery)*time.Second)
	exec := executor.New(execRepo)
	sched := scheduler.New(jobRepo, exec, elector, time.Duration(cfg.PollInterval)*time.Second)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go elector.Run(ctx)
	go sched.Run(ctx)

	handler := api.NewHandler(jobRepo, execRepo, elector, exec)
	router := api.NewRouter(handler)

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
