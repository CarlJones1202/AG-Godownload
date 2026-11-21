# Repository Agent Guide

## Purpose

This project is a Go gallery API and crawler/downloader with a React and TypeScript frontend and a PostgreSQL database. Make changes that are correct, maintainable, safe for stored media and metadata, and efficient at runtime. Do not trade code quality or execution performance for a quick result.

## Issues, Ideas, and Feature Plans

- Identify whether a request is a bug, feature, or idea. Clarify material ambiguities before making decisions that change user-visible behavior or risk data loss; otherwise, use reasonable assumptions and state them.
- Before implementing a feature, create or update `plans/<feature>.md`. Use a descriptive, kebab-case filename and keep the plan concise. Include the problem and intended outcome, scope and non-goals, acceptance criteria, implementation steps, risks or constraints, and validation.
- Update the plan as the work progresses. When the feature is complete, mark the plan `Completed` and record the completion date and validation performed. Keep completed plans; do not delete them.
- Do not create a full feature plan for a small bug fix unless it is useful to track the work.
- Keep implementation focused on the request. Do not silently expand scope or remove existing behavior.

## Quality, Performance, and Validation

- Prefer clear, idiomatic code and existing project patterns over clever or speculative changes. Address root causes and preserve existing behavior unless a change is explicitly intended.
- Treat runtime performance as a quality requirement. Measure or profile before and after performance-sensitive changes; do not claim improvements without evidence. Investigate slow SQL warnings rather than ignoring them.
- Add focused tests for behavior changed or introduced. Use synthetic test inputs and isolated test databases; do not use real user media or production data.
- Run the smallest relevant tests, linters, or checks, then report what was run and any failures. Do not claim validation that was not performed.
- Avoid builds unless needed. When a build is needed, place generated build output in `bin/` and avoid leaving generated files elsewhere in the repository.
- Do not commit test data, generated artifacts, downloaded-site HTML, or debug dumps. If such files are necessary during development, keep them outside the repository or in an ignored location and clean them up.

## Data, Credentials, and External Services

- Do not add, print, or expose credential values, including development credentials. Use environment variables or local untracked configuration for secrets; example configuration must contain placeholders only.
- Do not expose private media, database contents, or downloaded content in logs, test output, plans, or commits.
- Use isolated databases for tests. Never run destructive operations against user data; get explicit approval before any operation that could delete or irreversibly alter persistent data.
- Keep crawler and downloader network activity bounded. Use the project's existing HTTP clients and patterns for timeouts, concurrency, retries, and error reporting; avoid unnecessary requests to external hosts.

## Frontend and Usability

- The UI must be understandable and easy to use. Preserve established interaction patterns, provide clear feedback for loading, empty, success, and error states, and consider keyboard access and responsive layouts.
- For frontend work, follow [`designs/design-system.md`](designs/design-system.md) and its accessibility guidance. Deviating from the design system requires explicit instruction containing the exact phrase `Nike` (case sensitive).

## Project Conventions

- Read relevant existing code and documentation before changing behavior. Follow the repository's Go, PostgreSQL, React, and TypeScript patterns.
- Keep API and data-model changes compatible with existing callers and stored data where practical. Explain intentional compatibility changes.
- Surface errors explicitly; do not silently ignore failures or return success-shaped fallbacks.
- Keep generated files, local databases, uploads, and build outputs out of source changes unless the request explicitly requires them.
- Consult [`README.md`](README.md) for setup and run instructions and [`OPTIMIZATIONS.md`](OPTIMIZATIONS.md) for existing performance work. Verify guidance in those documents against the current code before relying on it.
