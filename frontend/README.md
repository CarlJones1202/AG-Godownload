# Gallery API frontend

React + TypeScript + Vite UI for the Gallery API. The Go backend runs separately;
start it with `go run .` from the repository root before using the app.

## Commands

```bash
npm install       # install dependencies
npm run dev       # dev server at http://localhost:3000
npm run build     # type-check and build into dist/
npm run lint      # ESLint
npm run preview   # preview the production build
```

## API access

The dev server proxies `/api` to the Go server at `http://localhost:8080` and strips
the `/api` prefix, so frontend code calls `/api/galleries` while the backend route is
`/galleries`. Image, thumbnail, person-image, and WebSocket paths are proxied separately
in `vite.config.ts`.

## Conventions

Follow [`../designs/design-system.md`](../designs/design-system.md) for UI changes,
including its accessibility guidance. No automated test runner is configured; the
`scripts` above are the supported checks.
