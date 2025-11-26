# Gallery API

A Go-based image gallery API with automatic crawling and image ripping capabilities.

## Features

- Gallery management with many-to-many relationships
- Image management with metadata
- Source crawling from various image hosts
- People management with automatic gallery linking
- Automatic image downloading and thumbnail generation

## Running the Application

```bash
go run .
```

The server will start on port 8080.

## Log Levels

The application supports different log levels to control verbosity. By default, only **WARN** and **ERROR** messages are shown.

### Available Log Levels

- `TRACE` - Most verbose, shows all internal operations
- `DEBUG` - Shows detailed debugging information (ripper operations, worker activity)
- `INFO` - Shows general informational messages (startup, migrations)
- `WARN` - Shows warnings (default level)
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

**Default (WARN)** - Minimal output, only warnings and errors:
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

### Galleries
- `GET /api/galleries` - List all galleries
- `POST /api/galleries` - Create a gallery
- `GET /api/galleries/:id` - Get gallery details
- `DELETE /api/galleries/:id` - Delete a gallery

### Images
- `GET /api/images` - List all images
- `POST /api/galleries/:id/images` - Add image to gallery
- `DELETE /api/images/:id` - Delete an image
- `GET /api/images/:filename` - Serve image file
- `GET /api/thumbnails/:filename` - Serve thumbnail

### Sources
- `GET /api/sources` - List all sources
- `POST /api/sources` - Create a source
- `POST /api/sources/:id/crawl` - Trigger crawl
- `DELETE /api/sources/:id` - Delete a source

### People
- `GET /api/people` - List all people
- `POST /api/people` - Create a person
- `GET /api/people/:id` - Get person details
- `PUT /api/people/:id` - Update person
- `DELETE /api/people/:id` - Delete person
- `POST /api/people/:id/link-galleries` - Auto-link galleries by name/alias

## Frontend

The React frontend is located in the `frontend/` directory.

```bash
cd frontend
npm install
npm run dev
```

The frontend will be available at http://localhost:5173
