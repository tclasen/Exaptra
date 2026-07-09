package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/execution"
)

// ErrorCategory identifies provider lifecycle failures.
type ErrorCategory string

const (
	ErrorCategoryProvider    ErrorCategory = "provider"
	ErrorCategoryLifecycle   ErrorCategory = "lifecycle"
	ErrorCategoryConnection  ErrorCategory = "connection"
	ErrorCategoryEnvironment ErrorCategory = "environment"
	ErrorCategoryTool        ErrorCategory = "tool"
	ErrorCategoryPermission  ErrorCategory = "permission"
)

// Error is the structured error returned by the MCP lifecycle layer.
type Error struct {
	Category ErrorCategory `json:"category"`
	Identity string        `json:"identity,omitempty"`
	Op       string        `json:"op,omitempty"`
	Message  string        `json:"message"`
	Err      error         `json:"-"`
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return "<nil>"
	case e.Identity != "" && e.Op != "" && e.Err != nil:
		return fmt.Sprintf("%s %s %s: %s: %v", e.Category, e.Identity, e.Op, e.Message, e.Err)
	case e.Identity != "" && e.Op != "":
		return fmt.Sprintf("%s %s %s: %s", e.Category, e.Identity, e.Op, e.Message)
	case e.Op != "" && e.Err != nil:
		return fmt.Sprintf("%s %s: %s: %v", e.Category, e.Op, e.Message, e.Err)
	case e.Op != "":
		return fmt.Sprintf("%s %s: %s", e.Category, e.Op, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Err)
	default:
		return fmt.Sprintf("%s: %s", e.Category, e.Message)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// AsError extracts a structured MCP error when possible.
func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

func newError(category ErrorCategory, identity, op, message string, err error) error {
	return &Error{
		Category: category,
		Identity: identity,
		Op:       op,
		Message:  message,
		Err:      err,
	}
}

// Identity uniquely identifies a configured provider in runtime state.
type Identity struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
}

func (i Identity) String() string {
	return fmt.Sprintf("%s[%d]", i.Name, i.Index)
}

// Connection represents one started provider process.
type Connection struct {
	identity Identity
	config   config.MCPProvider
	cmd      *exec.Cmd
}

func (c *Connection) Identity() Identity {
	if c == nil {
		return Identity{}
	}
	return c.identity
}

func (c *Connection) Config() config.MCPProvider {
	if c == nil {
		return config.MCPProvider{}
	}
	return c.config
}

func (c *Connection) Close() error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return newError(ErrorCategoryLifecycle, c.identity.String(), "close", "kill provider process", err)
	}
	_ = c.cmd.Wait()
	return nil
}

// Registry tracks active provider connections.
type Registry struct {
	mu          sync.Mutex
	connections map[string]*Connection
}

func NewRegistry() *Registry {
	return &Registry{
		connections: make(map[string]*Connection),
	}
}

// Open starts a configured provider process and stores it under a stable
// provider identity.
func (r *Registry) Open(ctx context.Context, cfg config.MCPProvider, index int) (*Connection, error) {
	if cfg.Name == "" {
		return nil, newError(ErrorCategoryProvider, "", "open", "provider name is required", nil)
	}
	if cfg.Command == "" {
		return nil, newError(ErrorCategoryProvider, cfg.Name, "open", "provider command is required", nil)
	}

	kind, err := execution.ParseKind(cfg.Execution.Kind)
	if err != nil {
		return nil, newError(ErrorCategoryEnvironment, Identity{Name: cfg.Name, Index: index}.String(), "resolve", "resolve provider backend", err)
	}
	cfg.Execution.Kind = kind.String()

	cmd, err := execution.DefaultRegistry().Build(ctx, cfg)
	if err != nil {
		return nil, newError(ErrorCategoryEnvironment, Identity{Name: cfg.Name, Index: index}.String(), "build", "build provider command", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, newError(ErrorCategoryConnection, Identity{Name: cfg.Name, Index: index}.String(), "start", "start provider process", err)
	}

	conn := &Connection{
		identity: Identity{Name: cfg.Name, Index: index},
		config:   cfg,
		cmd:      cmd,
	}

	r.mu.Lock()
	r.connections[conn.identity.String()] = conn
	r.mu.Unlock()

	return conn, nil
}

// CloseAll shuts down all tracked provider connections.
func (r *Registry) CloseAll() error {
	r.mu.Lock()
	connections := make([]*Connection, 0, len(r.connections))
	for _, conn := range r.connections {
		connections = append(connections, conn)
	}
	r.connections = make(map[string]*Connection)
	r.mu.Unlock()

	var firstErr error
	for _, conn := range connections {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Connection returns a tracked provider connection by identity string.
func (r *Registry) Connection(identity Identity) (*Connection, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conn, ok := r.connections[identity.String()]
	return conn, ok
}
