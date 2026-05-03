Finalize and publish the current branch's changes.

## Phase 1: Verify

1. Run `make test` — all tests must pass.
2. Run `make lint` — no issues.
3. Run `make verify` — formatting, vetting, tidying, and no uncommitted changes.
4. If a work item spec exists (`specs/YYYY-MM-DD-*/` with `status: in-progress`), verify all acceptance criteria are met.
5. Check that `CLAUDE.md` is up to date with any new packages or conventions.

If any check fails, stop and report the issue.

## Phase 2: Commit

1. Read `specs/conventions.md` for commit and PR format.
2. Check for any uncommitted changes. If present, use AskUserQuestion to confirm whether to commit them (and into which existing commit, or as a new commit).
3. Review the commit history on this branch. If there are fixup commits that should be squashed, do so.

## Phase 3: Publish

1. Use AskUserQuestion to confirm before pushing.
2. Push the branch and create a PR following the conventions in `specs/conventions.md`:
   - Title: conventional commit format
   - Body: Summary + Test plan sections
   - Link to the work item spec directory if one exists
3. If a work item spec exists, update its frontmatter:
   - Set `status: pr-submitted`
   - Add `pr: <PR URL>`

## Phase 4: Monitor CI

1. Spawn a background agent to watch the PR's CI checks. The agent should:
   - Poll the PR's check runs periodically until they complete.
   - Report back with any failures, including the failing check name and a summary of the error.
2. Summarize: PR URL, what was shipped, and that CI is being monitored in the background.
