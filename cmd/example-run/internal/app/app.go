package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/examples/localrun"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/runtrace"
	"github.com/tclasen/Exaptra/stream"
)

// Run executes the runnable example and writes the serialized run snapshot.
func Run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("example-run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "examples/localrun/config.example.json", "path to example config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		return err
	}

	s := stream.New()
	provenance := &stream.Provenance{Source: "assistant", Provider: cfg.Model.Provider, Model: cfg.Model.Name}
	if err := s.Append(stream.UserMessage("msg_1", 1, "find the example record", provenance)); err != nil {
		return err
	}

	catalog := mcp.NewCatalog()
	catalog.Permissions().GrantMutations("example run")
	provider := localrun.Provider{}
	identity := mcp.Identity{Name: cfg.MCP[0].Name, Index: 0}
	if _, err := catalog.DiscoverFrom(context.Background(), identity, provider); err != nil {
		return err
	}
	if err := catalog.Expose(identity, "lookup"); err != nil {
		return err
	}

	dispatcher := mcp.NewDispatcher(catalog, resolverMap{
		identity.String(): provider,
	})

	call, err := stream.FunctionCall("fc_1", 2, "lookup", "call_1", json.RawMessage(`{"query":"example"}`), provenance)
	if err != nil {
		return err
	}
	if _, err := dispatcher.Invoke(context.Background(), s, call); err != nil {
		return err
	}
	if err := s.Append(stream.AssistantMessage("msg_2", 4, "the example record was found", provenance)); err != nil {
		return err
	}

	compactor, err := meta.NewStreamCompactor(meta.NewValidator("compact"), s, 3, meta.Identity{Name: "agent", Index: 1}, meta.Identity{Name: identity.Name, Index: identity.Index})
	if err != nil {
		return err
	}
	if _, err := compactor.Compact(s); err != nil {
		return err
	}

	snapshot := runtrace.NewSnapshot(cfg, s, catalog, compactor.Audits())
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return err
	}
	return nil
}

type resolverMap map[string]mcp.ToolCaller

func (r resolverMap) ResolveToolCaller(identity mcp.Identity) (mcp.ToolCaller, bool) {
	caller, ok := r[identity.String()]
	return caller, ok
}
