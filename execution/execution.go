package execution

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tclasen/Exaptra/config"
)

// Kind identifies the command execution backend.
type Kind string

const (
	KindLocal  Kind = "local"
	KindDocker Kind = "docker"
	KindSSH    Kind = "ssh"
	KindWASM   Kind = "wasm"
)

// Builder constructs a runnable command for one execution backend.
type Builder interface {
	Build(ctx context.Context, provider config.MCPProvider, spec config.ExecutionEnvironment) (*exec.Cmd, error)
}

// Registry resolves builders by backend kind.
type Registry struct {
	builders map[Kind]Builder
}

// NewRegistry constructs a builder registry.
func NewRegistry(builders map[Kind]Builder) *Registry {
	clone := make(map[Kind]Builder, len(builders))
	for kind, builder := range builders {
		clone[kind] = builder
	}
	return &Registry{builders: clone}
}

// DefaultRegistry returns the built-in execution backends.
func DefaultRegistry() *Registry {
	return NewRegistry(map[Kind]Builder{
		KindLocal:  localBuilder{},
		KindDocker: dockerBuilder{},
		KindSSH:    sshBuilder{},
		KindWASM:   wasmBuilder{},
	})
}

// Build constructs the command for the selected backend.
func (r *Registry) Build(ctx context.Context, provider config.MCPProvider) (*exec.Cmd, error) {
	spec := provider.Execution
	kind, err := ParseKind(spec.Kind)
	if err != nil {
		return nil, err
	}
	builder, ok := r.builders[kind]
	if !ok {
		return nil, fmt.Errorf("execution: unsupported backend %q", spec.Kind)
	}
	return builder.Build(ctx, provider, spec)
}

type localBuilder struct{}

func (localBuilder) Build(ctx context.Context, provider config.MCPProvider, spec config.ExecutionEnvironment) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, provider.Command, provider.Args...)
	cmd.Env = append(os.Environ(), flattenEnv(provider.Env)...)
	return cmd, nil
}

type dockerBuilder struct{}

func (dockerBuilder) Build(ctx context.Context, provider config.MCPProvider, spec config.ExecutionEnvironment) (*exec.Cmd, error) {
	if spec.Target == "" {
		return nil, fmt.Errorf("execution: docker backend requires an image target")
	}
	args := []string{"run", "--rm", "-i"}
	args = append(args, spec.Args...)
	for key, value := range provider.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, spec.Target, provider.Command)
	args = append(args, provider.Args...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()
	return cmd, nil
}

type sshBuilder struct{}

func (sshBuilder) Build(ctx context.Context, provider config.MCPProvider, spec config.ExecutionEnvironment) (*exec.Cmd, error) {
	if spec.Target == "" {
		return nil, fmt.Errorf("execution: ssh backend requires a host target")
	}
	remote := []string{"env"}
	for key, value := range provider.Env {
		remote = append(remote, fmt.Sprintf("%s=%s", key, value))
	}
	remote = append(remote, provider.Command)
	remote = append(remote, provider.Args...)
	args := []string{}
	args = append(args, spec.Args...)
	args = append(args, spec.Target, "--")
	args = append(args, remote...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = os.Environ()
	return cmd, nil
}

type wasmBuilder struct{}

func (wasmBuilder) Build(ctx context.Context, provider config.MCPProvider, spec config.ExecutionEnvironment) (*exec.Cmd, error) {
	// The module path is provided through provider.Command, while the target can
	// carry runtime-specific arguments such as a preloaded component path.
	args := []string{"run"}
	args = append(args, spec.Args...)
	args = append(args, provider.Command)
	args = append(args, provider.Args...)
	if spec.Target != "" {
		args = append(args, "--", spec.Target)
	}
	cmd := exec.CommandContext(ctx, "wasmtime", args...)
	cmd.Env = append(os.Environ(), flattenEnv(provider.Env)...)
	return cmd, nil
}

func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

// KindString returns a stable string for logs and metadata.
func (k Kind) String() string {
	if k == "" {
		return string(KindLocal)
	}
	return string(k)
}

// ParseKind normalizes an execution backend kind.
func ParseKind(kind string) (Kind, error) {
	switch Kind(strings.ToLower(strings.TrimSpace(kind))) {
	case "", KindLocal:
		return KindLocal, nil
	case KindDocker:
		return KindDocker, nil
	case KindSSH:
		return KindSSH, nil
	case KindWASM:
		return KindWASM, nil
	default:
		return "", fmt.Errorf("execution: unsupported backend %q", kind)
	}
}
