# Optimization Status

This document tracks optimization work across the project. Statuses were verified
against the code on 2026-11-20; historical estimates and duplicate roadmaps were
removed because the work itself is the useful record.

## Status

| # | Item | Status | Notes |
|---|---|---|---|
| 1 | Database indexing | Complete | Indexes on hot columns in `models/*.go`. |
| 2 | Bounded download concurrency | Complete | Semaphore in `services/crawler_service.go`. |
| 3 | HTTP connection pooling | Complete | Shared clients in `services/http_client.go`. |
| 4 | Batch database writes | Complete | `CreateInBatches` in `services/crawler_service.go`; some junction inserts are still built individually. |
| 5 | Configurable worker pools | Complete | `CRAWLER_WORKERS`, `AITAG_WORKERS`, `PERSON_SCAN_WORKERS`, and recovery pools in `config/config.go`. |
| 6 | Context support and cancellation | Not started | No end-to-end cancellation plumbing through services. |
| 7 | Structured logging | Partial | `logger/logger.go` provides leveled, prefixed text logs; no structured fields. |
| 8 | Explicit error classification | Not started | No `errors/errors.go`; errors are wrapped ad hoc. |
| 9 | Graceful shutdown | Complete | Signal handling, worker drain, and queued-crawl requeue in `main.go` / `services/shutdown.go`. |
| 10 | Health and metrics endpoints | Partial | `/api/stats` exists; no dedicated `/health` or `/metrics` route. |
| 11 | Streaming downloads | Partial | Downloads stream with `io.Copy`; thumbnail generation still decodes/re-encodes in memory. |
| 12 | Rate limiting and circuit breakers | Not started | Retries exist, but no host-level throttling or circuit breaker. |
| 13 | Retry with exponential backoff | Complete | `services/http_client.go` honors `Retry-After` and backs off on retries. |
| 14 | Request middleware | Partial | Gin logger/recovery and a server read timeout exist; no custom timeout/compression middleware. |
| 15 | Resumable video downloads | Not started | Large video downloads restart from scratch. |

## Remaining work, in suggested order

1. **Context and cancellation (6)** — thread `context.Context` through download and
   crawl services so shutdown and client disconnects can stop in-flight work.
2. **Error classification (8)** — introduce typed errors so retries can distinguish
   transient network failures from permanent 404/403 responses.
3. **Health endpoint (10)** — add a lightweight `/health` probe separate from the
   stats/metrics routes.
4. **Request middleware (14)** — add explicit per-route timeouts; note that uploads and
   crawls can be long-running, so avoid a global write timeout.
5. **Rate limiting and circuit breakers (12)** — protect hosts that trigger bans, using
   the shared HTTP client so all rippers benefit.
6. **Structured logging (7)** — add key/value fields while keeping existing log output
   readable.
7. **Streaming thumbnails and resumable video downloads (11, 15)** — larger changes with
   the most implementation risk.

## Related notes

- Slow SQL warnings should be resolved rather than ignored; see `AGENTS.md`.
- Perf-sensitive changes should be measured before and after, using existing benchmarks
  in `services/` and `database/` where possible.
- `services/crawler_service.go` still has a few N+1-shaped code paths and a progress
  ticker that could be tuned, but these are maintenance items rather than committed
  roadmap work.
