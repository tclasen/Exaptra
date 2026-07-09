package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/tclasen/Exaptra/stream"
)

const (
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Task describes one subagent unit of work.
type Task struct {
	ID              string             `json:"id"`
	Prompt          string             `json:"prompt"`
	Workspace       string             `json:"workspace,omitempty"`
	SharedWorkspace bool               `json:"shared_workspace"`
	Provenance      *stream.Provenance `json:"provenance,omitempty"`
}

// Batch groups tasks under one parent run for fan-out execution.
type Batch struct {
	ParentRunID string `json:"parent_run_id,omitempty"`
	Tasks       []Task `json:"tasks"`
}

// TaskResult captures the worker output for a task.
type TaskResult struct {
	Output     json.RawMessage    `json:"output,omitempty"`
	Provenance *stream.Provenance `json:"provenance,omitempty"`
}

// Outcome records the finished state of one task.
type Outcome struct {
	Task       Task               `json:"task"`
	Status     string             `json:"status"`
	Output     json.RawMessage    `json:"output,omitempty"`
	Provenance *stream.Provenance `json:"provenance,omitempty"`
	Error      string             `json:"error,omitempty"`
}

// Aggregate is the merged fan-in result for one batch.
type Aggregate struct {
	ParentRunID    string    `json:"parent_run_id,omitempty"`
	MaxConcurrency int       `json:"max_concurrency"`
	Completed      int       `json:"completed"`
	Failed         int       `json:"failed"`
	Outcomes       []Outcome `json:"outcomes"`
}

// Worker executes one task.
type Worker interface {
	RunTask(context.Context, Task) (TaskResult, error)
}

// WorkerFunc adapts a function into a Worker.
type WorkerFunc func(context.Context, Task) (TaskResult, error)

// RunTask executes the wrapped function.
func (f WorkerFunc) RunTask(ctx context.Context, task Task) (TaskResult, error) {
	return f(ctx, task)
}

// Executor runs fan-out batches with bounded concurrency.
type Executor struct {
	worker         Worker
	maxConcurrency int
}

// NewExecutor constructs an executor.
func NewExecutor(worker Worker, maxConcurrency int) *Executor {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	return &Executor{
		worker:         worker,
		maxConcurrency: maxConcurrency,
	}
}

// Execute fans out the batch, waits for completion, and fan-ins deterministic outcomes.
func (e *Executor) Execute(ctx context.Context, batch Batch) (Aggregate, error) {
	if e == nil || e.worker == nil {
		return Aggregate{}, errors.New("orchestration: worker is required")
	}

	aggregate := Aggregate{
		ParentRunID:    batch.ParentRunID,
		MaxConcurrency: e.maxConcurrency,
		Outcomes:       make([]Outcome, len(batch.Tasks)),
	}
	if len(batch.Tasks) == 0 {
		return aggregate, nil
	}

	sem := make(chan struct{}, e.maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, task := range batch.Tasks {
		wg.Add(1)
		go func(index int, task Task) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				outcome := failedOutcome(task, ctx.Err())
				mu.Lock()
				aggregate.Outcomes[index] = outcome
				aggregate.Failed++
				mu.Unlock()
				return
			}
			defer func() { <-sem }()

			result, err := e.worker.RunTask(ctx, cloneTask(task))
			if err != nil {
				outcome := failedOutcome(task, err)
				mu.Lock()
				aggregate.Outcomes[index] = outcome
				aggregate.Failed++
				mu.Unlock()
				return
			}

			outcome := Outcome{
				Task:       cloneTask(task),
				Status:     StatusCompleted,
				Output:     cloneJSON(result.Output),
				Provenance: mergeProvenance(task.Provenance, result.Provenance, task.ID),
			}
			mu.Lock()
			aggregate.Outcomes[index] = outcome
			aggregate.Completed++
			mu.Unlock()
		}(i, task)
	}

	wg.Wait()

	if err := ctx.Err(); err != nil {
		return aggregate, err
	}
	return aggregate, nil
}

// CloneAggregate returns a deep copy of the aggregate.
func CloneAggregate(aggregate *Aggregate) *Aggregate {
	if aggregate == nil {
		return nil
	}
	cloned := *aggregate
	if len(aggregate.Outcomes) != 0 {
		cloned.Outcomes = make([]Outcome, len(aggregate.Outcomes))
		for i, outcome := range aggregate.Outcomes {
			cloned.Outcomes[i] = cloneOutcome(outcome)
		}
	}
	return &cloned
}

func failedOutcome(task Task, err error) Outcome {
	outcome := Outcome{
		Task:   cloneTask(task),
		Status: StatusFailed,
	}
	if err != nil {
		outcome.Error = err.Error()
	}
	outcome.Provenance = mergeProvenance(task.Provenance, nil, task.ID)
	return outcome
}

func mergeProvenance(base, result *stream.Provenance, taskID string) *stream.Provenance {
	provenance := cloneProvenance(base)
	if provenance == nil {
		provenance = &stream.Provenance{}
	}
	if provenance.Source == "" {
		provenance.Source = "subagent"
	}
	if provenance.Component == "" {
		provenance.Component = taskID
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
		provenance.TraceID = taskID
	}
	return provenance
}

func cloneTask(task Task) Task {
	task.Provenance = cloneProvenance(task.Provenance)
	return task
}

func cloneOutcome(outcome Outcome) Outcome {
	outcome.Task = cloneTask(outcome.Task)
	outcome.Output = cloneJSON(outcome.Output)
	outcome.Provenance = cloneProvenance(outcome.Provenance)
	return outcome
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

func (a Aggregate) String() string {
	return fmt.Sprintf("parent=%s completed=%d failed=%d", a.ParentRunID, a.Completed, a.Failed)
}
