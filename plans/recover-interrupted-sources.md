# Recover interrupted sources

Status: Completed (2026-11-17)

## Problem and outcome

Interrupted crawls can leave a gallery with fewer saved photos than the source
post contains. Add a command that compares those counts and asks the running
server to crawl mismatched sources, for one gallery or all eligible galleries.

## Scope and acceptance criteria

- Inspect one gallery by ID or all galleries associated with an HTTP(S) source.
- Compare saved image records with image links selected by the crawler from the
  source post.
- Ask the running server to queue a source only when counts differ; do not
  duplicate sources already queued or crawling.
- Process batch requests sequentially, skip galleries without crawlable sources,
  and continue after individual source errors while reporting them.
- Document usage and test selection, counting, and batch behavior.

## Implementation and constraints

Use the existing configured HTTP client and crawler selection logic, plus the
running server's crawl endpoint. Keep batch requests sequential and do not
download files or alter gallery records.

## Validation

`go test ./services ./handlers ./cmd/recover-source` passed (2026-11-17),
including live-endpoint request handling, duplicate-pending protection, and
batch coverage for skipped galleries and continued processing after a failed
source.
