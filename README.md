# Gallery API

A Go-based image gallery API with automatic crawling and image ripping capabilities.

## Features

- Gallery management with many-to-many relationships
- Image management with metadata
- Source crawling from various image hosts
- People management with automatic gallery linking
- Automatic image downloading and thumbnail generation

## Setup
1. Clone the repository.
2. Run the initialization script to install dependencies (FFmpeg, yt-dlp, Go, Node.js,
   Python 3.13, VC++ runtime), set up portable PostgreSQL, and prepare the optional
   local embedding runtime:
   ```powershell
   ./init.ps1
   ```
   *Note: This script uses `winget` to install system dependencies, downloads PostgreSQL
   and the SigLIP embedding model, and writes a local `.env`. `init.ps1` is interactive and
   may need an Administrator PowerShell session. `.env`, WireGuard configs, and cookie
   exports are local-only secrets and are not tracked by Git.*

## Running the Application

1. Ensure the PostgreSQL database is running:
   ```powershell
   ./start-db.ps1
   ```
   *(To stop the database later: `./stop-db.ps1`)*

2. Start the server:

```bash
go run .
```

The server will start on port 8080.

Missing-gallery rechecks run with four workers by default. Set
`PERSON_SCAN_WORKERS` in the server environment to adjust concurrency; scans for
the same provider remain serialized. Pending scans resume automatically after a
server restart.

### Graceful shutdown and crawl recovery

Stop the server with Ctrl+C (or send SIGTERM). Shutdown stops workers from
starting queued sources and waits for each active source crawl to finish before
exiting. Sources that were still queued are re-queued automatically when the
server starts again. The wait can last as long as the current source takes; keep
PostgreSQL running until the server reports that shutdown is complete.
PostgreSQL is a separate process: API shutdown does not start or stop it.
The database startup scripts launch PostgreSQL without sharing the API console,
so Ctrl+C for the server is not propagated to PostgreSQL.

### Recover a gallery with missing photos

If a crawl was interrupted and its gallery may be incomplete, inspect one
gallery and queue its source again when the saved photo count differs from the
count found in the source post:

```powershell
go run ./cmd/recover-source -gallery-id 123
```

To audit all galleries with an HTTP(S) source, use:

```powershell
go run ./cmd/recover-source -all
```

The command reports both counts and persists a queued status only when they
differ, then asks the running server to start the crawl immediately. Keep the
server running and use the same database configuration as the server. By
default, the command contacts `http://localhost:<PORT>` using the configured
`PORT`; use `-server-url` if the API is hosted elsewhere. Batch audits run
sequentially; galleries without an HTTP(S) source are skipped, and a failure
for one gallery does not stop the rest. If a live request fails, its queued
status remains persisted for the next server startup.

### ThePornDB provider aliases

Set `TPDB_API_KEY` in the server process environment or local `.env`, then
restart the server. Prefer the process environment and do not commit the key.
The key stays on the server and is not sent to the browser. The `.env.example`
file lists the required setting.

On a person's page, select **Find with TPDB** in the Provider Aliases section.
Review the matched performers and add each studio-specific name individually.

## Log Levels

The application supports different log levels to control verbosity. The default is **INFO**, which includes warnings and errors.

### Available Log Levels

- `TRACE` - Most verbose, shows all internal operations
- `DEBUG` - Shows detailed debugging information (ripper operations, worker activity)
- `INFO` - Shows general informational messages (startup, migrations) — default
- `WARN` - Shows warnings only
- `ERROR` - Shows only errors

### Setting Log Level

Set the `LOG_LEVEL` environment variable:

**Windows (PowerShell):**
```powershell
$env:LOG_LEVEL="DEBUG"
go run .
```

**Windows (CMD):**
```cmd
set LOG_LEVEL=DEBUG
go run .
```

**Linux/Mac:**
```bash
LOG_LEVEL=DEBUG go run .
```

### Log Level Examples

**Default (INFO)** - Startup and important events, plus warnings and errors:
```
[WARN]  Crawler queue full, skipping source 5
[ERROR] Worker 2 error crawling source 3: connection timeout
```

**INFO** - Includes startup and important events:
```
[INFO]  Database connected successfully
[INFO]  Database migrated successfully
[INFO]  Background crawler worker started
[INFO]  Server starting on :8080
```

**DEBUG** - Includes detailed operation logs:
```
[DEBUG] Starting RipViprIm for https://i7.vipr.im/th/02860/x1fgujtavvse.jpg
[DEBUG] Transformed Vipr.im URL: https://i7.vipr.im/i/02860/x1fgujtavvse.jpg
[DEBUG] Worker 1 processing source 42
[DEBUG] Crawler worker 3 started
```

## API Endpoints

Most routes are registered at the **root path**. The frontend prefixes requests with
`/api` and Vite strips that prefix when proxying to the Go server, so browser calls look
like `/api/galleries` while direct server calls use `/galleries`. Exceptions that really
do start with `/api` are `/api/stats`, `/api/export/db`, and `/api/import/db`.

### Galleries
- `GET /galleries` - List galleries
- `POST /galleries` - Create a gallery
- `GET /galleries/:id` - Get gallery details
- `PUT /galleries/:id` - Update a gallery
- `DELETE /galleries/:id` - Delete a gallery
- `POST /galleries/:id/images` - Add an image to a gallery

### Images
- `GET /images` - List images
- `DELETE /images/:imageId` - Delete an image
- `GET /images/*filepath` - Serve an image file
- `GET /thumbnails/*filepath` - Serve a thumbnail
- `POST /images/:imageId/favorite` - Toggle favorite
- `PATCH /images/:imageId/thumbnail-time` - Save a video thumbnail time and regenerate its thumbnail
- `GET /search/color` - Search by hex color
- `GET /colors/top` - Most used colors

### Sources
- `GET /sources` - List sources
- `POST /sources` - Create a source
- `POST /sources/:id/crawl` - Trigger a crawl
- `DELETE /sources/:id` - Delete a source

### People
- `GET /people` - List people
- `POST /people` - Create a person
- `GET /people/:id` - Get person details
- `PUT /people/:id` - Update a person
- `DELETE /people/:id` - Delete a person
- `POST /people/:id/link-galleries` - Auto-link galleries by name/alias

### Missing gallery discovery
- `GET /admin/vipergirls/search?query=<terms>` - Parse the first page of vipergirls.to
  photo-section search results into title, sub forum, thread URL, and first-post
  thumbnail. The request mirrors the site's own "Search photo sections" option so video
  sub forums are excluded.
- `POST /admin/vipergirls/scrape` - Associate a chosen thread with a person's missing
  gallery, create its source, and queue the crawl. The browser calls these through the
  `/api` proxy prefix.

### Similarity and ratings
- `GET /similar/:id` - Recommendations for an image
- `GET /similar?ids=<base64>` - Recommendations for a shared id list
- `GET|PUT|DELETE /ratings/:imageId` - Read, set, or clear a 1–5 rating
- `POST /profile/reset` - Reset the shared taste profile

## Frontend

The React frontend is located in the `frontend/` directory.

```bash
cd frontend
npm install
npm run dev
```

The frontend will be available at http://localhost:3000. The Vite dev server proxies
`/api`, `/images/`, `/thumbnails`, `/person-images`, and `/ws` to the Go server on port 8080.
