package workspace

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tclasen/Exaptra/tracker"
)

// State captures the lifecycle state of one issue workspace.
type State struct {
	Issue    tracker.IssueRef `json:"issue"`
	Path     string           `json:"path"`
	RunID    string           `json:"run_id,omitempty"`
	Claimed  bool             `json:"claimed"`
	Released bool             `json:"released"`
	Terminal bool             `json:"terminal"`
	Attempts int              `json:"attempts"`
}

// Manager tracks per-issue workspace lifecycles.
type Manager struct {
	mu    sync.Mutex
	root  string
	state map[string]State
}

// NewManager constructs a manager rooted at a deterministic workspace path.
func NewManager(root string) *Manager {
	return &Manager{
		root:  root,
		state: make(map[string]State),
	}
}

// Path returns the deterministic workspace path for an issue.
func (m *Manager) Path(issue tracker.IssueRef) string {
	return filepath.Join(m.root, sanitizePathPart(issue.Owner), sanitizePathPart(issue.Repo), fmt.Sprintf("%d", issue.Number))
}

// Claim ensures the workspace exists and marks it active for a run.
func (m *Manager) Claim(issue tracker.IssueRef, runID string) (State, error) {
	if err := issue.Validate(); err != nil {
		return State{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := issue.String()
	current := m.state[key]
	if current.Path == "" {
		current = State{
			Issue: issue,
			Path:  m.Path(issue),
		}
	}
	current.RunID = runID
	current.Claimed = true
	current.Released = false
	current.Terminal = false
	current.Attempts++
	m.state[key] = current
	return cloneState(current), nil
}

// Reconcile updates the workspace state after a tracker observation.
func (m *Manager) Reconcile(issue tracker.IssueRef, runID string, terminal bool) (State, error) {
	if err := issue.Validate(); err != nil {
		return State{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.state[issue.String()]
	if !ok {
		return State{}, fmt.Errorf("workspace: issue %s has not been claimed", issue.String())
	}
	current.RunID = runID
	current.Terminal = terminal
	m.state[issue.String()] = current
	return cloneState(current), nil
}

// Release marks the workspace inactive and optionally removes terminal workspaces.
func (m *Manager) Release(issue tracker.IssueRef, terminal bool) (State, error) {
	if err := issue.Validate(); err != nil {
		return State{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.state[issue.String()]
	if !ok {
		return State{}, fmt.Errorf("workspace: issue %s has not been claimed", issue.String())
	}
	current.Released = true
	current.Terminal = terminal
	if terminal {
		delete(m.state, issue.String())
		return cloneState(current), nil
	}
	m.state[issue.String()] = current
	return cloneState(current), nil
}

// Snapshot returns a deterministic, serializable snapshot of tracked workspaces.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	states := make([]State, 0, len(m.state))
	for _, state := range m.state {
		states = append(states, cloneState(state))
	}
	return Snapshot{Root: m.root, States: states}
}

// Snapshot is the serializable workspace manager state.
type Snapshot struct {
	Root   string  `json:"root"`
	States []State `json:"states"`
}

func cloneState(state State) State {
	state.Issue = tracker.IssueRef{
		Owner:  state.Issue.Owner,
		Repo:   state.Issue.Repo,
		Number: state.Issue.Number,
	}
	return state
}

func sanitizePathPart(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// MarshalJSON keeps the snapshot shape explicit.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type alias Snapshot
	return json.Marshal(alias(s))
}
