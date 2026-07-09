package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tclasen/Exaptra/stream"
)

const (
	NodeKindTask    = "task"
	NodeKindGate    = "gate"
	NodeKindSubplan = "subplan"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusPassed    = "passed"
	StatusBlocked   = "blocked"
	StatusRetrying  = "retrying"
)

// Node describes one stage in a graph plan.
type Node struct {
	ID              string             `json:"id"`
	Kind            string             `json:"kind"`
	Action          string             `json:"action,omitempty"`
	Subplan         string             `json:"subplan,omitempty"`
	OnSuccess       string             `json:"on_success,omitempty"`
	OnFailure       string             `json:"on_failure,omitempty"`
	OnMatch         string             `json:"on_match,omitempty"`
	OnMismatch      string             `json:"on_mismatch,omitempty"`
	ExpectStatus    string             `json:"expect_status,omitempty"`
	OutputContains  string             `json:"output_contains,omitempty"`
	RetryLimit      int                `json:"retry_limit,omitempty"`
	Workspace       string             `json:"workspace,omitempty"`
	SharedWorkspace bool               `json:"shared_workspace,omitempty"`
	Provenance      *stream.Provenance `json:"provenance,omitempty"`
}

// Plan is a serializable workflow graph.
type Plan struct {
	ID       string `json:"id"`
	Start    string `json:"start"`
	Nodes    []Node `json:"nodes"`
	Subplans []Plan `json:"subplans,omitempty"`
}

// TaskResult captures the output of a task node.
type TaskResult struct {
	Output     json.RawMessage    `json:"output,omitempty"`
	Provenance *stream.Provenance `json:"provenance,omitempty"`
}

// NodeRunner executes task nodes.
type NodeRunner interface {
	RunTask(context.Context, Node) (TaskResult, error)
}

// NodeRunnerFunc adapts a function to a NodeRunner.
type NodeRunnerFunc func(context.Context, Node) (TaskResult, error)

// RunTask executes the wrapped function.
func (f NodeRunnerFunc) RunTask(ctx context.Context, node Node) (TaskResult, error) {
	return f(ctx, node)
}

// Record captures one graph execution step.
type Record struct {
	PlanID     string             `json:"plan_id"`
	Depth      int                `json:"depth"`
	Node       Node               `json:"node"`
	Attempt    int                `json:"attempt,omitempty"`
	Status     string             `json:"status"`
	Matched    bool               `json:"matched,omitempty"`
	Next       string             `json:"next,omitempty"`
	Output     json.RawMessage    `json:"output,omitempty"`
	Provenance *stream.Provenance `json:"provenance,omitempty"`
	Error      string             `json:"error,omitempty"`
}

// Trace is the replayable execution record for one plan.
type Trace struct {
	PlanID    string   `json:"plan_id"`
	Completed int      `json:"completed"`
	Failed    int      `json:"failed"`
	Records   []Record `json:"records"`
	Plan      *Plan    `json:"plan,omitempty"`
}

// Executor executes a validated graph plan.
type Executor struct {
	runner NodeRunner
}

// NewExecutor constructs an executor.
func NewExecutor(runner NodeRunner) *Executor {
	return &Executor{runner: runner}
}

// Execute validates and runs a plan.
func (e *Executor) Execute(ctx context.Context, plan Plan) (Trace, error) {
	if e == nil || e.runner == nil {
		return Trace{}, errors.New("workflow: runner is required")
	}
	if err := plan.Validate(); err != nil {
		return Trace{}, err
	}

	clone := ClonePlan(&plan)
	trace, err := e.runPlan(ctx, *clone, 0)
	trace.Plan = clone
	return trace, err
}

func (e *Executor) runPlan(ctx context.Context, plan Plan, depth int) (Trace, error) {
	nodes := make(map[string]Node, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes[node.ID] = cloneNode(node)
	}
	subplans := make(map[string]Plan, len(plan.Subplans))
	for _, subplan := range plan.Subplans {
		subplans[subplan.ID] = *ClonePlan(&subplan)
	}

	trace := Trace{PlanID: plan.ID}
	current := plan.Start
	var previous *Record

	for current != "" {
		node, ok := nodes[current]
		if !ok {
			return trace, fmt.Errorf("workflow: node %q is missing from plan %q", current, plan.ID)
		}

		switch node.Kind {
		case NodeKindTask:
			record := e.runTaskNode(ctx, plan.ID, depth, node)
			trace.appendRecord(record)
			if record.Status == StatusFailed {
				current = node.OnFailure
			} else {
				current = node.OnSuccess
			}
			previous = &record
		case NodeKindGate:
			record, matched := gateRecord(plan.ID, depth, node, previous)
			trace.appendRecord(record)
			if matched {
				current = node.OnMatch
			} else {
				current = node.OnMismatch
			}
			previous = &record
		case NodeKindSubplan:
			subplan, ok := subplans[node.Subplan]
			if !ok {
				return trace, fmt.Errorf("workflow: subplan %q is not defined in plan %q", node.Subplan, plan.ID)
			}
			subtrace, err := e.runPlan(ctx, subplan, depth+1)
			for _, record := range subtrace.Records {
				trace.appendRecord(record)
			}
			summary := summarizeSubplan(subtrace)
			record := Record{
				PlanID:     plan.ID,
				Depth:      depth,
				Node:       cloneNode(node),
				Status:     subplanStatus(subtrace, err),
				Next:       selectNext(node.OnSuccess, node.OnFailure, subtrace, err),
				Output:     summary,
				Provenance: cloneProvenance(node.Provenance),
			}
			if err != nil {
				record.Error = err.Error()
			}
			trace.appendRecord(record)
			if err != nil || subtrace.Failed > 0 {
				current = node.OnFailure
			} else {
				current = node.OnSuccess
			}
			previous = &record
		default:
			return trace, fmt.Errorf("workflow: unsupported node kind %q in plan %q", node.Kind, plan.ID)
		}
	}

	trace.Completed = countRecords(trace.Records, StatusCompleted)
	trace.Failed = countRecords(trace.Records, StatusFailed)
	return trace, nil
}

func (e *Executor) runTaskNode(ctx context.Context, planID string, depth int, node Node) Record {
	attempts := node.RetryLimit + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := e.runner.RunTask(ctx, cloneNode(node))
		if err == nil {
			return Record{
				PlanID:     planID,
				Depth:      depth,
				Node:       cloneNode(node),
				Attempt:    attempt,
				Status:     StatusCompleted,
				Output:     cloneJSON(result.Output),
				Provenance: mergeProvenance(node.Provenance, result.Provenance, node.ID),
			}
		}

		status := StatusFailed
		if attempt < attempts {
			status = StatusRetrying
		}
		record := Record{
			PlanID:     planID,
			Depth:      depth,
			Node:       cloneNode(node),
			Attempt:    attempt,
			Status:     status,
			Provenance: cloneProvenance(node.Provenance),
		}
		if err != nil {
			record.Error = err.Error()
		}
		if status == StatusRetrying {
			record.Next = node.ID
		}
		if attempt < attempts {
			continue
		}
		return record
	}
	return Record{PlanID: planID, Depth: depth, Node: cloneNode(node), Status: StatusFailed}
}

func gateRecord(planID string, depth int, node Node, previous *Record) (Record, bool) {
	matched := true
	if node.ExpectStatus != "" {
		if previous == nil {
			matched = false
		} else {
			matched = previous.Status == node.ExpectStatus
		}
	}
	if matched && node.OutputContains != "" {
		if previous == nil {
			matched = false
		} else {
			matched = strings.Contains(string(previous.Output), node.OutputContains)
		}
	}

	status := StatusPassed
	if !matched {
		status = StatusBlocked
	}
	record := Record{
		PlanID:     planID,
		Depth:      depth,
		Node:       cloneNode(node),
		Status:     status,
		Matched:    matched,
		Provenance: cloneProvenance(node.Provenance),
	}
	return record, matched
}

func selectNext(success, failure string, subtrace Trace, err error) string {
	if err != nil || subtrace.Failed > 0 {
		return failure
	}
	return success
}

func subplanStatus(subtrace Trace, err error) string {
	if err != nil || subtrace.Failed > 0 {
		return StatusFailed
	}
	return StatusCompleted
}

func summarizeSubplan(subtrace Trace) json.RawMessage {
	payload, err := json.Marshal(map[string]any{
		"plan_id":   subtrace.PlanID,
		"completed": subtrace.Completed,
		"failed":    subtrace.Failed,
	})
	if err != nil {
		return json.RawMessage(`{"error":"failed to summarize subplan"}`)
	}
	return payload
}

func countRecords(records []Record, status string) int {
	total := 0
	for _, record := range records {
		if record.Status == status {
			total++
		}
	}
	return total
}

func (t *Trace) appendRecord(record Record) {
	t.Records = append(t.Records, cloneRecord(record))
}

// Validate ensures the plan is structurally sound and free of cycles.
func (p Plan) Validate() error {
	return validatePlan(p, map[string]struct{}{}, map[string]struct{}{})
}

func validatePlan(plan Plan, validated, recursionStack map[string]struct{}) error {
	if plan.ID == "" {
		return errors.New("workflow: plan id is required")
	}
	if plan.Start == "" {
		return fmt.Errorf("workflow: plan %q start node is required", plan.ID)
	}
	if _, ok := recursionStack[plan.ID]; ok {
		return fmt.Errorf("workflow: plan %q recursively references itself", plan.ID)
	}
	recursionStack[plan.ID] = struct{}{}
	defer delete(recursionStack, plan.ID)
	if _, ok := validated[plan.ID]; ok {
		return nil
	}
	validated[plan.ID] = struct{}{}

	nodes := make(map[string]Node, len(plan.Nodes))
	for _, node := range plan.Nodes {
		if node.ID == "" {
			return fmt.Errorf("workflow: plan %q contains a node without an id", plan.ID)
		}
		if _, ok := nodes[node.ID]; ok {
			return fmt.Errorf("workflow: plan %q contains duplicate node id %q", plan.ID, node.ID)
		}
		if node.Kind != NodeKindTask && node.Kind != NodeKindGate && node.Kind != NodeKindSubplan {
			return fmt.Errorf("workflow: node %q has unsupported kind %q", node.ID, node.Kind)
		}
		if node.Kind == NodeKindTask && node.Action == "" {
			return fmt.Errorf("workflow: task node %q requires an action", node.ID)
		}
		if node.RetryLimit < 0 {
			return fmt.Errorf("workflow: node %q has a negative retry limit", node.ID)
		}
		if node.Kind == NodeKindGate && node.ExpectStatus == "" && node.OutputContains == "" {
			return fmt.Errorf("workflow: gate node %q requires an expectation", node.ID)
		}
		if node.Kind == NodeKindSubplan && node.Subplan == "" {
			return fmt.Errorf("workflow: subplan node %q requires a subplan reference", node.ID)
		}
		nodes[node.ID] = node
	}
	if _, ok := nodes[plan.Start]; !ok {
		return fmt.Errorf("workflow: plan %q start node %q does not exist", plan.ID, plan.Start)
	}

	for _, node := range plan.Nodes {
		switch node.Kind {
		case NodeKindTask:
			for _, next := range []string{node.OnSuccess, node.OnFailure} {
				if err := validateEdge(plan.ID, nodes, node.ID, next); err != nil {
					return err
				}
			}
		case NodeKindGate:
			for _, next := range []string{node.OnMatch, node.OnMismatch} {
				if err := validateEdge(plan.ID, nodes, node.ID, next); err != nil {
					return err
				}
			}
		case NodeKindSubplan:
			for _, next := range []string{node.OnSuccess, node.OnFailure} {
				if err := validateEdge(plan.ID, nodes, node.ID, next); err != nil {
					return err
				}
			}
			if _, ok := subplanByID(plan.Subplans, node.Subplan); !ok {
				return fmt.Errorf("workflow: subplan node %q references unknown subplan %q", node.ID, node.Subplan)
			}
		}
	}

	for _, subplan := range plan.Subplans {
		if err := validatePlan(subplan, validated, recursionStack); err != nil {
			return err
		}
	}

	return validateAcyclic(plan.Start, nodes, map[string]bool{}, map[string]bool{})
}

func validateEdge(planID string, nodes map[string]Node, from, to string) error {
	if to == "" {
		return nil
	}
	if _, ok := nodes[to]; !ok {
		return fmt.Errorf("workflow: plan %q node %q references unknown node %q", planID, from, to)
	}
	return nil
}

func validateAcyclic(start string, nodes map[string]Node, visiting map[string]bool, visited map[string]bool) error {
	if visiting[start] {
		return fmt.Errorf("workflow: cycle detected at node %q", start)
	}
	if visited[start] {
		return nil
	}
	visiting[start] = true
	defer delete(visiting, start)
	visited[start] = true

	node := nodes[start]
	for _, next := range outgoingEdges(node) {
		if next == "" {
			continue
		}
		if err := validateAcyclic(next, nodes, visiting, visited); err != nil {
			return err
		}
	}
	return nil
}

func outgoingEdges(node Node) []string {
	switch node.Kind {
	case NodeKindTask:
		return []string{node.OnSuccess, node.OnFailure}
	case NodeKindGate:
		return []string{node.OnMatch, node.OnMismatch}
	case NodeKindSubplan:
		return []string{node.OnSuccess, node.OnFailure}
	default:
		return nil
	}
}

func subplanByID(subplans []Plan, id string) (Plan, bool) {
	for _, subplan := range subplans {
		if subplan.ID == id {
			return subplan, true
		}
	}
	return Plan{}, false
}

// ClonePlan returns a deep copy of a plan.
func ClonePlan(plan *Plan) *Plan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	if len(plan.Nodes) != 0 {
		cloned.Nodes = make([]Node, len(plan.Nodes))
		for i, node := range plan.Nodes {
			cloned.Nodes[i] = cloneNode(node)
		}
	}
	if len(plan.Subplans) != 0 {
		cloned.Subplans = make([]Plan, len(plan.Subplans))
		for i, subplan := range plan.Subplans {
			cloned.Subplans[i] = *ClonePlan(&subplan)
		}
	}
	return &cloned
}

// CloneTrace returns a deep copy of a trace.
func CloneTrace(trace *Trace) *Trace {
	if trace == nil {
		return nil
	}
	cloned := *trace
	if len(trace.Records) != 0 {
		cloned.Records = make([]Record, len(trace.Records))
		for i, record := range trace.Records {
			cloned.Records[i] = cloneRecord(record)
		}
	}
	cloned.Plan = ClonePlan(trace.Plan)
	return &cloned
}

func cloneNode(node Node) Node {
	node.Provenance = cloneProvenance(node.Provenance)
	return node
}

func cloneRecord(record Record) Record {
	record.Node = cloneNode(record.Node)
	record.Output = cloneJSON(record.Output)
	record.Provenance = cloneProvenance(record.Provenance)
	return record
}

func cloneProvenance(provenance *stream.Provenance) *stream.Provenance {
	if provenance == nil {
		return nil
	}
	cloned := *provenance
	return &cloned
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

func mergeProvenance(base, result *stream.Provenance, component string) *stream.Provenance {
	provenance := cloneProvenance(base)
	if provenance == nil {
		provenance = &stream.Provenance{}
	}
	if provenance.Source == "" {
		provenance.Source = "workflow"
	}
	if provenance.Component == "" {
		provenance.Component = component
	}
	if result != nil {
		if result.Source != "" {
			provenance.Source = result.Source
		}
		if result.Provider != "" {
			provenance.Provider = result.Provider
		}
		if result.Model != "" {
			provenance.Model = result.Model
		}
		if result.Component != "" {
			provenance.Component = result.Component
		}
		if result.TraceID != "" {
			provenance.TraceID = result.TraceID
		}
	}
	if provenance.TraceID == "" {
		provenance.TraceID = component
	}
	return provenance
}
