package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tclasen/Exaptra/stream"
)

func TestExecutorPreservesTaskOrderAndProvenance(t *testing.T) {
	executor := NewExecutor(WorkerFunc(func(ctx context.Context, task Task) (TaskResult, error) {
		if task.ID == "slow" {
			time.Sleep(40 * time.Millisecond)
		} else {
			time.Sleep(5 * time.Millisecond)
		}
		payload, _ := json.Marshal(map[string]any{"task": task.ID})
		return TaskResult{
			Output: payload,
			Provenance: &stream.Provenance{
				Source:    "subagent",
				Component: task.ID,
				TraceID:   task.ID + "-trace",
			},
		}, nil
	}), 2)

	aggregate, err := executor.Execute(context.Background(), Batch{
		ParentRunID: "run-1",
		Tasks: []Task{
			{ID: "slow", Prompt: "research"},
			{ID: "fast", Prompt: "validate", SharedWorkspace: true, Workspace: "shared"},
		},
	})
	if err != nil {
		t.Fatalf("execute batch: %v", err)
	}
	if aggregate.Completed != 2 || aggregate.Failed != 0 {
		t.Fatalf("aggregate counts = completed %d failed %d", aggregate.Completed, aggregate.Failed)
	}
	if aggregate.MaxConcurrency != 2 {
		t.Fatalf("max concurrency = %d, want 2", aggregate.MaxConcurrency)
	}
	if got := aggregate.Outcomes[0].Task.ID; got != "slow" {
		t.Fatalf("first outcome task = %q, want slow", got)
	}
	if got := aggregate.Outcomes[1].Task.ID; got != "fast" {
		t.Fatalf("second outcome task = %q, want fast", got)
	}
	if aggregate.Outcomes[0].Provenance == nil || aggregate.Outcomes[0].Provenance.TraceID != "slow-trace" {
		t.Fatalf("slow provenance = %#v, want trace id slow-trace", aggregate.Outcomes[0].Provenance)
	}
	if aggregate.Outcomes[1].Provenance == nil || aggregate.Outcomes[1].Provenance.Component != "fast" {
		t.Fatalf("fast provenance = %#v, want component fast", aggregate.Outcomes[1].Provenance)
	}
}

func TestExecutorHonorsBoundedConcurrency(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{})
	var active int32
	var peak int32

	executor := NewExecutor(WorkerFunc(func(ctx context.Context, task Task) (TaskResult, error) {
		cur := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if cur <= old {
				break
			}
			if atomic.CompareAndSwapInt32(&peak, old, cur) {
				break
			}
		}
		started <- task.ID
		<-release
		atomic.AddInt32(&active, -1)
		payload, _ := json.Marshal(map[string]any{"task": task.ID})
		return TaskResult{Output: payload}, nil
	}), 2)

	done := make(chan struct {
		aggregate Aggregate
		err       error
	}, 1)
	go func() {
		aggregate, err := executor.Execute(context.Background(), Batch{
			ParentRunID: "run-2",
			Tasks: []Task{
				{ID: "one"},
				{ID: "two"},
				{ID: "three"},
			},
		})
		done <- struct {
			aggregate Aggregate
			err       error
		}{aggregate: aggregate, err: err}
	}()

	first := <-started
	second := <-started
	if first == second {
		t.Fatal("expected distinct tasks to start first")
	}
	select {
	case third := <-started:
		t.Fatalf("third task started before a slot opened: %s", third)
	case <-time.After(50 * time.Millisecond):
	}

	release <- struct{}{}
	select {
	case third := <-started:
		if third == first || third == second {
			t.Fatalf("unexpected repeated task start: %s", third)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("third task did not start after release")
	}
	release <- struct{}{}
	release <- struct{}{}

	result := <-done
	if result.err != nil {
		t.Fatalf("execute batch: %v", result.err)
	}
	if atomic.LoadInt32(&peak) > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", peak)
	}
	if result.aggregate.Completed != 3 {
		t.Fatalf("completed = %d, want 3", result.aggregate.Completed)
	}
}

func TestExecutorIsolatesFailures(t *testing.T) {
	executor := NewExecutor(WorkerFunc(func(ctx context.Context, task Task) (TaskResult, error) {
		if task.ID == "fail" {
			return TaskResult{}, errors.New("boom")
		}
		payload, _ := json.Marshal(map[string]any{"task": task.ID})
		return TaskResult{Output: payload}, nil
	}), 2)

	aggregate, err := executor.Execute(context.Background(), Batch{
		ParentRunID: "run-3",
		Tasks: []Task{
			{ID: "ok-1"},
			{ID: "fail"},
			{ID: "ok-2", SharedWorkspace: true},
		},
	})
	if err != nil {
		t.Fatalf("execute batch: %v", err)
	}
	if aggregate.Completed != 2 || aggregate.Failed != 1 {
		t.Fatalf("aggregate counts = completed %d failed %d", aggregate.Completed, aggregate.Failed)
	}
	if aggregate.Outcomes[1].Status != StatusFailed {
		t.Fatalf("failing outcome status = %q, want failed", aggregate.Outcomes[1].Status)
	}
	if aggregate.Outcomes[1].Error != "boom" {
		t.Fatalf("failing outcome error = %q, want boom", aggregate.Outcomes[1].Error)
	}
	if aggregate.Outcomes[0].Status != StatusCompleted || aggregate.Outcomes[2].Status != StatusCompleted {
		t.Fatalf("successful outcomes were not preserved: %#v", aggregate.Outcomes)
	}
}

func TestCloneAggregateDeepCopies(t *testing.T) {
	original := &Aggregate{
		ParentRunID:    "run-4",
		MaxConcurrency: 2,
		Outcomes: []Outcome{{
			Task: Task{
				ID:         "task-1",
				Provenance: &stream.Provenance{Source: "subagent", Component: "task-1"},
			},
			Output:     json.RawMessage(`{"ok":true}`),
			Provenance: &stream.Provenance{TraceID: "trace-1"},
		}},
	}

	cloned := CloneAggregate(original)
	cloned.Outcomes[0].Output[0] = 'x'
	cloned.Outcomes[0].Task.Provenance.Source = "mutated"
	cloned.Outcomes[0].Provenance.TraceID = "mutated"

	if string(original.Outcomes[0].Output) != `{"ok":true}` {
		t.Fatalf("original output mutated: %s", original.Outcomes[0].Output)
	}
	if original.Outcomes[0].Task.Provenance.Source != "subagent" {
		t.Fatalf("original task provenance mutated: %#v", original.Outcomes[0].Task.Provenance)
	}
	if original.Outcomes[0].Provenance.TraceID != "trace-1" {
		t.Fatalf("original outcome provenance mutated: %#v", original.Outcomes[0].Provenance)
	}
}
