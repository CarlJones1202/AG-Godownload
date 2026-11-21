# ViperGirls missing-gallery search popup

Status: Completed
Completed: 2026-11-20

## Problem and intended outcome

The missing-gallery UI currently offers only an outbound link to a vipergirls.to search
page. The user has to leave the app, hunt through search results, and manually work out
which thread corresponds to the missing gallery.

Replace that link with an in-app popup that fetches the same search page and shows the
results as a grid of Title, Sub Forum, and the first thumbnail from the first post. Each
result offers two actions:

- **View post** — open the thread on vipergirls.to in a new tab.
- **Scrape** — automatically create/link the gallery to the person, create a Source for
  the thread, queue the crawl, and resolve the original missing-gallery result.

## Scope

- Backend: a search endpoint and a scrape/associate endpoint for vipergirls.to threads.
- Frontend: a reusable modal plus API/types, wired into both places that currently render
  the vipergirls link (Dashboard Attention > Missing Galleries and Person detail > Missing
  > Recent Scans).
- Focused Go tests for search-page parsing and URL validation.

## Non-goals

- Making vipergirls.to a general scan provider (no changes to `PROVIDER_OPTIONS`, TPDB
  aliases, or `ScanSourceForPerson`).
- Search pagination, login/cookies, or extra per-thread HTTP requests to discover a
  better thumbnail. The first `data-images` entry on the search result is used as-is.
- Changing the existing "Scrape" action for provider scan results.
- Any destructive cleanup of existing galleries or sources.

## Acceptance criteria

1. The two existing outbound vipergirls links are replaced by buttons that open the popup.
2. The popup shows a grid of results with title, sub forum, and first-post thumbnail;
   loading, empty, error, and no-thumbnail states are explicit. Only results from
   identifiable, non-video sub forums are shown (video/movie forum names are excluded).
3. Each result has a working "View post" external link and a "Scrape" action.
4. "Scrape" creates/links a gallery for the person, caches the thumbnail where possible,
   creates a Source for the vipergirls thread, queues the crawl, records the exclusion for
   the original missing result, and refreshes cached scans. It is idempotent for the same
   thread and person.
5. Only validated `https://vipergirls.to/.../threads/<id>...` URLs can be scraped; thumbnail
   URLs must be absolute HTTP(S). Errors are surfaced to the UI.
6. The popup is keyboard accessible (Escape closes, focus moves into the dialog, focus
   returns to the opener) and follows the project design system (subtle borders, no
   scale/hover transforms, visible focus rings).
7. Focused Go tests pass; frontend TypeScript check passes.

## Implementation steps

1. `services/vipergirls_service.go`: build the existing search URL, fetch via the shared
   HTTP client, parse `ol#searchbits li.threadbit` with goquery (title, `div.threadpostedin
   a`, first `data-images` entry), normalize thread URLs, enforce a result cap, and
   validate thread/thumbnail URLs.
2. `services/vipergirls_service.go`: implement `ScrapeVipergirlsAsMissingGallery` with
   person/gallery lookup, thumbnail caching via `DownloadProviderThumbnail`, source
   creation/lookup, crawler queueing, `ScanResultExclusion`, and cached-scan refresh.
3. `handlers/vipergirls_handlers.go` plus routes in `main.go` for search and scrape.
4. Frontend: types, API methods, shared query helper, `VipergirlsSearchDialog` component,
   and replacement of the two links.
5. Tests and validation.

## Risks and constraints

- vipergirls.to markup can change; parsing uses specific vBulletin 4 selectors with a
  clear empty-state/error rather than silently returning bad data.
- `data-images` may contain full-size images rather than thumbnails; they are used only
  for preview and cached with the existing downloader.
- Thumbnail hosts may block hotlinking; failure is logged and the gallery is still linked.
- Search and thumbnail download are bounded by the existing HTTP client timeouts/retries;
  no unbounded crawling from this feature.
- Photo-only filtering uses the sub forum label from each result. Forums whose names
  contain video/movie/clip/film terms are excluded, and results without a sub forum label
  are dropped because their type cannot be verified. A new video forum with an unrelated
  name would need the filter updated.
- The upstream search must mirror the site's own "Search photo sections" option:
  `imgsearch=1`, `childforums=1`, `contenttype=1`, `titleonly=1`, and the seven photo
  forum IDs (`25, 161, 233, 235, 268, 372, 274`). Sending `forumchoice[]=235` alone is
  ignored by the site's custom search code and searches every forum, which buried valid
  photo threads.
- Result rows can contain a thread-prefix link (e.g. `[MetArtX]`) that also uses class
  `title` and appears before the real thread link. The parser must select the link by
  its `threads/` href; selecting the first `a.title` silently drops those results.
- Thumbnail caching must stay off the request path. `DownloadProviderThumbnail` allows
  a two-minute context plus retries and IMX request slots, so running it synchronously
  left the UI stuck on "Scraping...". The scrape now stores the remote URL immediately
  and caches bytes in a background goroutine that re-reads the gallery before saving.
- Source `location` is unique; an existing source for the thread is reused instead of
  creating a duplicate.

## Validation

- `go test ./services ./handlers -count=1` — passed.
- `go test ./... -count=1` — passed.
- `go test ./services -run Vipergirls -count=1` — passed (new parser/URL tests).
- `npx tsc --noEmit -p tsconfig.app.json` in `frontend/` — passed.
- `eslint` on the changed frontend files — passed; the repository still has pre-existing
  lint errors in unrelated files.
- Manual UI verification pending a running server/database.

## Completion

Completed 2026-11-20. Search parsing, URL validation, backend tests, TypeScript check,
and targeted lint all pass. Manual UI verification remains outstanding because no server
or database was started for this change.
