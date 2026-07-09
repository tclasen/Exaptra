package meta

import (
	"encoding/json"
	"testing"
)

func TestMetaToolRequestAndAuditSerializeDeterministically(t *testing.T) {
	request := Request{
		Type:      MetaToolRequestType,
		Operation: "compact",
		Caller:    Identity{Name: "agent", Index: 1},
		Provider:  Identity{Name: "mcp", Index: 2},
		Target:    "stream.context",
		Input:     json.RawMessage(`{"limit":2}`),
	}
	transition := Transition{
		Type:      MetaTransitionType,
		Operation: "compact",
		Target:    "stream.context",
		Before:    json.RawMessage(`{"items":4}`),
		After:     json.RawMessage(`{"items":2}`),
		Validation: ValidationOutcome{
			Allowed: true,
			Reason:  "authorized",
		},
		Applied: true,
		Result:  "applied",
	}
	audit := AuditRecord{
		Type:        MetaAuditRecordType,
		Request:     request,
		Transition:  transition,
		Validation:  transition.Validation,
		FinalResult: "success",
	}

	reqJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	const wantRequest = `{"type":"exaptra:meta_tool_request","operation":"compact","caller":{"name":"agent","index":1},"provider":{"name":"mcp","index":2},"target":"stream.context","input":{"limit":2}}`
	if string(reqJSON) != wantRequest {
		t.Fatalf("request json mismatch\n got: %s\nwant: %s", reqJSON, wantRequest)
	}

	auditJSON, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("marshal audit: %v", err)
	}
	const wantAudit = `{"type":"exaptra:meta_audit","request":{"type":"exaptra:meta_tool_request","operation":"compact","caller":{"name":"agent","index":1},"provider":{"name":"mcp","index":2},"target":"stream.context","input":{"limit":2}},"transition":{"type":"exaptra:meta_transition","operation":"compact","target":"stream.context","before":{"items":4},"after":{"items":2},"validation":{"allowed":true,"reason":"authorized"},"applied":true,"result":"applied"},"validation":{"allowed":true,"reason":"authorized"},"final_result":"success"}`
	if string(auditJSON) != wantAudit {
		t.Fatalf("audit json mismatch\n got: %s\nwant: %s", auditJSON, wantAudit)
	}

	again, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("marshal audit again: %v", err)
	}
	if string(again) != string(auditJSON) {
		t.Fatalf("audit serialization not deterministic\nfirst:  %s\nsecond: %s", auditJSON, again)
	}
}

func TestMetaFormatsAreDistinctFromOrdinaryToolCalls(t *testing.T) {
	if MetaToolRequestType == "function_call" || MetaTransitionType == "function_call_output" {
		t.Fatal("meta records must be distinct from ordinary stream tool call types")
	}
}
