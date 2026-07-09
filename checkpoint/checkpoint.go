package checkpoint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tclasen/Exaptra/runtrace"
)

const CurrentVersion = 1

// Envelope is the explicit durable checkpoint format.
type Envelope struct {
	Version  int               `json:"version"`
	RunID    string            `json:"run_id"`
	Snapshot runtrace.Snapshot `json:"snapshot"`
}

// Store persists and loads durable run checkpoints.
type Store interface {
	Save(runID string, snapshot runtrace.Snapshot) (Envelope, error)
	Load(runID string) (Envelope, error)
}

// FileStore writes checkpoint envelopes atomically under a root directory.
type FileStore struct {
	root string
}

// NewFileStore constructs a checkpoint store rooted at root.
func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

// Save persists a versioned checkpoint after re-redacting the snapshot config.
func (s *FileStore) Save(runID string, snapshot runtrace.Snapshot) (Envelope, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return Envelope{}, errors.New("checkpoint: store root is required")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Envelope{}, errors.New("checkpoint: run id is required")
	}
	envelope := Envelope{
		Version:  CurrentVersion,
		RunID:    runID,
		Snapshot: snapshot.Redacted(),
	}
	if err := validateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}

	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: encode %q: %w", runID, err)
	}
	encoded = append(encoded, '\n')

	path := s.path(runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: create directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return Envelope{}, fmt.Errorf("checkpoint: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return Envelope{}, fmt.Errorf("checkpoint: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: replace checkpoint: %w", err)
	}
	cleanup = false
	return envelope, nil
}

// Load reads and validates the latest checkpoint for runID.
func (s *FileStore) Load(runID string) (Envelope, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return Envelope{}, errors.New("checkpoint: store root is required")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Envelope{}, errors.New("checkpoint: run id is required")
	}
	raw, err := os.ReadFile(s.path(runID))
	if err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: read %q: %w", runID, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope Envelope
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, fmt.Errorf("checkpoint: decode %q: %w", runID, err)
	}
	if err := validateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	if envelope.RunID != runID {
		return Envelope{}, fmt.Errorf("checkpoint: run id mismatch: file has %q, requested %q", envelope.RunID, runID)
	}
	return envelope, nil
}

func (s *FileStore) path(runID string) string {
	return filepath.Join(s.root, sanitize(runID)+".json")
}

func validateEnvelope(envelope Envelope) error {
	if envelope.Version != CurrentVersion {
		return fmt.Errorf("checkpoint: unsupported version %d", envelope.Version)
	}
	if strings.TrimSpace(envelope.RunID) == "" {
		return errors.New("checkpoint: run id is required")
	}
	return nil
}

func sanitize(input string) string {
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
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}
