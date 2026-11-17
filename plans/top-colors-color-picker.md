# Top-16 Colors + Color Picker for Image Color Search

**Status:** Completed (2026-11-17)

## Problem & Intended Outcome

TODO (Major Tasks → Advanced image searching): "Search for images based on color
(top 16 colors used in database should be provided to the user, with an optional
color picker)".

The Images page already has a hex-input color search (`GET /search/color`), but
the user has no way to discover which colors actually exist in their library and
no visual picker.

**Outcome:** the color-search UI offers up to 16 preset swatches derived from the
colors most used in the database, plus a native color picker that fills the hex
input; clicking a swatch runs the similarity search for that color.

## Decisions (confirmed with user)

- **Top-16 method — bucket similar colors:** each stored hex is quantized to 16
  shades per channel (4 bits/channel, 4096 buckets). A bucket ranks by the
  number of *images* whose palette contains it (deduped per image, so one image
  with several near-identical colors counts once). The swatch shows the average
  of the actual member colors. This guarantees 16 visually distinct swatches
  instead of near-duplicate exact hexes.
- **Color picker behavior — fill input only:** choosing a color in the native
  `<input type="color">` updates the hex field and preview swatch; the user
  presses Search (or Enter). Avoids firing the full-table similarity search
  repeatedly while dragging in the picker. Clicking a *preset swatch*, by
  contrast, runs the search immediately (discrete click = clear intent).
- Endpoint follows the `GET /tags/top` pattern: `GET /colors/top?limit=16`
  (default 16, clamped 1–64), returning `[{ "color": "#RRGGBB", "image_count": N }]`.

## Scope

- Backend: aggregation service (`TopDominantColors`), `GetTopColors` handler,
  route registration.
- Frontend: `colors.top()` API helper + `TopColor` type, top-16 swatch row and
  color picker in the Images-page color-search UI.
- Focused tests (bucketing, per-image dedupe, ranking, limit, average color,
  single-query assertion, handler validation) + a benchmark for the new
  full-column scan.

## Non-goals

- No change to `SearchByColor` semantics (threshold, pagination, full-scan
  similarity) — pre-existing behavior, out of scope.
- No new DB index/migration and no SQL-side JSON parsing (must stay portable
  across sqlite tests and Postgres production; aggregation runs in Go over a
  single streamed column).
- No URL-param persistence of the active color search (existing color search is
  local state; keep consistent).
- No backend response cache; freshness is handled client-side via react-query
  `staleTime` (revisit only if the benchmark shows the scan is slow).

## Acceptance Criteria

1. `GET /colors/top` returns up to 16 entries, sorted by image count descending
   (ties broken by color for determinism), each with a valid `#RRGGBB` average
   color; empty library → `[]`, not `null`.
2. Near-identical hexes (same 4-bit bucket) merge into one entry; an image whose
   palette hits the same bucket twice counts once.
3. Invalid/empty `dominant_colors` values are skipped without failing the scan.
4. The scan is a single SQL statement (asserted with the existing sqlCounter
   logger pattern).
5. Images page shows the swatch row (hidden when no colors exist), clicking a
   swatch sets the hex field and runs the search, the active search color is
   highlighted on its swatch, and the native picker only fills the input.
6. UI follows `designs/design-system.md`: subtle borders, 6–8px radii, border-
   color-only hover, 2px accent focus ring, no emoji/gradients/pills; keyboard
   accessible (real buttons, `aria-label`/`aria-pressed`, visible focus).
7. Validation: `go vet ./...`, `go test ./... -count=1`, frontend `tsc -b`
   and `eslint` on touched files, benchmark recorded in this plan.

## Implementation Steps

1. `services/color_service.go`: `TopColor` type + `TopDominantColors(limit)`
   streaming `dominant_colors` in one query, bucketing/dedupe/average as above.
2. `handlers/color_handlers.go`: `GetTopColors` (limit parse + clamp, error
   handling mirroring `GetTopTags`).
3. `main.go`: register `GET /colors/top` next to `/search/color`.
4. Frontend `types` + `lib/api.ts`: `TopColor`, `colors.top(limit)`.
5. `ImagesPage.tsx`: fetch top colors (`enabled: showFilters`, `staleTime`
   5 min); add picker next to the hex input and a "Top colors" swatch row below
   it; swatch click → set input + search; active-search highlight; empty/loading
   states hide the row gracefully.
6. Tests: services (bucket merge, dedupe, ranking, limit, average, invalid
   input, empty DB, query count) and handler (limit clamp, JSON shape).
7. Benchmark `BenchmarkTopDominantColors` for evidence.
8. Run validation; record results here; mark Completed.

## Risks & Constraints

- The aggregation scans one column of the whole `images` table — acceptable for
  a personal-scale gallery and measured via benchmark; client `staleTime`
  bounds request frequency. If the benchmark shows multi-hundred-ms at test
  scale, add a server-side cache before merging.
- `sources.location` UNIQUE constraint affects test seeding (use distinct
  `Location` values) — already handled by the shared recovery test helper.
- Repo uses CRLF; verify formatting by comparing gofmt output modulo line
  endings (never run `gofmt -w` without restoring CRLF).
- Go not on PATH: prefix commands with
  `$env:Path = "C:\Program Files\Go\bin;$env:Path"`.

## Validation

Completed 2026-11-17. All 8 implementation steps done.

**Backend tests** (new, all PASS):
- `services/color_top_test.go`: `TestTopDominantColorsRanksDedupesAndAverages`
  (bucket merge, per-image dedupe, malformed/invalid/NULL/empty/`[]` skipping,
  ranking, member-average color), `TestTopDominantColorsLimitAndTieBreak`
  (limit, tie-break, default/oversized limits),
  `TestTopDominantColorsSingleQuery` (sqlCounter proves exactly 1 statement),
  `TestTopDominantColorsEmptyLibrary` (`[]` not `null`),
  `TestColorBucketKeyQuantizesTo16Shades` (packing + boundaries).
- `handlers/color_top_test.go`: `TestGetTopColorsClampsLimit` (default 16,
  clamp 1–64, unparsable/non-positive → default, `#RRGGBB` + positive count on
  every entry, 70-seed DB), `TestGetTopColorsEmptyLibrary` (200 + body `[]`).
- Shared helpers `setupRecoveryTestDB`/`seedImages` widened to `testing.TB`
  (benchmark reuse); `file_exists_service_test.go` re-verified CLEAN.

**Commands run:**
- `go vet ./...` → exit 0.
- `go test ./... -count=1` → PASS (database, handlers, services).
- `gofmt` modulo CRLF on all 6 touched Go files → clean (verified by byte
  comparison of `gofmt` output vs file with line endings normalized; the repo
  is CRLF and `gofmt -l` flags ~100 pre-existing files, so a raw `-l` is not
  the signal here).
- Frontend `npx tsc -b` → exit 0.
- `npx eslint` on touched files → only pre-existing `no-explicit-any` errors
  (api.ts:115/286, ImagesPage.tsx:37/70, types/index.ts:211) and the
  pre-existing `displayImages` exhaustive-deps warnings — all on untouched
  lines; project-wide baseline is 28 such errors, none introduced by this
  change (new code contains no `any`).

**Benchmark** (`go test ./services/ -run XXX -bench BenchmarkTopDominantColors
-benchtime=50x -count=3`, 2000 images × 5 colors, AMD Ryzen 7 7700X):
6.17 ms / 4.17 ms / 8.24 ms per op (median ≈ 6.2 ms). Far below the
multi-hundred-ms threshold in Risks — no server-side cache needed; client
`staleTime` (5 min) bounds request frequency as planned.

**Acceptance criteria:** all 7 met (endpoint shape/ordering/empty behavior,
bucket merge + dedupe, invalid-row tolerance, single-query proof, swatch row
+ picker + immediate swatch search + active highlight + fill-only picker,
design-system compliance with real buttons/`aria-label`/`aria-pressed`/2px
accent focus ring, full validation above).
