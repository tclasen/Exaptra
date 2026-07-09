package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

const (
	RecordTypeComment = "exaptra:tracker_comment"
	RecordTypeState   = "exaptra:tracker_state_transition"
	RecordTypePRLink  = "exaptra:tracker_pr_link"
	ResultApplied     = "applied"
	ResultRejected    = "rejected"
	ResultFailed      = "failed"
)

// IssueRef identifies a tracked issue.
type IssueRef struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

func (r IssueRef) Validate() error {
	if r.Owner == "" || r.Repo == "" {
		return errors.New("tracker: issue owner and repo are required")
	}
	if r.Number <= 0 {
		return errors.New("tracker: issue number must be positive")
	}
	return nil
}

func (r IssueRef) String() string {
	return fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
}

// PullRequestRef identifies a linked pull request.
type PullRequestRef struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	URL    string `json:"url,omitempty"`
}

func (r PullRequestRef) Validate() error {
	if r.Owner == "" || r.Repo == "" {
		return errors.New("tracker: pull request owner and repo are required")
	}
	if r.Number <= 0 {
		return errors.New("tracker: pull request number must be positive")
	}
	return nil
}

func (r PullRequestRef) String() string {
	if r.URL != "" {
		return r.URL
	}
	return fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
}

// HandoffState describes a workflow-defined issue lifecycle state.
type HandoffState string

const (
	HandoffStateOpen   HandoffState = "open"
	HandoffStateActive HandoffState = "in_progress"
	HandoffStateReview HandoffState = "review_ready"
	HandoffStateClosed HandoffState = "done"
)

func (s HandoffState) Validate() error {
	if s == "" {
		return errors.New("tracker: handoff state is required")
	}
	return nil
}

// Provenance captures where a tracker mutation came from.
type Provenance struct {
	RunID     string `json:"run_id,omitempty"`
	Source    string `json:"source,omitempty"`
	Component string `json:"component,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

// CommentRequest describes a tracker comment write.
type CommentRequest struct {
	RunID      string     `json:"run_id"`
	Issue      IssueRef   `json:"issue"`
	Body       string     `json:"body"`
	Provenance Provenance `json:"provenance"`
}

// StateRequest describes a workflow state mutation.
type StateRequest struct {
	RunID      string       `json:"run_id"`
	Issue      IssueRef     `json:"issue"`
	State      HandoffState `json:"state"`
	Reason     string       `json:"reason,omitempty"`
	Provenance Provenance   `json:"provenance"`
}

// PullRequestLinkRequest describes a PR handoff update.
type PullRequestLinkRequest struct {
	RunID       string         `json:"run_id"`
	Issue       IssueRef       `json:"issue"`
	PullRequest PullRequestRef `json:"pull_request"`
	State       HandoffState   `json:"state"`
	Provenance  Provenance     `json:"provenance"`
}

// Comment captures an appended tracker comment.
type Comment struct {
	Body       string     `json:"body"`
	RunID      string     `json:"run_id,omitempty"`
	Provenance Provenance `json:"provenance,omitempty"`
}

// IssueState captures the in-memory view of a tracked issue.
type IssueState struct {
	Issue       IssueRef        `json:"issue"`
	State       HandoffState    `json:"state,omitempty"`
	Comments    []Comment       `json:"comments,omitempty"`
	PullRequest *PullRequestRef `json:"pull_request,omitempty"`
	LastRunID   string          `json:"last_run_id,omitempty"`
	UpdatedBy   Provenance      `json:"updated_by,omitempty"`
}

// AuditRecord captures a tracker mutation and its result.
type AuditRecord struct {
	Type       string          `json:"type"`
	Sequence   int64           `json:"sequence"`
	Operation  string          `json:"operation"`
	RunID      string          `json:"run_id"`
	Issue      IssueRef        `json:"issue"`
	Request    json.RawMessage `json:"request"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Provenance Provenance      `json:"provenance"`
	Result     string          `json:"result"`
	Error      string          `json:"error,omitempty"`
}

// Adapter executes tracker writes against an external provider.
type Adapter interface {
	AddComment(context.Context, CommentRequest) error
	SetIssueState(context.Context, StateRequest) error
	LinkPullRequest(context.Context, PullRequestLinkRequest) error
}

// Store records tracker state changes and audits them.
type Store struct {
	mu      sync.Mutex
	adapter Adapter
	issues  map[string]IssueState
	audits  []AuditRecord
	seq     int64
}

// NewStore creates a tracker store with an optional external adapter.
func NewStore(adapter Adapter) *Store {
	return &Store{
		adapter: adapter,
		issues:  make(map[string]IssueState),
	}
}

// Comment appends a tracker comment and records the audit trail.
func (s *Store) Comment(ctx context.Context, req CommentRequest) (AuditRecord, error) {
	return s.write(ctx, "comment", req.RunID, req.Issue, req.Provenance, req, func(state *IssueState) error {
		if req.Body == "" {
			return errors.New("tracker: comment body is required")
		}
		if s.adapter != nil {
			if err := s.adapter.AddComment(ctx, req); err != nil {
				return err
			}
		}
		state.Comments = append(state.Comments, Comment{
			Body:       req.Body,
			RunID:      req.RunID,
			Provenance: req.Provenance,
		})
		state.LastRunID = req.RunID
		state.UpdatedBy = req.Provenance
		return nil
	})
}

// SetState records a workflow state change for the issue.
func (s *Store) SetState(ctx context.Context, req StateRequest) (AuditRecord, error) {
	return s.write(ctx, "state", req.RunID, req.Issue, req.Provenance, req, func(state *IssueState) error {
		if err := req.State.Validate(); err != nil {
			return err
		}
		if s.adapter != nil {
			if err := s.adapter.SetIssueState(ctx, req); err != nil {
				return err
			}
		}
		state.State = req.State
		state.LastRunID = req.RunID
		state.UpdatedBy = req.Provenance
		return nil
	})
}

// LinkPullRequest records a pull request handoff for the issue.
func (s *Store) LinkPullRequest(ctx context.Context, req PullRequestLinkRequest) (AuditRecord, error) {
	return s.write(ctx, "pr_link", req.RunID, req.Issue, req.Provenance, req, func(state *IssueState) error {
		if err := req.PullRequest.Validate(); err != nil {
			return err
		}
		if err := req.State.Validate(); err != nil {
			return err
		}
		if s.adapter != nil {
			if err := s.adapter.LinkPullRequest(ctx, req); err != nil {
				return err
			}
		}
		state.State = req.State
		pr := req.PullRequest
		state.PullRequest = &pr
		state.LastRunID = req.RunID
		state.UpdatedBy = req.Provenance
		return nil
	})
}

// State returns the tracked state for an issue.
func (s *Store) State(issue IssueRef) (IssueState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.issues[issue.String()]
	return cloneState(state), ok
}

// Audits returns the tracker audit log.
func (s *Store) Audits() []AuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditRecord, len(s.audits))
	copy(out, s.audits)
	return out
}

func (s *Store) write(ctx context.Context, operation, runID string, issue IssueRef, provenance Provenance, request any, mutate func(*IssueState) error) (AuditRecord, error) {
	if err := issue.Validate(); err != nil {
		return s.record(operation, runID, issue, provenance, request, nil, nil, ResultRejected, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := issue.String()
	current, ok := s.issues[key]
	if !ok {
		before := json.RawMessage("null")
		next := IssueState{Issue: issue}
		if err := mutate(&next); err != nil {
			return s.recordLocked(operation, runID, issue, provenance, request, before, before, resultForError(err), err)
		}
		after, err := json.Marshal(next)
		if err != nil {
			return s.recordLocked(operation, runID, issue, provenance, request, before, before, ResultFailed, err)
		}
		s.issues[key] = next
		return s.recordLocked(operation, runID, issue, provenance, request, before, after, ResultApplied, nil)
	}

	before, _ := json.Marshal(cloneState(current))
	if before == nil {
		before = []byte("null")
	}

	next := cloneState(current)
	next.Issue = issue
	if err := mutate(&next); err != nil {
		return s.recordLocked(operation, runID, issue, provenance, request, before, before, resultForError(err), err)
	}

	after, err := json.Marshal(next)
	if err != nil {
		return s.recordLocked(operation, runID, issue, provenance, request, before, before, ResultFailed, err)
	}

	s.issues[key] = next
	return s.recordLocked(operation, runID, issue, provenance, request, before, after, ResultApplied, nil)
}

func (s *Store) record(operation, runID string, issue IssueRef, provenance Provenance, request any, before, after []byte, result string, err error) (AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordLocked(operation, runID, issue, provenance, request, before, after, result, err)
}

func (s *Store) recordLocked(operation, runID string, issue IssueRef, provenance Provenance, request any, before, after []byte, result string, err error) (AuditRecord, error) {
	req, _ := json.Marshal(request)
	s.seq++
	audit := AuditRecord{
		Type:       recordTypeFor(operation),
		Sequence:   s.seq,
		Operation:  operation,
		RunID:      runID,
		Issue:      issue,
		Request:    req,
		Before:     cloneJSON(before),
		After:      cloneJSON(after),
		Provenance: provenance,
		Result:     result,
	}
	if err != nil {
		audit.Error = err.Error()
	}
	s.audits = append(s.audits, audit)
	return audit, err
}

func recordTypeFor(operation string) string {
	switch operation {
	case "comment":
		return RecordTypeComment
	case "state":
		return RecordTypeState
	case "pr_link":
		return RecordTypePRLink
	default:
		return "exaptra:tracker_audit"
	}
}

func resultForError(err error) string {
	if err == nil {
		return ResultApplied
	}
	return ResultFailed
}

func cloneState(state IssueState) IssueState {
	if state.PullRequest != nil {
		pr := *state.PullRequest
		state.PullRequest = &pr
	}
	if len(state.Comments) != 0 {
		state.Comments = append([]Comment(nil), state.Comments...)
	}
	return state
}

func cloneJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
