# User-Selected Video Thumbnail Time

Status: Completed  
Date: 2026-11-20

Completed: 2026-11-20. Implemented the persisted thumbnail-time flow, in-player picker, immediate regeneration, recovery/re-crawl support, and focused tests.

## Problem and Intended Outcome

Video thumbnails are currently generated at a hardcoded seek time (`10s`, or `duration/3` for short videos) and the chosen time is never persisted. A redownload from missing-video recovery or source re-crawl therefore produces a thumbnail from a different moment than the one the user saw.

Users should be able to scrub to any moment in the existing player, choose it as the thumbnail, and have that exact playback time saved with the video. The thumbnail should be regenerated immediately, and every later generation path should reuse the saved time.

## Scope

- Add a nullable per-video thumbnail time to the `Image` model/database schema.
- Add an API endpoint to save the time and regenerate the current thumbnail.
- Add a "Set as thumbnail" action to `VideoPlayer` with loading, success, and error feedback.
- Thread the saved time through inline downloads, source re-crawl, missing-video recovery, and present-video thumbnail regeneration.
- Preserve the saved time across source re-crawl despite the existing delete-and-recreate behavior.
- Add focused tests for time resolution/validation and the API behavior.

### Non-Goals

- No new single-video redownload endpoint or UI action; manual redownload remains the existing source re-crawl (per user decision).
- No changes to trickplay sprite/VTT generation or playback controls.
- No new thumbnail storage layout; thumbnails stay at `uploads/<source>/thumbnails/<hash>.jpg`.
- No provider/gallery thumbnail changes.
- No attempt to compensate for frame timing differences caused by a differently encoded redownload; see Risks.

## Acceptance Criteria

1. Scrubbing to a time in `VideoPlayer` and activating "Set as thumbnail" saves that time (seconds, millisecond precision) to the video record.
2. The thumbnail file is replaced immediately from the selected time, and the card thumbnail updates without a full page reload.
3. Reloading the page shows the saved thumbnail and saved time.
4. Missing-video recovery generates the recovered thumbnail from the saved time.
5. The present-video thumbnail regeneration pass uses the saved time when rebuilding a missing thumbnail.
6. Source re-crawl carries the saved time from the previous record for that source URL to the replacement record and generates its thumbnail at that time.
7. Videos without a saved time retain the existing default-seek and skip-if-exists behavior.
8. Invalid requests (non-video, missing file, non-finite/negative time, time beyond duration) return a clear 4xx error and do not overwrite the saved value.
9. Backend and frontend type checks/tests pass.

## Implementation Steps

1. **Model/schema**
   - Add `ThumbnailTime *float64` with `json:"thumbnail_time_seconds"` and GORM column `thumbnail_time_seconds` to `models.Image`.
   - Let existing `AutoMigrate` add the nullable column; no data backfill is needed.

2. **Thumbnail generation service**
   - Refactor `GenerateVideoThumbnail` so an explicit time forces regeneration and writes to a temporary file before atomically replacing the existing JPG.
   - Keep the existing default-seek and skip-if-exists behavior when no time is supplied.
   - Clamp an explicit time to the video duration and format it consistently so repeated generations use the same seek value.
   - Add a small helper to resolve the effective seek time for testability.

3. **Call sites**
   - Add an optional thumbnail-time argument through `buildVideoResult`, `downloadVideo`, `DownloadVideo`, `ImportLocalVideo`, and `importLocalVideo`.
   - In `ProcessVideoSource`, capture the saved time from existing videos before deletion (matching on `OriginalURL`, preferring the newest matching row), reuse it for download/import, and store it on the new record.
   - Add `thumbnail_time_seconds` to missing-video scan rows and `VideoRecoveryTask`; pass it to recovery post-processing and present-video thumbnail regeneration.

4. **API**
   - Add `PATCH /images/:imageId/thumbnail-time`.
   - Validate video type, local file existence, finite non-negative time, and duration bound.
   - Regenerate the thumbnail at the requested time, then persist the value and return the updated `Image`.

5. **Frontend**
   - Add `thumbnail_time_seconds` to the `Image` type and `videos.setThumbnailTime` to the API client.
   - Add an icon button with tooltip/aria-label to `VideoPlayer`; use the current `HTMLVideoElement.currentTime`, throttled to millisecond precision by the API payload.
   - Show saving/success/error feedback with `role="status"`/`role="alert"` and keep controls keyboard accessible.
   - Add an optional `onThumbnailChange` callback so each page that opens the player (`VideosPage`, `FavoritesPage`, `PersonDetailPage`) can update its local video data from the API response.

6. **Tests and validation**
   - Add service tests for default seek, explicit seek, clamping, and forced regeneration.
   - Add a handler test for validation and successful save/regeneration using a synthetic video generated by ffmpeg, or an isolated fake where ffmpeg is unavailable.
   - Run focused Go tests and frontend type/lint checks; report exact commands and results.

## Risks and Constraints

- ffmpeg's `-ss` placed after `-i` decodes to the frame at or after the requested timestamp; if a redownload is re-encoded differently, the decoded frame may differ slightly. Storing the exact requested timestamp and using the identical command for every generation is the closest reproducible behavior available.
- An explicit time beyond the duration would produce no frame; clamp to the probed duration and reject values beyond duration plus a small tolerance at the API boundary.
- Source re-crawl deletes old records and files before downloading. The saved time must be captured before that loop; if several old records match the same URL, use the most recently updated one.
- Immediate regeneration and a concurrent recovery pass could both write the same thumbnail. The temporary-file/rename approach keeps either write atomic; both use the same stored time.
- Existing video deletion uses a different thumbnail filename convention (`<hash>.mp4.jpg` vs generated `<hash>.jpg`). This feature does not change that behavior; it is a separate cleanup concern.

## Validation

- `gofmt -w` on all changed Go files — passed.
- `go test ./models ./services ./handlers` — passed (`models` has no test files; `services` and `handlers` passed). Added `services/video_thumbnail_time_test.go` and `handlers/video_thumbnail_time_test.go`.
- `tsc -p frontend/tsconfig.app.json --noEmit` — passed.
- Targeted `eslint` on changed frontend files — no new findings; existing `no-explicit-any` errors remain in `api.ts`, `PersonDetailPage.tsx`, and `types/index.ts` on pre-existing lines.
- Manual verification is still recommended: open a video, scrub to a distinctive frame, set it as thumbnail, reload, trigger source re-crawl, and simulate a missing file/recovery to confirm the same frame is used.
