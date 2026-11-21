# Database import recovery

Status: Completed (2026-11-20)

## Problem and intended outcome

A successful database restore must treat its uploaded database as a complete
catalog whose media may be absent locally. It should automatically run the same
missing-file recovery process used at startup, rather than leaving restored
rows with stale `file_exists` state or waiting for a manual maintenance action.

## Scope and non-goals

- Start recovery only after PostgreSQL restore, reconnect, and migrations finish
  successfully.
- Resync image `file_exists` state, remove duplicate image rows, and start image,
  video, and person-image missing-file verification.
- Protect against duplicate recovery launches while one import recovery is
  already running.
- Do not alter failed-import behavior or start recovery after a restore error.
- Do not add a new download implementation or duplicate existing startup logic.

## Acceptance criteria

- A successful `/import/db` request starts the missing-file recovery process.
- The process runs disk synchronization before missing-image and missing-video
  collection.
- Import recovery continues with verification even if duplicate removal or an
  individual verifier returns an error.
- Repeated import recovery calls are coalesced.
- Focused tests cover recovery launch/guard behavior and import integration.

## Implementation steps

1. Add a shared service entry point for missing-file recovery.
2. Replace the startup missing-file setup with the shared entry point.
3. Invoke recovery after a successful database import and migration.
4. Add focused tests and run relevant Go tests.

## Risks and constraints

- Restoration replaces tables while HTTP handlers may still run, so recovery
  starts asynchronously after the database is reconnected.
- `VerifyDownloadedImages`, `VerifyDownloadedVideos`, and `VerifyPersonImages`
  already launch their own background work and return immediately.

## Validation

- `go test ./services ./handlers` passed (2026-11-20).
