# Forum missing-gallery search popup

Status: Completed
Completed: 2026-11-20 (initial vipergirls.to implementation; kitty-kats.net follow-up)

## Problem and intended outcome

The missing-gallery UI currently offers only an outbound link to a vipergirls.to search
page. The user has to leave the app, hunt through search results, and manually work out
which thread corresponds to the missing gallery.

Replace that link with an in-app popup that searches both vipergirls.to and
kitty-kats.net and shows the results in one grid of Site, Title, Sub Forum, and the first
thumbnail from the first post. Each result offers two actions:

- **View post** — open the thread on its source site in a new tab.
- **Scrape** — automatically create/link the gallery to the person, create a Source for
  the thread, queue the crawl, and resolve the original missing-gallery result.

## Scope

- Backend: a combined search endpoint and a scrape/associate endpoint that accept both
  vipergirls.to and kitty-kats.net threads.
- Frontend: a reusable modal plus API/types, wired into both places that currently render
  the vipergirls link (Dashboard Attention > Missing Galleries and Person detail > Missing
  > Recent Scans).
- Focused Go tests for search-page parsing, URL validation, thumbnail extraction, and
  partial search failures.

## Non-goals

- Making either forum a general scan provider (no changes to `PROVIDER_OPTIONS`, TPDB
  aliases, or `ScanSourceForPerson`).
- Search pagination or login. vipergirls uses the first `data-images` entry from the
  search result; kitty-kats makes at most one bounded request per visible result to read
  the first post's thumbnail.
- Changing the existing "Scrape" action for provider scan results.
- Any destructive cleanup of existing galleries or sources.

## Acceptance criteria

1. The two existing outbound vipergirls links are replaced by buttons that open the popup.
2. The popup shows one grid with results from both sites, each labelled with its site,
   title, sub forum, and first-post thumbnail; loading, empty, error, and no-thumbnail
   states are explicit. Only results from identifiable, non-video sub forums are shown
   (video/movie forum names are excluded).
3. Each result has a working "View post" external link and a "Scrape" action. A
   successful scrape closes the popup; a failed one keeps it open with the error shown.
4. "Scrape" creates/links a gallery for the person, caches the thumbnail where possible,
   creates a Source for the chosen thread, queues the crawl, records the exclusion for
   the original missing result, and refreshes cached scans. It is idempotent for the same
   thread and person.
5. Only validated `https` thread URLs from vipergirls.to or kitty-kats.net can be scraped;
   thumbnail URLs must be absolute HTTP(S). Errors and site-specific search failures are
   surfaced to the UI.
6. The popup is keyboard accessible (Escape closes, focus moves into the dialog, focus
   returns to the opener) and follows the project design system (subtle borders, no
   scale/hover transforms, visible focus rings).
7. Focused Go tests pass; frontend TypeScript check passes.

## Implementation steps

1. `services/vipergirls_service.go`: build the existing search URL, fetch via the shared
   HTTP client, parse `ol#searchbits li.threadbit` with goquery (title, `div.threadpostedin
   a`, first `data-images` entry), normalize thread URLs, enforce a result cap, and
   validate thread/thumbnail URLs.
2. `services/kitty_kats_service.go`: fetch the XenForo search form with a cookie jar and
   CSRF token, POST the query, parse `li.block-row` result cards, and fetch the first
   post's first image for up to 16 visible results with bounded concurrency.
3. `services/vipergirls_service.go`: combine both searches under one request context,
   returning partial results plus per-site errors when one site fails.
4. `services/vipergirls_service.go`: implement `ScrapeVipergirlsAsMissingGallery` with
   person/gallery lookup, asynchronous thumbnail caching via `DownloadProviderThumbnail`,
   source creation/lookup, crawler queueing, `ScanResultExclusion`, and cached-scan refresh.
   Accept either supported thread host.
5. `handlers/vipergirls_handlers.go` plus routes in `main.go` for search and scrape.
6. Frontend: types, API methods, shared query helper, `VipergirlsSearchDialog` component,
   and replacement of the two links.
7. Tests and validation.

## Kitty-kats implementation notes

The example search URL returned a Cloudflare challenge to direct `fetch` calls, so the
result markup was inspected in the user's in-app browser. Kitty-kats is XenForo 2: result
cards are `li.block-row`, the title is `h3.contentRow-title > a`, the sub forum is the
`/forums/` link in `.contentRow-minor`, and the first post's preview image is inside
`article.message--post .message-userContent .bbWrapper a img`. The user approved one
bounded per-result request to obtain that thumbnail. The application's own Go HTTP client
can complete the search and thumbnail requests for the example query.

## Risks and constraints

- Both sites' markup can change; parsing uses specific vBulletin 4 and XenForo 2 selectors
  with a clear empty-state/error rather than silently returning bad data.
- Kitty-kats search requires a CSRF token from `/search/`, so one client with a cookie jar
  performs the form fetch and the POST. Cloudflare can still block a site; that is reported
  in `sources[].error` and the other site's results remain usable.
- `data-images` may contain full-size images rather than thumbnails; they are used only
  for preview and cached with the existing downloader.
- Thumbnail hosts may block hotlinking; failure is logged and the gallery is still linked.
- Search and thumbnail download are bounded by one request context (25s for kitty-kats,
  8s per thread thumbnail, 4 concurrent, at most 16 thread requests); no unbounded crawling
  from this feature.
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

- `go test ./services -run 'KittyKats|Vipergirls|ForumSearch' -count=1 -v` — passed.
- `go test ./services ./handlers -count=1` — new tests passed; one unrelated pre-existing
  live probe (`TestProbeLiveImageTwist544`) failed because an external image host no longer
  returned the expected image.
- `go test ./... -count=1` — same unrelated live probe failure; all other packages passed.
- `go vet ./services ./handlers` — passed.
- `npx tsc --noEmit -p tsconfig.app.json` in `frontend/` — passed.
- `eslint` on `src/components/VipergirlsSearchDialog.tsx` — passed. `src/types/index.ts`
  still has a pre-existing `no-explicit-any` error at line 214.
- Bounded live probe with the user's example query: combined search returned 15 results
  (14 vipergirls, 1 kitty-kats) with no site errors and a first-post thumbnail on the
  kitty-kats result.
- Manual UI verification pending a running server/database.

## Completion

Completed 2026-11-20. Both forum searches are combined in the popup, scrape accepts either
source's threads, synthetic tests cover parsing/validation/partial failures, TypeScript and
targeted lint pass, and a bounded live probe confirms the example query returns results
from both sites. Manual UI verification remains outstanding because no server or database
was started for this change.
