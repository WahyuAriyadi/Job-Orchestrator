package scheduler

import (
	"context"
	"database/sql"
	"log"
	"sync/atomic"
	"time"
)

// LeaderElector decides which of potentially many running instances of this
// service is allowed to actually dispatch jobs. Only one instance should
// ever run the scheduler loop at a time, or every due job would fire once
// per instance.
//
// How it works:
//   - It holds ONE dedicated *sql.Conn (not the shared pool) for its entire
//     lifetime, because pg_advisory_lock is tied to the session/connection
//     that took it. If that connection is lost (crash, network partition,
//     process killed), Postgres automatically releases the lock — so
//     failover is automatic with no heartbeat/TTL bookkeeping needed.
//   - Non-leaders poll pg_try_advisory_lock (non-blocking) every
//     retryInterval, trying to become leader.
//   - The current leader periodically pings its held connection; if the
//     ping fails, it demotes itself and starts retrying like everyone else.
type LeaderElector struct {
	db            *sql.DB
	lockID        int64
	retryInterval time.Duration

	heldConn *sql.Conn
	isLeader atomic.Bool
}

func NewLeaderElector(db *sql.DB, lockID int64, retryInterval time.Duration) *LeaderElector {
	return &LeaderElector{db: db, lockID: lockID, retryInterval: retryInterval}
}

func (le *LeaderElector) IsLeader() bool {
	return le.isLeader.Load()
}

// Run blocks until ctx is cancelled, continuously attempting to acquire (or
// keep) leadership. Call it in its own goroutine.
func (le *LeaderElector) Run(ctx context.Context) {
	ticker := time.NewTicker(le.retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			le.release()
			return
		case <-ticker.C:
			if le.isLeader.Load() {
				le.checkHeldLock(ctx)
			} else {
				le.tryAcquire(ctx)
			}
		}
	}
}

func (le *LeaderElector) tryAcquire(ctx context.Context) {
	conn, err := le.db.Conn(ctx)
	if err != nil {
		log.Printf("[leader] failed to obtain connection: %v", err)
		return
	}

	var acquired bool
	err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, le.lockID).Scan(&acquired)
	if err != nil {
		log.Printf("[leader] lock attempt failed: %v", err)
		conn.Close()
		return
	}

	if !acquired {
		conn.Close()
		return
	}

	le.heldConn = conn
	le.isLeader.Store(true)
	log.Printf("[leader] acquired leadership (lock id %d)", le.lockID)
}

// checkHeldLock verifies the leader's connection (and therefore its lock)
// is still alive. A failed ping means the session is gone and, with it,
// the advisory lock — so we must demote immediately rather than keep
// believing we're the leader.
func (le *LeaderElector) checkHeldLock(ctx context.Context) {
	if le.heldConn == nil {
		le.isLeader.Store(false)
		return
	}
	if err := le.heldConn.PingContext(ctx); err != nil {
		log.Printf("[leader] lost connection, demoting: %v", err)
		le.heldConn.Close()
		le.heldConn = nil
		le.isLeader.Store(false)
	}
}

func (le *LeaderElector) release() {
	if le.heldConn == nil {
		return
	}
	_, _ = le.heldConn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, le.lockID)
	le.heldConn.Close()
	le.heldConn = nil
	le.isLeader.Store(false)
	log.Printf("[leader] released leadership")
}
