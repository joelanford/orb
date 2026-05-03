Plan the next work item for the project.

## Step 1: Check for clean state

Ensure the working tree is on a fresh, empty change with no pending modifications. If there are pending changes, use AskUserQuestion to ask whether to (a) make a new commit and proceed, (b) proceed on the current change anyway, or (c) abort.

## Step 2: Analyze the backlog

1. List all directories under `specs/` matching `YYYY-MM-DD-*/`.
2. Read the `README.md` in each to get its title, status (from frontmatter), and summary.
3. Filter to only `idea` and `ready` items.
4. Present the filtered items to the user.

## Step 3: Choose what to work on

If the user provided input via $ARGUMENTS, use that as a starting point.

Otherwise, use AskUserQuestion to help the user decide:
- Show `idea` and `ready` items as candidates
- Suggest which item to tackle next based on: dependencies between items, logical ordering, and project goals from `specs/mission.md`
- The user can also describe a new idea to create

## Step 4: Create or refine the work item

### If creating a new item:

1. Create `specs/YYYY-MM-DD-<slug>/README.md` with this structure:
   ```markdown
   ---
   status: idea
   ---
   # <Title>

   <One or two sentence description of the idea.>
   ```
2. Use AskUserQuestion to ask: should we refine this now or leave it as an idea for later?

### If refining an existing `idea` item (or a new item the user wants to refine now):

Use AskUserQuestion iteratively to gather requirements, implementation approach, and verification criteria. Reference `specs/tech-stack.md` for tech choices and `specs/mission.md` for design principles throughout.

When refined, update the spec directory to the full structure with four files:

**README.md** — high-level summary and overview:
```markdown
---
status: ready
---
# <Title>

## Summary
<What this work item delivers and why it matters.>

## Design
<Key design decisions, type definitions, caller patterns, and how different
implementations map to the API. This is the heart of the spec — it should
be detailed enough that a reader understands the full shape of the work.>
```

**requirements.md** — functional requirements:
```markdown
# Requirements

- <Requirement 1>
- <Requirement 2>
- ...

## Acceptance Criteria
- <Criterion 1>
- <Criterion 2>
- ...
```

**plan.md** — specific implementation plan:
```markdown
# Implementation Plan

1. <Task group 1>
2. <Task group 2>
3. ...
```

**verification.md** — how to verify the implementation:
```markdown
# Verification

## Implementation Correctness
- [ ] <Verification that the implementation plan was followed correctly>
- [ ] <Verification step 2>
- ...

## Project Conventions
- [ ] <Check against specs/conventions.md>
- [ ] <Check against specs/mission.md design principles>
- [ ] <Check against specs/tech-stack.md>
- ...
```

## Step 5: Review

After writing, re-read all spec files and check:
- Does the implementation plan align with `specs/mission.md` design principles?
- Does it use the tech stack from `specs/tech-stack.md` correctly?
- Are acceptance criteria testable and specific?
- Are there any gaps or ambiguities?

Fix straightforward issues directly. Use AskUserQuestion for anything with multiple valid options.
