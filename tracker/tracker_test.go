package tracker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubAdapter struct {
	commentErr error
	stateErr   error
	linkErr    error
}

func (s stubAdapter) AddComment(context.Context, CommentRequest) error {
	return s.commentErr
}

func (s stubAdapter) SetIssueState(context.Context, StateRequest) error {
	return s.stateErr
}

func (s stubAdapter) LinkPullRequest(context.Context, PullRequestLinkRequest) error {
	return s.linkErr
}

func TestStoreRecordsProgressAndHandoffWrites(t *testing.T) {
	store := NewStore(nil)
	issue := IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 52}

	commentAudit, err := store.Comment(context.Background(), CommentRequest{
		RunID: "run-1",
		Issue: issue,
		Body:  "progress update",
		Provenance: Provenance{
			RunID:     "run-1",
			Source:    "orchestrator",
			Component: "tracker",
		},
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	if commentAudit.Result != ResultApplied {
		t.Fatalf("comment result = %q, want %q", commentAudit.Result, ResultApplied)
	}

	stateAudit, err := store.SetState(context.Background(), StateRequest{
		RunID:  "run-1",
		Issue:  issue,
		State:  HandoffStateReview,
		Reason: "ready for human review",
		Provenance: Provenance{
			RunID:     "run-1",
			Source:    "orchestrator",
			Component: "tracker",
		},
	})
	if err != nil {
		t.Fatalf("set state: %v", err)
	}

	linkAudit, err := store.LinkPullRequest(context.Background(), PullRequestLinkRequest{
		RunID: "run-1",
		Issue: issue,
		PullRequest: PullRequestRef{
			Owner:  "tclasen",
			Repo:   "Exaptra",
			Number: 99,
			URL:    "https://github.com/tclasen/Exaptra/pull/99",
		},
		State: HandoffStateReview,
		Provenance: Provenance{
			RunID:     "run-1",
			Source:    "orchestrator",
			Component: "tracker",
		},
	})
	if err != nil {
		t.Fatalf("link pull request: %v", err)
	}

	state, ok := store.State(issue)
	if !ok {
		t.Fatal("tracked issue state missing")
	}
	if state.State != HandoffStateReview {
		t.Fatalf("issue state = %q, want %q", state.State, HandoffStateReview)
	}
	if len(state.Comments) != 1 || state.Comments[0].Body != "progress update" {
		t.Fatalf("comments = %#v, want one progress update", state.Comments)
	}
	if state.PullRequest == nil || state.PullRequest.Number != 99 {
		t.Fatalf("pull request = %#v, want linked PR #99", state.PullRequest)
	}
	if state.LastRunID != "run-1" {
		t.Fatalf("last run id = %q, want run-1", state.LastRunID)
	}

	audits := store.Audits()
	if len(audits) != 3 {
		t.Fatalf("audit len = %d, want 3", len(audits))
	}
	if audits[0].Sequence != 1 || audits[1].Sequence != 2 || audits[2].Sequence != 3 {
		t.Fatalf("audit sequences = %d,%d,%d", audits[0].Sequence, audits[1].Sequence, audits[2].Sequence)
	}
	for _, audit := range audits {
		if audit.RunID != "run-1" {
			t.Fatalf("audit run id = %q, want run-1", audit.RunID)
		}
		if audit.Issue != issue {
			t.Fatalf("audit issue = %#v, want %#v", audit.Issue, issue)
		}
		if !strings.Contains(string(audit.Request), `"run_id":"run-1"`) {
			t.Fatalf("audit request missing run id: %s", audit.Request)
		}
	}
	if len(stateAudit.Before) == 0 || len(stateAudit.After) == 0 {
		t.Fatal("state audit missing before/after snapshot")
	}
	if len(linkAudit.After) == 0 {
		t.Fatal("link audit missing after snapshot")
	}
}

func TestStoreRecordsAdapterFailuresWithoutMutatingState(t *testing.T) {
	store := NewStore(stubAdapter{commentErr: errors.New("boom")})
	issue := IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 52}

	audit, err := store.Comment(context.Background(), CommentRequest{
		RunID: "run-2",
		Issue: issue,
		Body:  "attempted update",
		Provenance: Provenance{
			RunID:     "run-2",
			Source:    "orchestrator",
			Component: "tracker",
		},
	})
	if err == nil {
		t.Fatal("comment unexpectedly succeeded")
	}
	if audit.Result != ResultFailed {
		t.Fatalf("audit result = %q, want %q", audit.Result, ResultFailed)
	}
	if audit.Error != "boom" {
		t.Fatalf("audit error = %q, want boom", audit.Error)
	}
	if _, ok := store.State(issue); ok {
		t.Fatal("failed comment mutated issue state")
	}
}
