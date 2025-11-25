# Frontend Setup Instructions

The React frontend has been created in the `frontend/` directory.

## Prerequisites

You'll need Node.js and npm installed. If you don't have them:
- Download from: https://nodejs.org/ (LTS version recommended)

## Installation

1. Navigate to the frontend directory:
```bash
cd frontend
```

2. Install dependencies:
```bash
npm install
```

## Running the Frontend

Start the development server:
```bash
npm run dev
```

The frontend will be available at `http://localhost:3000`

## Features

- **Gallery View**: Browse all your galleries with thumbnail previews
- **Image Grid**: View all images in a gallery with a responsive grid layout
- **Lightbox**: Click any image to view it full-size with keyboard navigation (← → arrows, ESC to close)
- **Source Management**: Add new sources and trigger manual crawls
- **Real-time Status**: See crawl status and last checked timestamps

## API Proxy

The Vite config includes a proxy that forwards `/api/*` requests to `http://localhost:8080`, so make sure your Go API server is running.

## Build for Production

```bash
npm run build
```

The production build will be in the `dist/` directory.
