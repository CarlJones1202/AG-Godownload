# Code Audit Report — AG-Godownload

**Project:** gallery_api (Go backend + React/TypeScript frontend)
**Audit scope:** Entire project — all `.go` files under `/`, plus frontend `src/`
**Priority order:** Bugs (correctness) first, then performance, then security/maintainability.
**Date:** 2026-10-27

---

## Executive Summary

The project is a media gallery management system with crawling/ripping, person matching, vector-based similarity search, and a React frontend. The Go code is generally well-structured with sensible patterns (batched inserts, connection pooling, progress batching, context timeouts on most HTTP calls). However, the audit uncovered **14 distinct bugs** (several critical) and **10 performance issues**, ranging from data-corruption risks (wrong image↔gallery associations) to resource-exhaustion risks (unbounded memory loads, missing HTTP timeouts that can hang indefinitely).

---

## SECTION 1 — BUGS (Priority: Fix Immediately)

### Bug 1 — Image↔Gallery ID mismatch after bulk insert (CRITICAL — data corruption)

**File:** `services/crawler_service.go`, lines 543–587

After bulk-inserting newly crawled images, the code retrieves **all** image IDs for the gallery (not just the ones just inserted):

```go
var insertedIDs []uint
database.DB.Model(&models.Image{}).Where("gallery_id = ?", gallery.ID).Pluck("id", &insertedIDs)
```

It then zips `insertedIDs[i]` with `imagesToInsert[i]` to build `image_galleries` junction rows. If the gallery already contained images from a previous crawl, `insertedIDs` includes pre-existing IDs, so the association loop pairs the wrong image IDs with the new image structs — inserting incorrect `image_galleries` rows and/or missing associations for the genuinely new images.

**Fix:** Track the IDs of only the newly inserted rows (e.g., `RETURNING id` or `LAST_INSERT_ID()` equivalent, or select by the exact set of filenames/download URLs just inserted) before building associations.

---

### Bug 2 — HTTP clients with no timeout can hang indefinitely (CRITICAL — resource exhaustion)

**File:** `services/pornhub_ripper.go`, line 29  
**File:** `services/video_ripper_service.go`, line 319 (`RipTnaFlix`)

```go
client := &http.Client{}   // no Timeout set
```

An `http.Client` with no `Timeout` has no deadline on the entire request. If the remote host never responds (or responds extremely slowly), the goroutine blocks forever. Because rippers are invoked from crawler workers (`worker_service.go`), a single hung site can permanently consume a worker slot and, over time, exhaust all workers.

**Fix:** Use `GetConfiguredHTTPClient()` (which applies `config.Global.HTTPRequestTimeout`) or construct `&http.Client{Timeout: ...}` explicitly in every ripper.

---

### Bug 3 — Pornhub flashvars regex cannot parse nested JSON (HIGH — feature broken on current site layouts)

**File:** `services/pornhub_ripper.go`, line 56

```go
flashvarsPattern := regexp.MustCompile(`var\s+flashvars_\d+\s*=\s*(\{.+?\});`)
```

The `.+?` is non-greedy and stops at the **first** `}`. Pornhub's `flashvars` object contains nested braces (e.g., `mediaDefinitions: [{...}]`), so the regex captures a truncated, invalid JSON fragment, causing `json.Unmarshal` to fail and the rip to error out with "could not find flashvars".

**Fix:** Replace the regex extraction with a balanced-brace scanner, or use a more permissive pattern (`\{.*\}` with `FindStringSubmatch` and validate the JSON parses; fall back to parsing from the first `{` to the last `}` before `;`).

---

### Bug 4 — Most image rippers discard session cookies (HIGH — downloads fail on cookie-gated hosts)

**File:** `services/ripper_service.go`, lines 15–37 (`newCollector`)

`newCollector()` creates a `colly.Collector` **without** a cookie jar. Only `visitPageForCookies()` and `RipImageBam` install one. Every other ripper (`RipImageBox`, `RipImx`, `RipImagetwist`, `RipTurboImg`, etc.) silently drops `Set-Cookie` headers, so hosts that require a session cookie obtained from a prior page visit will reject subsequent requests.

**Fix:** Set a shared `cookiejar` on every collector created in `newCollector()`, or require callers to pass a jar.

---

### Bug 5 — Gallery delete uses deprecated column instead of M2M junction (HIGH — wrong data deleted)

**File:** `handlers/image_handlers.go`, line 358

```go
database.DB.Where("gallery_id = ?", id).Delete(&models.Image{})
```

The `Image.GalleryID` field is marked `Deprecated: use Galleries` in the model. Deleting by `gallery_id` only removes rows that still have the denormalized column set; images associated solely through the `image_galleries` many-to-many table are left orphaned in the DB (and their files may be deleted from disk while their DB rows remain).

**Fix:** Delete via the junction table: delete rows from `image_galleries WHERE gallery_id = ?`, then delete `Image` rows that are no longer associated with any gallery (or use GORM association clearing).

---

### Bug 6 — N+1 queries inside `GetPerson` gallery loop (HIGH — performance + latency)

**File:** `handlers/person_handlers.go`, line 316

```go
for i := range galleries {
    ...
    database.DB.Preload("Source").First(&galleries[i], galleries[i].ID)  // one query per gallery
    ...
    database.DB.Model(&models.Image{}).Where("gallery_id = ?", galleries[i].ID).Count(&count)  // another query per gallery
    ...
    database.DB.Where("gallery_id = ? AND deleted_at IS NULL", galleries[i].ID).Order(...).First(&firstImage)  // another query per gallery
}
```

A person with 20 galleries triggers ~60 extra round-trips. This makes person detail pages slow and adds needless DB load.

**Fix:** Batch-load counts and first images using grouped queries (same pattern already used in `GetGalleries` and `GetPeople`).

---

### Bug 7 — `CleanupDupes` loads every image record into memory (HIGH — OOM risk)

**File:** `handlers/maintenance.go`, lines 29–33

```go
var images []models.Image
if err := database.DB.Find(&images).Error; err != nil { ... }
```

`Find` without pagination/limits materializes the entire `images` table in Go memory. On a library with hundreds of thousands of images this causes an out-of-memory crash or severe GC pressure.

**Fix:** Process in keyset-paginated chunks (same pattern used in `GetFailedImages` / `RetryAllImages`).

---

### Bug 8 — Crawler worker panic kills the worker goroutine permanently (MEDIUM — silent capacity loss)

**File:** `services/worker_service.go`, lines 141–154

```go
for {
    task := CrawlerTaskQueue.Pop()
    if task == nil { return }
    AddActiveCrawlerSource(task.SourceID)
    if err := CrawlSource(task.SourceID); err != nil { ... }
    RemoveActiveCrawlerSource(task.SourceID)
}
```

If `CrawlSource` panics (e.g., nil pointer in a ripper), the worker goroutine dies and is never replaced. The pool silently shrinks until all workers are dead and nothing crawls.

**Fix:** Wrap the worker body in `defer func(){ if r := recover(); r != nil { ... } }()` and log the panic.

---

### Bug 9 — `ClearCookieCache` replaces the `sync.Map` while in-flight readers hold the old instance (MEDIUM — race / stale cookies)

**File:** `services/http_client.go`, line 250

```go
func ClearCookieCache() {
    cookieCache = sync.Map{}   // replaces the whole map
}
```

`GetOrFetchCookies` reads `cookieCache` at call time. If a crawl is in progress and `ClearCookieCache()` is called (e.g., at the start of a new crawl), in-flight goroutines keep the old map instance. New operations get the empty map. This is technically safe (`sync.Map` is safe for concurrent use) but semantically racy: cache invalidation is not atomic with respect to ongoing lookups, so a domain can be re-fetched for cookies mid-crawl.

**Fix:** Use `cookieCache.Range(func(k, v any) bool { cookieCache.Delete(k); return true })` to clear in place, or keep a versioned pointer.

---

### Bug 10 — Privileged maintenance endpoints are open when `MAINTENANCE_TOKEN` is unset (MEDIUM — security)

**File:** `handlers/maintenance.go`, lines 21–26

```go
if config.Global.MaintenanceToken != "" {
    if strings.TrimSpace(token) != strings.TrimSpace(config.Global.MaintenanceToken) {
        c.JSON(http.StatusUnauthorized, ...)
        return
    }
}
```

When `MAINTENANCE_TOKEN` is not configured (the default is `""`), the `if` block is skipped entirely and **every request is authorized** for `/cleanup-dupes`, `/purge-placeholders`, and `/admin/sync-file-exists`. These endpoints can delete DB rows and files.

**Fix:** Default to deny (require a token), or gate these routes behind an admin middleware that always checks a non-empty token.

---

### Bug 11 — `ServeImage` / `ServeThumbnail` path-traversal check is incomplete on Windows (MEDIUM — security)

**File:** `handlers/image_handlers.go`, lines 304–324 and 375–384

The only traversal guard is `strings.Contains(filepathParam, "..")`. While the subsequent `filepath.Abs` prefix check against `UploadsDir` catches most escapes, on Windows alternate-stream syntax (`filename::$DATA`), short names (`~1`), or volume-relative paths could still slip past the naive `".."` check before `filepath.Clean` normalizes.

**Fix:** Rely solely on the resolved `filepath.Abs` prefix check (already present), and reject any path that, after `Clean`, does not have `UploadsDir` as a strict prefix. Remove the redundant `..` string check or keep it as a fast pre-filter only.

---

### Bug 12 — `AddToScanQueue` has a check-then-act race (MEDIUM — duplicate scan jobs)

**File:** `services/scan_queue_service.go`, lines 36–77

`AddToScanQueue` queries for an existing `(person_id, provider)` scan, checks its status, then inserts a new row. Two concurrent calls can both see "no processing scan" and both insert, creating duplicate queue entries and duplicate scan work.

**Fix:** Wrap the check+insert in a transaction with `SELECT ... FOR UPDATE` or add a unique constraint on `(person_id, provider)` where status is pending/processing, and handle the conflict.

---

### Bug 13 — `StoreImageEmbedding` / `VectorIndex` embedder-name drift (MEDIUM — similarity results silently wrong)

**File:** `services/embedding_service.go`, lines 133–172 and `services/vector_index.go`, lines 50–93

`StoreImageEmbedding` stores `CurrentEmbedder().Name()` in the DB row, while `MemoryIndex.Rebuild()` loads rows filtered by `CurrentEmbedder().Name()`. If the embedder changes at runtime (e.g., config reload, or `CurrentEmbedder()` lazily constructs a different `LowLevelEmbedder` in `embedding_service.go:123–128`), the in-memory index and the DB rows can disagree about which embedder produced which vector, producing incorrect similarity results or missing vectors.

**Fix:** Pass the embedder name explicitly rather than calling `CurrentEmbedder()` in two places; make `activeEmbedder` immutable after `InitEmbedding`.

---

### Bug 14 — `downloadImageWithCookies` validates image after a failed decode may have already removed the file (LOW — confusing error paths)

**File:** `services/image_service.go`, lines 271–296

When `imaging.Open(destPath)` fails, the file is removed at line 274 and an error is returned — so execution never reaches the placeholder check at line 290. This is not incorrect, but the placeholder check (`IsHotlinkPlaceholder`) re-opens and re-hashes the file, duplicating the work already done during download validation.

**Fix:** Minor: perform placeholder detection before full decode, or reuse the already-computed hash from download.

---

### Bug 15 — `println` used instead of structured logger in `DeleteImage` (LOW — observability)

**File:** `handlers/image_handlers.go`, lines 523, 530, 538, 545, 560, 566

`println("Warning: ...")` bypasses the structured logger, so these warnings do not respect log level, lack timestamps/prefixes, and are not captured by any log aggregation.

**Fix:** Replace with `logger.Warnf`.

---

## SECTION 2 — PERFORMANCE ISSUES

### Perf 1 — Brute-force O(n) vector similarity search (HIGH impact)

**File:** `services/vector_index.go`, lines 121–155

```go
func (m *MemoryIndex) SearchByVector(...) {
    m.mu.RLock()
    results := make([]ScoredImage, 0, len(m.vecs))
    for id, v := range m.vecs {
        ...
        sim := dotProduct(query, v)
        ...
    }
    m.mu.RUnlock()
    sort.Slice(results, ...)
}
```

Every similarity query computes the dot product against **every vector in memory**, then sorts the full result set. For a library with tens of thousands of images, every `/similar` or recommendation request is O(n) and holds a read lock for the entire scan.

**Fix:** Swap in an ANN index (HNSW, IVF, or pgvector with an index on PostgreSQL). At minimum, cap candidate sets before sorting and consider pre-filtering by tags/source.

---

### Perf 2 — `GetImages` preloads four relations per image (HIGH impact)

**File:** `handlers/image_handlers.go`, line 253

```go
query.Preload("Galleries.Source").Preload("Source").Preload("People").Preload("Tags")
```

With the default `limit=100`, each page load fetches galleries+sources+people+tags for every image. This multiplies result-set size and causes redundant row reads.

**Fix:** Only preload relations the current view actually needs. For list views, fetch image fields first, then batch-load relations (galleries, tags) in separate indexed queries keyed by the returned image IDs.

---

### Perf 3 — `GetPeople` performs multiple sequential batch queries plus nested per-person subqueries (HIGH impact)

**File:** `handlers/person_handlers.go`, lines 107–252

Gallery counts, video counts, profile images, tagged-image thumbnails, and gallery-fallback thumbnails are each separate queries. The video-count query is a raw SQL with `UNION` and `IN ?` executed once (fine), but the overall handler does ~5–6 round trips and the `GetPerson` detail handler has the N+1 noted in Bug 6.

**Fix:** Combine counts into a single grouped query; continue batching; eliminate the per-gallery loop queries (see Bug 6 fix).

---

### Perf 4 — Crawler writes progress to DB every ≤2 seconds per active source (MEDIUM impact)

**File:** `services/crawler_service.go`, lines 215–308

The progress-flush goroutine opens a transaction and issues one `UPDATE sources ... WHERE id = ?` per active source every 2 seconds (or when progress changes by ≥2%). With many concurrent crawls this is a steady stream of write transactions and WAL churn.

**Fix:** Increase the flush interval (e.g., 10 s) or only persist on completion and on significant percentage changes. The UI does not need sub-second crawl progress fidelity.

---

### Perf 5 — Daily scan cron re-queues **all** people with aliases every day regardless of change (MEDIUM impact)

**File:** `services/scan_queue_service.go`, lines 224–239

`StartDailyScanScheduler` calls `AddAllPeopleToScanQueue()` at 3 AM unconditionally. Every person with a provider alias is re-scanned daily even if provider data has not changed, wasting network, CPU, and DB writes.

**Fix:** Track `last_scanned_at` per `(person, provider)` and only re-scan after a configurable interval (e.g., weekly), or when the provider's known gallery set is believed to have changed.

---

### Perf 6 — `GetFailedImages` / `RetryAllImages` `os.Stat` every file in the library (MEDIUM impact)

**File:** `handlers/image_handlers.go`, lines 591–607 and 638–664

Both handlers paginate through **all** images and call `os.Stat` on each one's filesystem path. For large libraries this is a full-disk scan per request, adding latency and disk IOPS.

**Fix:** Rely on the `file_exists` column (kept in sync by `SyncFileExists`) instead of stat-ing every file on demand; add an endpoint to bulk-reconcile only rows where `file_exists = true` but the record is old.

---

### Perf 7 — `SyncFileExists` stats every file on disk (MEDIUM impact)

**File:** `services/file_exists_service.go`, lines 30–53

Same pattern: pages through all images and calls `os.Stat` per row. Runs at startup (see `main.go:44`) and on demand. For 100k+ images this is a noticeable startup delay and disk load.

**Fix:** This is largely unavoidable for a full sync, but it should run with lower priority, be rate-limited, and optionally skip videos (which are large and expensive to stat on network filesystems).

---

### Perf 8 — Thumbnail generation re-decodes and re-encodes every downloaded image (MEDIUM impact)

**File:** `services/image_service.go`, lines 311–341 (`GenerateThumbnail`)

Every image downloaded by the crawler is opened with `imaging.Open`, resized with Lanczos, and re-saved as JPEG quality 80 — even when the source is already a JPEG and already small. This is CPU-intensive for every crawl.

**Fix:** Skip re-encoding when the source image is already ≤ target dimensions and already JPEG; just copy the file or generate a thumbnail only on demand (lazy thumbnailing).

---

### Perf 9 — Bulk SQL built by string concatenation (MEDIUM impact)

**File:** `services/crawler_service.go`, lines 540–551

```go
placeholders = append(placeholders, "(?, ?, ?, ?, ?, ?)")
...
stmt := fmt.Sprintf("INSERT INTO images ... VALUES %s", strings.Join(placeholders, ","))
```

For very large crawl batches (hundreds of images), the generated SQL string can become very large. While PostgreSQL handles long statements, this is less efficient than `exec.CopyFrom` or `CreateInBatches` with proper batching, and it prevents the driver from reusing a prepared statement.

**Fix:** Use `database.DB.CreateInBatches(imagesWithoutAssoc, 100)` or `gorm`'s batch insert, or `pq.CopyFrom` for PostgreSQL.

---

### Perf 10 — Connection pool may be undersized for concurrent crawls (MEDIUM impact)

**File:** `database/db.go`, lines 59–60

```go
sqlDB.SetMaxOpenConns(25)
sqlDB.SetMaxIdleConns(5)
```

With `CrawlerWorkers` (default 5) plus verification workers, embedding workers, and HTTP requests, 25 open connections is workable but can become a bottleneck under concurrent crawling + similarity search + UI traffic on PostgreSQL.

**Fix:** Size `MaxOpenConns` based on `CrawlerWorkers + verification + embedding + HTTP headroom` (e.g., `config.Global.CrawlerWorkers * 2 + 20`), and monitor for `waiting on connection` errors.

---

## SECTION 3 — SECURITY / MAINTAINABILITY (Lower Priority)

| # | Issue | File | Recommendation |
|---|-------|------|----------------|
| S1 | `PornhubMediaDefinition.Quality` parsed with `strconv.Atoi` ignoring error (line 134) | `pornhub_ripper.go` | Check the error; unparseable quality silently becomes 0, which may pick the wrong "best" quality. |
| S2 | `video_ripper_service.go` builds `ffmpeg`/`ffprobe` commands from file paths without sanitization | `image_service.go`, `video_ripper_service.go` | Paths come from user-controlled sources; ensure they cannot inject shell args (they're passed as `exec.Command` args, so injection risk is low, but validate paths). |
| S3 | `LoadCookies` sets cookies for hardcoded YouTube domains regardless of the cookie file's actual domain | `video_ripper_service.go:1262–1274` | Cookies for `xhamster_cookies.txt` get applied to YouTube domains too — harmless but incorrect; set cookies only for the domain in the file. |
| S4 | `database.Checkpoint()` is a no-op for PostgreSQL but called at crawl end | `crawler_service.go:622` | Harmless but misleading; remove or document. |
| S5 | Frontend `api.ts` uses `/api` base with Vite proxy rewriting to backend root | `frontend/src/lib/api.ts` + `vite.config.ts` | Works in dev via proxy; production builds need the same reverse-prefix mapping or the API base must be configurable. |
| S6 | No rate limiting on crawl/download/retry endpoints | `main.go` routes | A malicious or buggy client can trigger unlimited concurrent crawls/downloads. Add per-IP or token-based rate limiting. |
| S7 | `main.go` startup goroutine runs many heavy services without waiting; HTTP server may accept requests before startup tasks finish | `main.go:42–95` | Acceptable for background tasks, but `VerifyDownloadedImages`, `ScanMissingMetadata`, etc. can compete with live traffic for DB connections at startup. |

---

## SECTION 4 — CODE HEALTH OBSERVATIONS (Positive)

The codebase has several good practices worth preserving:
- Connection pooling with sensible `SetMaxOpenConns`/`SetMaxIdleConns` (`database/db.go`)
- Retry logic with exponential backoff and `Retry-After` handling (`http_client.go`)
- Batched progress updates to reduce DB contention (`crawler_service.go`)
- Batched image counts and first-image lookups (`gallery_handlers.go`, `person_handlers.go`)
- Chunked 8 MB ranged video downloads with fallback (`video_ripper_service.go`)
- Proper graceful shutdown (workers stop, HTTP drains, DB closes) (`main.go`)
- Use of `sync.Map`, `sync.Cond`, `container/heap` for task queues (`worker_service.go`, `scan_queue_service.go`)

---

## SECTION 5 — RECOMMENDED FIX ORDER

1. **Bug 1** (image↔gallery mismatch) — data integrity
2. **Bug 2** (no HTTP timeouts) — can hang workers forever
3. **Bug 5** (gallery delete wrong column) — data loss/orphans
4. **Bug 4** (missing cookie jars) — rippers silently fail
5. **Bug 3** (pornhub regex) — feature broken
6. **Bug 7** (CleanupDupes OOM) — operational stability
7. **Bug 6** (N+1 in GetPerson) — page latency
8. **Perf 1** (brute-force vector search) — similarity page latency at scale
9. **Perf 2** (GetImages over-preloading) — list page latency
10. **Bug 10** (maintenance token default-allow) — security

---

## Appendix — Files Reviewed

All `.go` files under `cmd/`, `config/`, `database/`, `handlers/`, `logger/`, `models/`, `services/`, plus `main.go`. Frontend: `frontend/src/**/*.{ts,tsx}`. Key files examined in depth:

- `main.go`, `config/config.go`, `database/db.go`, `logger/logger.go`
- `models/image.go`, `models/gallery.go`, `models/person.go`, `models/source.go`
- `handlers/gallery_handlers.go`, `handlers/image_handlers.go`, `handlers/person_handlers.go`, `handlers/source_handlers.go`, `handlers/maintenance.go`, `handlers/stats_handlers.go`
- `services/crawler_service.go`, `services/ripper_service.go`, `services/pornhub_ripper.go`, `services/image_service.go`, `services/video_service.go`, `services/video_ripper_service.go`, `services/http_client.go`, `services/worker_service.go`, `services/scan_queue_service.go`, `services/embedding_service.go`, `services/vector_index.go`, `services/download_status_service.go`, `services/purge_service.go`, `services/file_exists_service.go`, `services/recover_service.go`
- `frontend/src/lib/api.ts`, `frontend/vite.config.ts`, `frontend/src/App.tsx`
