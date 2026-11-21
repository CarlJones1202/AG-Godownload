# Shared Premium Frontend UI Skill

**Status:** Completed (2026-11-20)

## Problem & Intended Outcome

Install the community `premium-frontend-ui` skill so repository-aware OpenCode,
Antigravity, and GitHub Copilot agents can discover its frontend design guidance.

## Scope

- Add the upstream `SKILL.md` and its MIT license under `.agents/skills/`.
- Use one shared workspace skill directory rather than per-agent copies.

## Non-goals

- Do not install the skill into a personal/global agent directory.
- Do not alter the skill content or existing frontend design-system instructions.

## Acceptance Criteria

1. All three agents discover the skill from the shared `.agents/skills` location.
2. The installed skill matches its upstream source and includes the license notice.
3. No existing files or unrelated worktree changes are modified.

## Implementation Steps

1. Verify the supported shared skill location and upstream license.
2. Install the upstream skill and license at `.agents/skills/premium-frontend-ui/`.
3. Verify source parity, metadata, license presence, and worktree scope.
4. Record validation and mark this plan complete.

## Risks & Constraints

- The skill recommends optional frontend patterns that may not fit every task;
  repository design-system, accessibility, and user requirements remain
  applicable.
- Existing unrelated worktree changes must be left untouched.

## Validation

Completed 2026-11-20.

- Confirmed `.agents/skills` is a supported project skill location for
  OpenCode, Antigravity, and GitHub Copilot.
- Compared the installed `SKILL.md` with upstream; content matches after
  normalizing line endings and upstream trailing spaces.
- Confirmed the MIT license is present and the added files are not ignored.
- No tests run; this is a documentation/skill-only change.
