package workspace

import (
	"testing"

	"github.com/tclasen/Exaptra/tracker"
)

func TestPathIsDeterministicAndScopedByIssue(t *testing.T) {
	manager := NewManager("/tmp/workspaces")
	first := manager.Path(tracker.IssueRef{Owner: "Tclasen", Repo: "Exaptra", Number: 47})
	second := manager.Path(tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 47})
	other := manager.Path(tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 48})

	if first != second {
		t.Fatalf("path is not deterministic: %q vs %q", first, second)
	}
	if first == other {
		t.Fatalf("different issues mapped to same path: %q", first)
	}
}

func TestClaimReconcileAndReleasePreserveStateAcrossRetries(t *testing.T) {
	manager := NewManager("/tmp/workspaces")
	issue := tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 47}

	claimed, err := manager.Claim(issue, "run-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed.Claimed || claimed.Attempts != 1 {
		t.Fatalf("claimed state = %#v", claimed)
	}

	reclaimed, err := manager.Claim(issue, "run-2")
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if reclaimed.Path != claimed.Path || reclaimed.Attempts != 2 {
		t.Fatalf("reclaimed state = %#v, claimed = %#v", reclaimed, claimed)
	}

	reconciled, err := manager.Reconcile(issue, "run-2", false)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled.Terminal {
		t.Fatalf("reconcile unexpectedly marked terminal: %#v", reconciled)
	}

	released, err := manager.Release(issue, true)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released.Released || !released.Terminal {
		t.Fatalf("released state = %#v", released)
	}
	if snapshot := manager.Snapshot(); len(snapshot.States) != 0 {
		t.Fatalf("terminal release should remove state from snapshot: %#v", snapshot)
	}
}
