---
policy:
  mode: gate
  labels:
    - phase:5-orchestration
  blockers:
    - waiting on prerequisite issues
prompts:
  orchestrator: coordinate the slice and preserve issue scope
  research: gather source material before editing
  validate: confirm the handoff state and tracker writes
hooks:
  - event: before_run
    command: ./scripts/validate.sh
runtime:
  shared_workspace: true
  max_concurrency: 2
---

# Workflow Contract

Issue: #{{.Issue.Number}} {{.Issue.Title}}

Owner: {{.Issue.Owner}}/{{.Issue.Repo}}

Labels: {{join .Issue.Labels ", "}}

Blockers: {{join .Issue.Blockers ", "}}

Instructions: {{.Issue.Instructions}}

The orchestrator should treat this file as the authoritative run contract for the issue slice.
