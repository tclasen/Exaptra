package execution

import (
	"context"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/config"
)

func TestBuildLocalCommand(t *testing.T) {
	cmd, err := DefaultRegistry().Build(context.Background(), config.MCPProvider{
		Name:    "localrun",
		Command: "builtin",
		Args:    []string{"--serve"},
		Execution: config.ExecutionEnvironment{
			Kind: KindLocal.String(),
		},
	})
	if err != nil {
		t.Fatalf("build local command: %v", err)
	}
	if got := cmd.Path; got != "builtin" {
		t.Fatalf("command path = %q, want builtin", got)
	}
}

func TestBuildDockerCommand(t *testing.T) {
	cmd, err := DefaultRegistry().Build(context.Background(), config.MCPProvider{
		Name:    "filesystem",
		Command: "server",
		Args:    []string{"--read"},
		Execution: config.ExecutionEnvironment{
			Kind:   KindDocker.String(),
			Target: "provider-image:latest",
			Args:   []string{"--network", "host"},
		},
	})
	if err != nil {
		t.Fatalf("build docker command: %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "docker") || !strings.Contains(got, "provider-image:latest") || !strings.Contains(got, "server --read") {
		t.Fatalf("docker command args = %q", got)
	}
}

func TestBuildSSHCommand(t *testing.T) {
	cmd, err := DefaultRegistry().Build(context.Background(), config.MCPProvider{
		Name:    "filesystem",
		Command: "server",
		Execution: config.ExecutionEnvironment{
			Kind:   KindSSH.String(),
			Target: "build-host",
			Args:   []string{"-p", "2222"},
		},
	})
	if err != nil {
		t.Fatalf("build ssh command: %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "ssh") || !strings.Contains(got, "build-host") || !strings.Contains(got, "server") {
		t.Fatalf("ssh command args = %q", got)
	}
}

func TestBuildWASMCommand(t *testing.T) {
	cmd, err := DefaultRegistry().Build(context.Background(), config.MCPProvider{
		Name:    "filesystem",
		Command: "server.wasm",
		Args:    []string{"--mode", "serve"},
		Execution: config.ExecutionEnvironment{
			Kind: KindWASM.String(),
		},
	})
	if err != nil {
		t.Fatalf("build wasm command: %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "wasmtime") || !strings.Contains(got, "server.wasm") || !strings.Contains(got, "--mode serve") {
		t.Fatalf("wasm command args = %q", got)
	}
}
