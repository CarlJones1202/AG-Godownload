# Fresh-Install Recovery Performance

**Status:** Completed (2026-11-17)

## Problem

On a new machine (restored database, empty `uploads/`), startup recovery takes far
longer than the raw network transfer of hundreds of GB requires. Time is lost
before, during, and after the actual downloads:

- Startup runs two full serial `os.Stat` sweeps over every image row
  (`SyncFileExists`, then `VerifyDownloadedImages` Phase 1), plus serial
  provider-thumbnail validation/re-download passes, before bulk downloads begin.
- `ScanMissingMetadata` runs 2 queries per video for every video in the table and
  falls back to a full `filepath.Walk(UploadsDir)` per missing video; it blocks
  video recovery from starting.
- Recovery issues one `UPDATE` per missing row while scanning, per-row source-name
  queries (N+1) in video recovery, and a per-row `fmt.Printf` (slow on Windows).
- Download workers synchronously run CPU-heavy post-processing (k-means color
  extraction, Lanczos thumbnail resize, ffmpeg video thumbnail + trickplay
  sprite), starving download slots — trickplay alone can block a slot for minutes.
- Concurrency is hardcoded to 10 per pool and ignores configuration.
- Image recovery also attempts `type='video'` rows: it downloads the entire video
  body, then discards it when image decoding fails — every video is fetched twice
  on a fresh install.
- Every recovered image immediately enqueues embedding work (Python inference),
  competing with downloads for CPU/DB from minute one.

## Intended outcome

- Bulk image and video downloads start within seconds of process start instead of
  after serial thumbnail/metadata passes.
- Scan phases cost O(chunks) SQL instead of O(rows) syscalls + O(rows) UPDATEs.
- Download workers spend their time downloading; color/thumbnail/trickplay work
  runs on a separate bounded pool.
- Concurrency is tunable via environment variables.
- Videos are only fetched by the video pipeline.

## Scope

1. Reorder startup so `SyncFileExists` → `RemoveDuplicateImages` → bulk recovery
   start first; `ScanMissingMetadata` and provider-thumbnail work run afterwards,
   alongside recovery.
2. Fresh-install fast path in `SyncFileExists`: when `UploadsDir` is missing or
   empty, one `UPDATE … SET file_exists = false` replaces the per-row stat sweep.
   Parallelize (64-wide) the stat sweep for the normal path.
3. `VerifyDownloadedImages` Phase 1: stop re-statting; select
   `file_exists = false` rows (column just synced), excluding `type='video'`
   (handled by the video pipeline). Batch the `file_exists` bookkeeping away
   entirely (rows are already `false`).
4. `VerifyDownloadedVideos` Phase 1: select `file_exists = false` rows; batch
   source-name resolution (shared helper: direct `source_id` first, gallery
   fallback); aggregate the per-row console output; keep the present-row thumbnail
   regeneration as a separate pass that runs alongside downloads.
5. Post-processing pools (configurable): image pool computes dominant colors +
   thumbnail after download; video pool runs video thumbnail + trickplay after
   download. DB result updates flow through the image pool (colors are needed for
   the update); video DB updates are sent immediately.
6. `RECOVERY_CONCURRENCY` (default 10, current behavior) replaces hardcoded pool
   sizes; `RECOVERY_POSTPROCESS_WORKERS` (default 4) sizes the post pools.
7. `ScanMissingMetadata`: one lazily-built basename→path index of `UploadsDir`
   replaces the per-video recursive walk; batched source-name resolution per
   chunk.
8. Defer embedding enqueue during image recovery: call
   `EnqueueMissingEmbeddings()` once after recovery completes instead of per row.
9. Parallelize `MigrateMissingProviderThumbnails` and
   `ValidateProviderThumbnails` local stat/decode work and bound their network
   retries with small semaphores (4 scrape / 8 download).
10. `RemoveDuplicateImages`: compute on-disk existence once per image instead of
    `os.Stat` inside the sort comparator.

### Intentional behavior changes

- Image recovery no longer attempts `type='video'` rows (previously downloaded in
  full and then discarded by image decoding — pure waste; the video pipeline
  recovers them).
- Video recovery source naming now prefers the direct `source_id` and falls back
  to the linked gallery (previously gallery-only), matching `RecoverImage` and
  `ScanMissingMetadata`. Fresh-install video re-downloads land in the correct
  source directory instead of `uncategorized`.
- Verification progress "total/processed" for images now covers non-video rows
  (videos have their own progress section); missing counts are no longer
  double-reported by both sections.
- Per-missing-video console lines become one aggregate log line.
- `ScanMissingMetadata` no longer detects files moved into `UploadsDir` *while*
  the scan runs (index is a point-in-time snapshot); files present when the scan
  starts are found as before. Re-downloaded videos get metadata from the recovery
  pipeline itself.
- Recovery download helpers gained a no-post-processing variant
  (`DownloadVideoWithoutPostProcessing`, `importLocalVideo(postProcess)`,
  `DownloadImageWithGalleryDLWithoutPostProcessing`) so the recovery pools never
  run ffmpeg/k-means inline. All non-recovery callers keep the previous inline
  behavior via the unchanged exported functions.
- Pre-existing bug fix required to pass the `go vet ./...` validation gate:
  `ProcessVideoSource` (PMVHaven branches) shadowed `localPath` with `:=` and
  assigned it to itself, so the ripped file path was lost before
  `ImportLocalVideo`. The three branches now assign the outer variable.

### Non-goals

- Changing prioritization order (favorites → favorite galleries → alternating) or
  video smallest-first ordering.
- Resumable/partial download state across restarts.
- Paginating the provider-thumbnail gallery loads (memory, not scan time).
- Rate-limit/sleep policy for slow downloads (kept as-is, network politeness).
- Frontend work.

## Acceptance criteria

- On an empty `uploads/`, `SyncFileExists` performs zero per-row stats (single
  UPDATE) and marks all rows `file_exists = false`.
- Image Phase 1 collection performs ≤ `ceil(rows/1000)+1` queries, no
  `os.Stat`, no per-row UPDATEs, and never yields video tasks.
- Video Phase 1 collection performs ≤ `ceil(rows/500)+3` queries regardless of
  row count (no per-row source queries).
- Download workers do not call `GenerateThumbnail`,
  `ExtractDominantColors`, `GenerateVideoThumbnail`, or
  `GenerateTrickplayData` inline.
- Pool sizes come from config; defaults preserve current behavior (10).
- Startup order starts both bulk recoveries before
  `ScanMissingMetadata` / provider-thumbnail work.
- Existing tests pass; new focused tests cover fast path, collection semantics,
  batched source resolution, and the uploads file index.

## Implementation steps

0. Make `services.UploadsDir` a `var` (identical default `"uploads"`) so tests
   and benchmarks can redirect it to a temp directory; record baseline
   `SyncFileExists` benchmark numbers before further changes.
2. `config`: add `RecoveryConcurrency`, `RecoveryPostprocessWorkers`.
3. `download_status_service`: add `AddVerificationProcessed(n)`.
4. `file_exists_service`: fast path + parallel stat sweep.
5. `verification_service`: extracted `collectMissingImageTasks`, gallery-relation
   helper, post-processing pool, config concurrency, deferred embedding enqueue,
   end-of-recovery `EnqueueMissingEmbeddings()`.
6. `video_verification_service`: split collection/thumb passes, shared batched
   `resolveVideoSourceNames`, post-processing pool, config concurrency.
7. `image_service`: `DownloadImageWithoutColors` (skip k-means at download time),
   `ScanMissingMetadata` index + batching.
8. `migration_service`: parallel stat/decode + bounded network in thumbnail
   migration/validation.
9. `verification_service` `RemoveDuplicateImages`: precompute existence.
10. `main.go`: startup reorder.
11. Tests: fast path, normal path, image collection (incl. video exclusion),
    video collection (incl. batched source names, skip rules), query-count
    bounds, uploads file index.
12. Run benchmarks (after), `go vet ./...`, focused + full test suites; record.

**All steps completed 2026-11-17.** Steps 6–8 additionally required the
recovery-side no-post-processing download variants listed under intentional
behavior changes (the inline ffmpeg/k-means calls lived inside `DownloadVideo`
and the gallery-dl fallback, not only in the worker bodies).

## Risks / constraints

- `VerifyDownloadedImages`/`VerifyDownloadedVideos` rely on `file_exists` being
  fresh; they are only invoked at startup immediately after `SyncFileExists`
  (verified: only caller is `main.go`). Documented in code comments.
- Post pools add bounded goroutines (default 4) running ffmpeg/k-means; CPU
  contention with downloads is reduced, not eliminated.
- Concurrent gallery thumbnail saves use the shared GORM pool (25 conns) — safe,
  but semaphores keep it well below the pool limit.
- SQLite test DBs must stay isolated per test (existing pattern:
  `file:<name>?mode=memory&cache=shared` + save/restore `database.DB`).

## Validation

- `go test ./... -count=1` — **PASS** (services, handlers, database; config and
  models have no test files). Focused suites also run directly:
  `go test ./services/ ./handlers/ ./config/ -count=1`.
- `go vet ./...` — **PASS** (required the pre-existing PMVHaven `localPath`
  shadowing fix described under intentional changes).
- `gofmt` check on every touched file — clean (repo convention is CRLF, so
  formatting was verified by comparing gofmt output modulo line endings).
- Benchmarks: `go test ./services/ -run XXX -bench BenchmarkSyncFileExists
  -benchtime=50x -count=3`, plus serial reference variants of the old sweep
  added to `file_exists_bench_test.go` for a same-session A/B.
- New focused tests: fast path (exactly 1 statement), normal path, image
  collection (video exclusion, URL rule, query bound), video collection
  (batched direct/gallery/uncategorized source names, file:// and empty-URL
  skip rules, present-row exclusion, query bound), `resolveSourceNamesBatch`
  direct-wins, uploads file index, config fallbacks.

### Results

- Baseline (pre-change, original code, 2000 rows, `-benchtime=5x`):
  FreshInstall **24,723,380 ns/op**; Populated **28,341,860 ns/op**.
- Post-change (2000 rows, `-benchtime=50x`, median of 3):
  FreshInstall **8.66 ms/op**; Populated **40.44 ms/op**.
- Same-session A/B vs a serial reference implementation of the old stat sweep
  (identical harness and machine conditions, 50x, median of 3):
  - FreshInstall: **8.66 ms vs 52.87 ms → 6.1× faster** (fast path: one UPDATE,
    zero per-row stats).
  - Populated: **40.44 ms vs 54.00 ms → 1.34× faster** (64-wide parallel stat
    sweep vs serial).
  - Cross-session absolute numbers vary with machine load (the recorded
    baseline and the later runs were taken hours apart), so the same-session
    A/B is the reliable comparison; both paths improved against it.
- Query-count tests prove the scan phases are chunked, not per-row:
  image collection ≤ `ceil(rows/1000)+1`, video collection ≤
  `ceil(rows/500)+3` statements regardless of row count.
