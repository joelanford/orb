Implement a work item from its spec.

## Step 1: Identify the work item

If the user provided input via $ARGUMENTS, use that to find the spec directory.

Otherwise, list `specs/YYYY-MM-DD-*/` directories with `status: ready` or `status: in-progress` in their README.md frontmatter. Use AskUserQuestion to ask which one to implement.

## Step 2: Read the spec

Read all files in the work item's spec directory:
- `README.md` — summary and design
- `requirements.md` — functional requirements and acceptance criteria
- `plan.md` — implementation plan
- `verification.md` — verification criteria

If the spec is incomplete (still an `idea` or missing files), stop and tell the user to run `/sdd-plan-next-phase` to refine it first.

## Step 3: Update status

Set the frontmatter `status: in-progress` in the work item's README.md.

## Step 4: Implement

Follow the implementation plan in `plan.md` task groups in order. For each task group:

1. Read `specs/mission.md` for design principles and `specs/tech-stack.md` for technical guidance.
2. Implement the changes for this task group.
3. Run `make test` to verify nothing is broken.
4. Use AskUserQuestion for any decisions not covered by the spec.
5. Commit the task group's changes following `specs/conventions.md`. Each commit should be minimal — containing only the changes necessary for its logical unit of work — and must pass all tests independently.

## Step 5: Verify

After implementation is complete:

1. Walk through each check in `verification.md` and confirm it passes.
2. Walk through each acceptance criterion in `requirements.md` and confirm it is met.
3. Run `make test`, `make lint`, and `make verify` one final time.
4. If any verification check or criterion fails, amend the fix into the commit where the issue was introduced rather than creating a new fixup commit.

## Step 6: Update governing docs

Review whether anything learned during implementation should be reflected in global SDD documents (`specs/mission.md`, `specs/tech-stack.md`, `specs/conventions.md`, `CLAUDE.md`, or any other top-level specs). If updates are needed, make them in a dedicated commit separate from the implementation commits.

## Step 7: Suggest review

Suggest the user run `/sdd-review` to review the changes for correctness and consistency before shipping.
