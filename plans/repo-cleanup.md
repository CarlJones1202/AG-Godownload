# Repo Cleanup

Status: Completed (2026-11-20)
Date: 2026-11-20

## Problem and intended outcome

The repository has accumulated stale documentation, obsolete snapshots, dead code/assets,
stale generated artifacts, tracked credential-bearing files, and a malformed `.gitignore`.
The intended outcome is a smaller, accurate repository where docs match the current
implementation, unreferenced files are removed, and local secrets are no longer tracked
going forward.

## Scope

- Refresh inaccurate statements in `README.md`, `OPTIMIZATIONS.md`,
  `docs/CONTENT_SIMILARITY.md`, `WIREGUARD_SETUP.md`, `frontend/README.md`, and `AGENTS.md`.
- Remove dangling references to the deleted `TODO.md` from docs and plans.
- Delete confirmed orphaned/dead files and stale generated artifacts (see implementation steps).
- Untrack and ignore `.env`, `wireguard.conf`, `xhamster_cookies.txt`, and
  `youtube_cookies.txt`; keep the local working-tree files intact.
- Fix the malformed `.gitignore` rule and add ignore rules for local secrets.
- Add this plan and mark it completed with validation results.

## Non-goals

- No runtime behavior changes, API changes, or schema/migration changes.
- No rewriting of Git history, rotation of exposed credentials, or deletion of the user's
  local copies of untracked secret files.
- No changes to the user's existing uncommitted work: deleted `TODO.md`, modified `init.ps1`,
  modified `handlers/import_db.go`, and new `handlers/import_db_test.go`.
- No removal of legacy-but-referenced code such as `cmd/importer`.

## Acceptance criteria

- Every remaining doc statement about ports, routes, log levels, and feature status matches
  current code; no doc references `TODO.md` or other nonexistent files.
- All listed dead files, stale artifacts, and the obsolete `code.audit.md` are removed.
- `git ls-files` no longer lists the four credential-bearing files; `.gitignore` blocks them.
- Go tests/build and frontend type-check/lint still pass as before the cleanup.
- Uncommitted user changes remain present and unmodified.

## Implementation steps

1. Read the affected docs and configs to verify exact replacements.
2. Update docs, remove dangling references, and tidy duplicate/stale sections.
3. Delete confirmed orphaned files, dead code, and stale artifacts.
4. Update `.gitignore`; untrack credential files with `git rm --cached`.
5. Add safe placeholder examples where useful without exposing values.
6. Run focused validation and update this plan with results.

## Risks and constraints

- Cookie/WireGuard/.env secrets are already in Git history; untracking only prevents future
  commits. Rotation must be handled separately by the user.
- `git rm --cached` alters the index; avoid staging or touching unrelated user changes.
- Some orphaned files may be intentionally kept as future references; all deletions were
  confirmed with the user via the "All of the above" cleanup scope.

## Validation

Completed 2026-11-20:

- `go test ./...` — all packages pass (gallery_api, cmd/recover-source, database,
  handlers, services; remaining packages report no test files).
- Frontend `npm run build` (`tsc -b && vite build`) — passes. Generated `frontend/dist`
  was removed after validation.
- Frontend `npm run lint` — fails with 28 pre-existing errors and 3 warnings in files
  that were not changed by this cleanup (`api.ts`, `utils.ts`, page components,
  `types/index.ts`). No lint errors came from removed files. Fixing those is out of scope.
- `git check-ignore` confirms `.env`, `wireguard.conf`, and `*_cookies.txt` are ignored;
  `git rm --cached` removed them from the index while leaving local files intact.
- Final `git status` checked to ensure the user's `TODO.md` deletion and `go.mod`
  line-ending change were not staged.

## Notes

- Exposed credentials remain in Git history; untracking only prevents new commits.
  Rotation/invalidation is a separate user decision.
- The user committed their in-flight `init.ps1` / `handlers/import_db` work as
  `5689f25` during this cleanup; it was not altered.
- The user's unstaged `TODO.md` deletion was left untouched. The dangling references
  to it in docs/plans were removed instead, and the "Nike" design-system exception
  was preserved in `AGENTS.md`.
