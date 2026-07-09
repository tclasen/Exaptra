package checkpoint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/runtrace"
	"github.com/tclasen/Exaptra/stream"
)

func TestFileStoreSavesAndLoadsVersionedCheckpoint(t *testing.T) {
	store := NewFileStore(t.TempDir())
	snapshot := testSnapshot()

	saved, err := store.Save("run-1", snapshot)
	if err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	if saved.Version != CurrentVersion || saved.RunID != "run-1" {
		t.Fatalf("saved envelope = %#v", saved)
	}

	loaded, err := store.Load("run-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if loaded.Version != CurrentVersion || loaded.RunID != "run-1" {
		t.Fatalf("loaded envelope = %#v", loaded)
	}
	if got := loaded.Snapshot.Stream.Items[0].Text(); got != "hello" {
		t.Fatalf("loaded stream text = %q", got)
	}
}

func TestFileStoreRedactsSecretsBeforePersisting(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	snapshot := testSnapshot()

	if _, err := store.Save("run-secret", snapshot); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "run-secret.json"))
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if strings.Contains(string(raw), "model-secret") || strings.Contains(string(raw), "provider-secret") {
		t.Fatalf("checkpoint leaked secret: %s", raw)
	}
	if !strings.Contains(string(raw), `"api_key": ""`) || !strings.Contains(string(raw), `"EXAPTRA_TOKEN": "[redacted]"`) {
		t.Fatalf("checkpoint missing redacted markers: %s", raw)
	}
}

func TestFileStoreReplacesCheckpointAtomically(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	first := testSnapshot()
	second := testSnapshot()
	second.Stream.Items[0] = stream.AssistantMessage("msg-2", 2, "replacement", nil)

	if _, err := store.Save("run-1", first); err != nil {
		t.Fatalf("save first checkpoint: %v", err)
	}
	if _, err := store.Save("run-1", second); err != nil {
		t.Fatalf("save second checkpoint: %v", err)
	}
	loaded, err := store.Load("run-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if got := loaded.Snapshot.Stream.Items[0].Text(); got != "replacement" {
		t.Fatalf("loaded text = %q, want replacement", got)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %#v", matches)
	}
}

func TestFileStoreReportsMissingAndCorruptCheckpoints(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	if _, err := store.Load("missing"); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing checkpoint error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte(`{"version":1,`), 0o600); err != nil {
		t.Fatalf("write corrupt checkpoint: %v", err)
	}
	if _, err := store.Load("corrupt"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("corrupt checkpoint error = %v", err)
	}
}

func TestFileStoreRejectsUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	raw, err := json.Marshal(Envelope{Version: CurrentVersion + 1, RunID: "run-1"})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run-1.json"), raw, 0o600); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	_, err = NewFileStore(dir).Load("run-1")
	if err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("unsupported version error = %v", err)
	}
}

func testSnapshot() runtrace.Snapshot {
	s := stream.New()
	_ = s.Append(stream.UserMessage("msg-1", 1, "hello", nil))
	return runtrace.Snapshot{
		Config: config.Config{
			Model: config.ModelConfig{
				Provider: "openai",
				Name:     "gpt-4.1",
				APIKey:   "model-secret",
			},
			MCP: []config.MCPProvider{{
				Name:    "filesystem",
				Command: "npx",
				Env:     map[string]string{"EXAPTRA_TOKEN": "provider-secret"},
			}},
		},
		Stream: s.Trajectory(),
	}
}
