package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tclasen/Exaptra/config"
)

func TestRegistryOpensAndClosesProviderConnection(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := registry.Open(ctx, config.MCPProvider{
		Name:    "sleepy",
		Command: "sleep",
		Args:    []string{"60"},
	}, 0)
	if err != nil {
		t.Fatalf("open provider: %v", err)
	}

	if got := conn.Identity().String(); got != "sleepy[0]" {
		t.Fatalf("identity = %q, want sleepy[0]", got)
	}
	if _, ok := registry.Connection(Identity{Name: "sleepy", Index: 0}); !ok {
		t.Fatal("connection not tracked in registry")
	}

	if err := registry.CloseAll(); err != nil {
		t.Fatalf("close all: %v", err)
	}
}

func TestRegistryKeepsProviderDefinitionsDistinct(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer registry.CloseAll()

	first, err := registry.Open(ctx, config.MCPProvider{Name: "provider", Command: "sleep", Args: []string{"60"}}, 0)
	if err != nil {
		t.Fatalf("open first provider: %v", err)
	}
	second, err := registry.Open(ctx, config.MCPProvider{Name: "provider", Command: "sleep", Args: []string{"60"}}, 1)
	if err != nil {
		t.Fatalf("open second provider: %v", err)
	}

	if first.Identity() == second.Identity() {
		t.Fatalf("identities are not distinct: %+v", first.Identity())
	}
	if got := first.Identity().String(); got != "provider[0]" {
		t.Fatalf("first identity = %q, want provider[0]", got)
	}
	if got := second.Identity().String(); got != "provider[1]" {
		t.Fatalf("second identity = %q, want provider[1]", got)
	}
}

func TestRegistryReturnsStructuredErrorForBadCommand(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := registry.Open(ctx, config.MCPProvider{
		Name:    "broken",
		Command: "definitely-not-a-real-command",
	}, 0)
	if err == nil {
		t.Fatal("open succeeded with missing command")
	}

	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured error, got %T", err)
	}
	if structured.Category != ErrorCategoryConnection {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryConnection)
	}
	if structured.Identity != "broken[0]" {
		t.Fatalf("identity = %q, want broken[0]", structured.Identity)
	}
	if !errors.As(err, &structured) {
		t.Fatal("expected structured error to unwrap cleanly")
	}
}
