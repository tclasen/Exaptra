#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export EXAPTRA_MODEL_API_KEY="${EXAPTRA_MODEL_API_KEY:-validation-secret}"

go test ./config ./execution ./mcp ./meta ./orchestration ./profiles ./runtrace ./stream ./tracker ./workflow ./workspace ./workflowdoc ./cmd/example-run/internal/app

output="$(go run ./cmd/example-run -config examples/localrun/config.example.json)"

printf '%s\n' "$output" | grep -Fq '"type": "function_call"'
printf '%s\n' "$output" | grep -Fq '"type": "function_call_output"'
printf '%s\n' "$output" | grep -Fq '"type": "exaptra:meta_transition"'
printf '%s\n' "$output" | grep -Fq '"type": "exaptra:tracker_comment"'
printf '%s\n' "$output" | grep -Fq '"type": "exaptra:tracker_pr_link"'
printf '%s\n' "$output" | grep -Fq '"state": "review_ready"'
printf '%s\n' "$output" | grep -Fq '"pull_request": {'
printf '%s\n' "$output" | grep -Fq '"profile": {'
printf '%s\n' "$output" | grep -Fq '"execution": {'
printf '%s\n' "$output" | grep -Fq '"kind": "local"'
printf '%s\n' "$output" | grep -Fq '"workspace": {'
printf '%s\n' "$output" | grep -Fq '.exaptra/workspaces/tclasen/exaptra/52'
printf '%s\n' "$output" | grep -Fq '"orchestration": {'
printf '%s\n' "$output" | grep -Fq '"workflow": {'
printf '%s\n' "$output" | grep -Fq '"kind": "gate"'
printf '%s\n' "$output" | grep -Fq '[local/example-model:research] summarize the lookup output'
printf '%s\n' "$output" | grep -Fq '[local/example-model:validate] confirm handoff state and tracker writes'
printf '%s\n' "$output" | grep -Fq '"shared_workspace": true'
printf '%s\n' "$output" | grep -Fq '"availability": "exposed"'
printf '%s\n' "$output" | grep -Fq '"api_key": ""'
printf '%s\n' "$output" | grep -Fq '[redacted]'
