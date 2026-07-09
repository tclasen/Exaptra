#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export EXAPTRA_MODEL_API_KEY="${EXAPTRA_MODEL_API_KEY:-validation-secret}"

go test ./...

output="$(go run ./cmd/example-run -config examples/localrun/config.example.json)"

printf '%s\n' "$output" | grep -Fq '"type": "function_call"'
printf '%s\n' "$output" | grep -Fq '"type": "function_call_output"'
printf '%s\n' "$output" | grep -Fq '"type": "exaptra:meta_transition"'
printf '%s\n' "$output" | grep -Fq '"type": "exaptra:tracker_comment"'
printf '%s\n' "$output" | grep -Fq '"state": "review_ready"'
printf '%s\n' "$output" | grep -Fq '"availability": "exposed"'
printf '%s\n' "$output" | grep -Fq '"api_key": ""'
printf '%s\n' "$output" | grep -Fq '[redacted]'
