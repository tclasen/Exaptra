# Exaptra

Exaptra is an experimental agent harness designed around one core principle:
every meaningful part of an agent should be externalized, inspectable, and
replaceable.

The project aims to separate the agent runtime from the capabilities it uses.
Instead of embedding native tools directly into the harness, Exaptra will use
external tool providers, starting with MCP, so tool access, permissions,
configuration, and behavior can be managed outside the core agent loop.

## Objectives

- Build an agent harness with a small, explicit runtime core.
- Externalize tools, memory, context management, references, and other agent
  capabilities wherever possible.
- Support MCP as the primary mechanism for exposing tools to agents.
- Avoid native built-in tools in the harness itself.
- Make tool availability and top-level agent capabilities dynamic rather than
  fixed at startup.
- Provide a model for advanced stream-level capabilities through meta tools.

## Core Concepts

### Externalized Tools

In Exaptra, tools are not intended to be hardcoded into the agent runtime. The
harness should discover and use tools through external interfaces such as MCP.
This keeps the runtime smaller and makes tool behavior easier to audit, replace,
compose, and permission independently.

### Meta Tools

Meta tools are a proposed class of tool that operate on the conversation or
agent state at a higher level than ordinary tools.

Traditional tools append results back into the chat stream. A meta tool can
instead operate on the stream itself or on the top-level agent configuration
around that stream. This allows capabilities that change how the agent sees,
stores, compresses, or extends its working context.

Examples of meta tool capabilities include:

- Compaction of long conversation history.
- Memory creation, recall, update, and deletion.
- Progressive disclosure of context and references.
- Adding or removing available tools.
- Adding or removing top-level references or resources.
- Rewriting, filtering, or restructuring parts of the working stream.

### Agent Stream

The agent stream is the working context the agent uses to reason and act. In
most systems, this is treated as an append-only chat log. Exaptra treats the
stream as a managed runtime object that can be inspected and transformed by
authorized meta tools.

### Tracker Writes

Tracker writes are explicit orchestrator operations for comments, workflow
state changes, and PR handoff links. They carry run and issue identity,
record provenance, and are intended to sit behind an adapter boundary so
different tracker providers can be supported over time.

### Fan-Out / Fan-In

Long-horizon tasks can be decomposed into bounded fan-out batches of subagent
work, then merged back into a parent run with ordered outcomes and explicit
provenance for each result.

### Graph Plans

Run stages can also be represented as graph plans with gates and reusable
subpipelines. A graph trace records the plan structure, branching decisions,
and nested subplan execution so the run can be inspected or replayed later.

### Provider Profiles

Provider-aligned profiles can shape tool exposure and prompt composition for a
given model and workflow. The example run resolves a profile from the active
provider and workflow, then records that selection in the run snapshot.

## Design Direction

Exaptra is intended to explore an agent architecture where the core harness is
responsible for orchestration, policy boundaries, and state transitions, while
capabilities are supplied externally.

The long-term design should make these questions explicit:

- What capabilities are available to the agent?
- Where did each capability come from?
- What permissions does each capability have?
- What parts of the stream or runtime state can it read or modify?
- How are changes to the stream represented, validated, and audited?
- How can capabilities be added, removed, or scoped during a run?

## Non-Goals

- Embedding a large standard library of native tools in the harness.
- Treating the chat stream as the only place where agent state can change.
- Hiding memory, compaction, or context management behind implicit runtime
  behavior.
- Coupling the agent runtime to a single provider, model, or tool transport.

## Current Status

This repository is at the project framing stage. The initial focus is defining
the architecture, terminology, and boundaries for an externalized agent harness
with MCP tools and stream-level meta tools.

## Getting Started

1. Set the example model secret:

```bash
export EXAPTRA_MODEL_API_KEY=example-secret
```

2. Run the full test suite:

```bash
go test ./...
```

3. Run the local example session:

```bash
go run ./cmd/example-run -config examples/localrun/config.example.json
```

The example discovers a local MCP tool, invokes it, compacts the stream, and
prints a redacted debug snapshot of the run.

## Terminology

- Runtime: the orchestration layer that coordinates model, tool, and state
  transitions.
- Capability: an inspectable action the runtime can authorize, expose, or
  deny.
- Ordinary tool: a non-meta tool that returns a normal tool result.
- Meta tool: a higher-level operation that can inspect or modify stream or
  runtime state.
- Stream: the ordered working record of messages, tool calls, tool results,
  and meta transitions.
- Transition: a validated state change recorded with before/after context.
- Provider: the external implementation that supplies a model or tool
  capability.
- Permission: an explicit decision about whether a capability may read or
  modify state.
- Provenance: metadata that explains where a record came from and what created
  it.

## Configuration

The example configuration lives at
[`examples/localrun/config.example.json`](examples/localrun/config.example.json).
It demonstrates secret-safe loading: the model API key is read from the
`EXAPTRA_MODEL_API_KEY` environment variable instead of being committed in
plain text.

Provider-specific environment values in debug snapshots are redacted before
serialization. The example run prints the redacted config, stream trajectory,
tracker audit records, provider profile, orchestration results, workflow graph
traces, tool registry state, and meta audit records.

## MVP Limitations

- The local example uses a simple built-in provider implementation rather than
  a networked MCP transport.
- The current MVP focuses on explicit state boundaries and auditability, not a
  full production scheduler or memory subsystem.
- Meta tools are intentionally narrow and are limited to the concrete
  transitions implemented in this repository.
- The core loop is still an experimental harness, not a finished agent
  runtime.

## Validation

Maintainers can validate the current MVP with:

```bash
make validate
go test ./...
go run ./cmd/example-run -config examples/localrun/config.example.json
```
