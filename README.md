# orchestratord

A mini Cron-as-a-Service, built to answer one question that always seems to
get skipped in tutorials: **what happens to your scheduled jobs when you
scale past one instance?**

Register a job with a cron schedule and a callback URL, and this service
will call that URL right on time — retrying failed calls with backoff,
keeping a full history of every attempt, and never double-firing a job even
if you're running three copies of the service behind a load balancer.

## Why I built this

Most "build a scheduler" tutorials stop at a single process with an
in-memory cron loop. That works fine until you deploy it — the moment you
run more than one replica for redundancy, every replica ends up running its
own scheduler, and every due job fires once per replica. I wanted to
actually solve that problem rather than wave it away, so this project is
built around one core idea: **only one instance is ever allowed to dispatch
jobs at a time**, and the rest sit in standby, ready to take over the
instant the active one disappears.

That's leader election, and it's usually the kind of thing you read about
in a system design interview, not something you actually implement. So I
implemented it — using nothing more exotic than a PostgreSQL advisory lock,
because I didn't want to bolt on etcd or Zookeeper just to coordinate a
handful of processes that already talk to the same database anyway.

## How it actually works

Every running instance of `orchestratord` executes the exact same code —
there's no special "primary" build. On startup, each instance tries to grab
`pg_advisory_lock` on a fixed key. Whoever gets it becomes the leader and
starts polling for due jobs; everyone else just... waits, checking every few
seconds whether the lock has become available.

The nice part is what happens on failure. Advisory locks in Postgres are
tied to the database *session* that took them, not to some TTL you have to
babysit. If the leader's process dies — crash, OOM kill, someone unplugging
a Cloud Run instance — its database connection drops, and Postgres releases
the lock the instant that happens. No heartbeat protocol, no stale-lock
cleanup job, no 30-second window where nothing is scheduled because a lease
hasn't expired yet. A standby picks it up on its next poll.

```
┌───────────────┐    HTTP     ┌───────────────────┐
│ Vue dashboard  │────────────▶│     REST API       │
└───────────────┘             │ (net/http, no       │
                               │  framework)          │
                               └─────────┬───────────┘
                                         │
                  ┌──────────────────────┼──────────────────────┐
                  │                      │                       │
          ┌───────▼───────┐    ┌─────────▼────────┐    ┌─────────▼────────┐
          │ JobRepository  │    │  LeaderElector    │    │   Scheduler       │
          │ (CRUD + "what's│    │  (pg advisory     │    │   (poll loop —    │
          │  due" queries) │    │   lock)           │    │   only acts if    │
          └───────┬────────┘    └─────────┬─────────┘    │   it's the leader)│
                  │                       │              └─────────┬────────┘
                  └───────────┬───────────┴────────────────────────┘
                              │
                       ┌──────▼──────┐          ┌───────────────┐
                       │  PostgreSQL │          │    Executor     │──▶ your callback URL
                       │  (jobs +    │          │  (HTTP call +   │    (retries w/ backoff)
                       │  executions)│          │   backoff)      │
                       └─────────────┘          └───────────────┘
```

## What's under the hood

- **Go**, stdlib `net/http` only — Go 1.22's method-based routing
  (`"POST /api/jobs"`) meant I genuinely didn't need a router library.
- **PostgreSQL**, doing double duty as both the data store and the
  coordination primitive for leader election.
- **Vue 3** for the dashboard, loaded straight from a CDN — no build step,
  no `node_modules`, just plain HTML/CSS/JS you can open and edit directly.
- The cron parser, the retry/backoff logic, and the leader election are all
  written by hand — no external cron library, no job-queue framework. Partly
  because the whole point of this project was to actually understand those
  mechanisms, not just import them, and partly because when someone in an
  interview asks "okay, but how does that actually work," I wanted a real
  answer.

The only third-party dependency in the whole codebase is the Postgres
driver (`lib/pq`).

## Getting it running

```bash
# spin up Postgres
docker compose up -d
make migrate

# run the API + scheduler
cp .env.example .env && export $(cat .env | xargs)
make run
# → listening on :8080

# in another terminal, serve the dashboard
make dashboard
# → open http://localhost:5173
```

No Docker? Point it at any Postgres you've already got:

```bash
psql "$DATABASE_URL" -f migrations/001_init.sql
DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=disable" go run ./cmd/server
```

### Seeing the leader election actually work

This is the part I'd actually demo in an interview:

```bash
HTTP_PORT=8080 go run ./cmd/server   # terminal 1 — grabs leadership
HTTP_PORT=8081 go run ./cmd/server   # terminal 2 — sits in standby

curl localhost:8080/api/health   # {"is_leader": true,  ...}
curl localhost:8081/api/health   # {"is_leader": false, ...}
```

Now kill terminal 1 (Ctrl+C) and hit `/api/health` on :8081 a few seconds
later. It'll have taken over — no manual failover, no restart needed.

## API

| Method | Path                        | What it does                                  |
|--------|-----------------------------|------------------------------------------------|
| POST   | `/api/jobs`                 | Create a job                                    |
| GET    | `/api/jobs`                 | List all jobs                                   |
| GET    | `/api/jobs/{id}`             | Get a single job                                |
| PUT    | `/api/jobs/{id}`             | Update a job (partial updates supported)        |
| DELETE | `/api/jobs/{id}`             | Delete a job                                    |
| POST   | `/api/jobs/{id}/trigger`     | Run a job right now, outside its normal schedule|
| GET    | `/api/jobs/{id}/executions`  | See the execution history for a job             |
| GET    | `/api/health`                | Health check + whether this instance is leader  |

Creating a job looks like this:

```bash
curl -X POST localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "daily-stock-check",
    "cron_expression": "0 8 * * *",
    "callback_url": "https://api.example.com/webhooks/stock-check",
    "payload": {"warehouse": "main"},
    "max_retries": 3,
    "timeout_seconds": 30
  }'
```

### Cron syntax

Standard 5-field format — `minute hour day-of-month month day-of-week`
(Sunday = 0). Supports `*`, single values, comma lists (`1,15,30`), ranges
(`1-5`), and steps (`*/15`).

| Expression     | Runs                              |
|-----------------|-------------------------------------|
| `* * * * *`     | every minute                        |
| `*/15 * * * *`  | every 15 minutes                    |
| `0 9 * * *`     | daily at 9am                        |
| `0 9 * * 1-5`   | 9am, Monday through Friday          |
| `0 0 1 * *`     | midnight on the 1st of every month  |

## Project layout

```
cmd/server/main.go       wiring: config, db, leader elector, scheduler, HTTP server
internal/config/         env-var configuration
internal/models/         Job, Execution, request/response types
internal/db/              pooled Postgres connection
internal/repository/     SQL queries — jobs and executions
internal/cronparser/     hand-written cron parser, with tests
internal/scheduler/      leader election + the poll-and-dispatch loop
internal/executor/       HTTP callback execution, retries, backoff
internal/api/             HTTP handlers and routing
migrations/001_init.sql  schema
web/index.html            dashboard markup
web/style.css             dashboard styling
web/app.js                dashboard logic (Vue 3, no build step)
```

## Trade-offs I made on purpose

Nothing here is accidental, but a few things are worth calling out because
they were deliberate choices rather than the "obvious" answer:

**This is at-least-once execution, not exactly-once.** If the leader
advances a job's `next_run_at` and then crashes before the HTTP callback
actually completes, that run is gone until the next scheduled time. Getting
to exactly-once would mean adding a per-job in-flight lock and some kind of
outbox pattern — doable, but it's real complexity for a problem most
callback consumers can solve on their own by making their endpoint
idempotent. I'd rather be upfront about this than pretend it doesn't exist.

**Polling, not a priority queue or timer wheel.** The scheduler checks "what's
due" every few seconds rather than maintaining an in-memory heap of next-run
times. It's simpler to reason about, and at the scale this is meant for, the
latency (bounded by the poll interval) is a non-issue. If I needed
near-real-time dispatch, I'd look at `LISTEN`/`NOTIFY` instead of tightening
the poll interval into a busy-loop.

**Fire-and-forget dispatch.** When a job is due, the scheduler spawns a
goroutine and moves straight on to the next one — it doesn't wait for the
callback to finish before checking what else is due. One slow endpoint
shouldn't be able to stall every other job's schedule.
