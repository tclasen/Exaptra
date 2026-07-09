# AGENTS.md

Language-agnostic engineering rules for agents working in this repository.

## Core Rule

Deliver correct, maintainable software in small verified increments.

## Workflow

1. Read relevant code and docs before editing.
2. Make the smallest coherent change that advances the task.
3. Run the fastest relevant automated check.
4. Fix failures before continuing.
5. Repeat in small batches.
6. Before handoff, run the full relevant validation available for the change.

Prefer targeted checks while developing, then broader checks before finalizing.
Report exactly what was run and what passed or failed. If checks cannot be run,
say why.

## GitHub Ticket Workflow

Every task must be tracked by a GitHub issue on the project board, including
interactive or conversational work.

Before picking up new work, first scan for partially completed or abandoned
work so resumption happens before new task selection. Check the current branch,
local changes, open PRs, and related issues for unfinished work that should be
continued, closed, or decomposed instead of starting fresh.

Before starting implementation:

1. Find the GitHub issue associated with the requested work.
2. If no issue exists, create one and add it to the project board.
3. If the issue depends on missing prerequisite work, create prerequisite
   issues, add them to the project board, and complete prerequisites first.
4. If the issue is too large or can be usefully decomposed, create smaller
   issues and work them independently, even when that results in multiple pull
   requests for one user request.
5. Create a new branch for the active issue and keep all work associated with
   that issue.

After implementation:

1. Commit the completed work with an atomic Conventional Commit.
2. Push the branch to the remote.
3. Open a pull request linked to the issue.
4. Create a sub-agent with a fresh context window to review the pull request and
   place review comments.
5. Resolve all review comments.
6. Merge the pull request only after review comments are resolved and checks
   pass.
7. Close the issue when the work satisfies it.

Always watch for missing tickets, prerequisite work, and opportunities to split
oversized tickets. Keep active work associated with exactly one current issue
unless deliberately decomposing it into multiple issues.

### Next Task Requests

When the user asks to "work on the next task" or uses similar phrasing:

1. Search the open tickets on the project board for the highest-priority task.
2. Work on that task using the GitHub Ticket Workflow.
3. Exit after the workflow is complete.

### Unspecified Work Requests

When the user asks you to work, continue, or make progress without naming a
specific task:

1. First check for any hanging tickets or pull requests that need to be
   resumed.
2. If a hanging ticket or PR exists, resume that work before starting new
   work.
3. If nothing is hanging, follow the Next Task Requests workflow.

## Human-On-The-Loop

When a task needs additional human feedback or approval:

- Tag the user in the issue thread and pause that work until feedback arrives.
- When feedback arrives, apply it before continuing, and ask follow-up
  questions only if they are required to proceed.
- For high-risk or high-importance pull requests, request review from the
  project human expert before merging. Infer the best reviewer from repository
  ownership, maintainership metadata, or recent relevant commit history.
- If the project human expert leaves feedback on a pull request, address it and
  request a new review before merging.
- If a task is blocked waiting for human approval or feedback, switch to any
  other unblocked work instead of waiting on the blocked task.
- Use the inferred project human expert as the default reviewer for
  human-on-the-loop checks.

## Small Batches

Keep each batch focused on one intent:

- one feature slice
- one bug fix
- one refactor
- one test improvement
- one docs update
- one dependency or configuration change

Do not mix unrelated behavior changes, refactors, formatting, dependency
updates, or generated files.

Prefer vertical slices over horizontal layers. A good slice delivers a thin,
working path through the system, with tests, rather than isolated infrastructure
that cannot yet be exercised end to end.

## Commits

When committing is allowed, commit frequently after each verified batch.

Every commit must be atomic:

- one logical change
- relevant tests or docs included
- no unrelated files
- repository left in a working state
- generated files included only when intentionally tracked

When an agent makes changes and creates a commit, it must add itself as a
co-author using a standard `Co-authored-by:` trailer.

Use Conventional Commits:

```text
<type>[optional scope]: <description>
```

Common types:

- `feat`: new capability
- `fix`: bug fix
- `docs`: documentation
- `test`: tests
- `refactor`: behavior-preserving restructuring
- `perf`: performance
- `style`: formatting only
- `build`: build or dependency changes
- `ci`: CI changes
- `chore`: maintenance
- `revert`: revert a prior commit

Use `!` or a `BREAKING CHANGE:` footer for breaking changes. If one change
needs multiple commit types, split it.

## Testing

Behavior changes require tests. Bug fixes require regression tests when
practical.

Choose the right level:

- unit tests for local logic
- integration tests for boundaries
- contract tests for APIs, schemas, protocols, and tools
- smoke or end-to-end tests for critical workflows

Do not skip, weaken, or delete tests just to make a suite pass.

## Design

Prefer simple, explicit designs.

- Follow existing project patterns.
- Keep responsibilities cohesive.
- Make dependencies and side effects clear.
- Avoid speculative abstractions.
- Separate refactors from behavior changes.
- Treat public APIs, data formats, protocols, and configuration as contracts.
- Build shared layers only when a concrete vertical slice proves they are
  needed.

## Security

Apply secure defaults:

- validate inputs at trust boundaries
- avoid leaking secrets in code, logs, errors, tests, or docs
- use least privilege
- prefer vetted libraries over custom security code
- treat shell execution, deserialization, dynamic loading, plugins, and external
  tools as high-risk boundaries
- keep dependencies minimal, maintained, locked, and license-compatible

## Agent Conduct

- Preserve user changes.
- Never revert unrelated work.
- Ask before destructive actions.
- Keep edits scoped to the request.
- Document meaningful behavior, setup, config, architecture, and security
  changes.
- Surface uncertainty and remaining risk clearly.

## Self-Evolution

Agents may update this file when a change would improve future agent loops.
Keep edits small, general, and durable. Do not add task-specific notes,
temporary preferences, or instructions that conflict with higher-priority user,
system, or developer guidance.

## Done Means

- Requested change is complete.
- Work is associated with a GitHub issue on the project board.
- Pull request review comments are resolved.
- Relevant checks pass.
- Tests/docs are updated as needed.
- No unrelated edits are included.
- Commit history, if created, is atomic and uses Conventional Commits.
- The pull request is merged and the issue is closed when satisfied.
- Final response lists checks run and any skipped checks or risks.

## References

- Google Engineering Practices: https://google.github.io/eng-practices/
- Conventional Commits: https://www.conventionalcommits.org/
- Continuous Integration: https://martinfowler.com/articles/continuousIntegration.html
- Trunk-Based Development: https://trunkbaseddevelopment.com/
- NIST SSDF: https://csrc.nist.gov/projects/ssdf
- OWASP Secure Coding: https://owasp.org/www-project-secure-coding-practices-quick-reference-guide/
- SLSA: https://slsa.dev/
