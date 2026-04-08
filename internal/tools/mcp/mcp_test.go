package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/claude-code-go/internal/tools"
	"github.com/anthropics/claude-code-go/internal/tools/mcp"
)

// ── MCPProxyTool ──────────────────────────────────────────────────────────────

func makeProxy() *mcp.MCPProxyTool {
	schema := tools.NewInputSchema(
		map[string]json.RawMessage{
			"input": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "test input",
			}),
		},
		[]string{"input"},
	)
	return mcp.NewMCPProxyTool("mcp__myserver__doThing", "myserver", "Does a thing", schema)
}

func TestMCPProxyTool_Name(t *testing.T) {
	p := makeProxy()
	if p.Name() != "mcp__myserver__doThing" {
		t.Errorf("unexpected name: %q", p.Name())
	}
}

func TestMCPProxyTool_Description(t *testing.T) {
	p := makeProxy()
	if p.Description(nil, nil) != "Does a thing" {
		t.Errorf("unexpected description: %q", p.Description(nil, nil))
	}
}

func TestMCPProxyTool_InputSchema(t *testing.T) {
	p := makeProxy()
	schema := p.InputSchema()
	if _, ok := schema.Properties["input"]; !ok {
		t.Error("schema missing 'input'")
	}
}

func TestMCPProxyTool_IsConcurrencySafe_True(t *testing.T) {
	p := makeProxy()
	if !p.IsConcurrencySafe(nil) {
		t.Error("MCPProxyTool should be concurrency-safe")
	}
}

func TestMCPProxyTool_IsReadOnly_False(t *testing.T) {
	p := makeProxy()
	if p.IsReadOnly(nil) {
		t.Error("MCPProxyTool should not be read-only (unknown side effects)")
	}
}

func TestMCPProxyTool_UserFacingName(t *testing.T) {
	p := makeProxy()
	name := p.UserFacingName(nil)
	if name != "mcp__myserver" {
		t.Errorf("unexpected UserFacingName: %q", name)
	}
}

func TestMCPProxyTool_MCPInfo(t *testing.T) {
	p := makeProxy()
	info := p.MCPInfo()
	if info.ServerName != "myserver" {
		t.Errorf("unexpected ServerName: %q", info.ServerName)
	}
}

func TestMCPProxyTool_Call_ReturnsError(t *testing.T) {
	p := makeProxy()
	in, _ := json.Marshal(map[string]string{"input": "hello"})
	result, err := p.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub MCP tool")
	}
}

func TestMCPProxyTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = makeProxy()
}

// ── ListMcpResourcesTool ──────────────────────────────────────────────────────

func TestListMcpResourcesTool_Name(t *testing.T) {
	if mcp.ListMcpResourcesTool.Name() != "ListMcpResources" {
		t.Errorf("expected ListMcpResources, got %q", mcp.ListMcpResourcesTool.Name())
	}
}

func TestListMcpResourcesTool_IsConcurrencySafe_True(t *testing.T) {
	if !mcp.ListMcpResourcesTool.IsConcurrencySafe(nil) {
		t.Error("ListMcpResources should be concurrency-safe")
	}
}

func TestListMcpResourcesTool_IsReadOnly_True(t *testing.T) {
	if !mcp.ListMcpResourcesTool.IsReadOnly(nil) {
		t.Error("ListMcpResources should be read-only")
	}
}

func TestListMcpResourcesTool_InputSchema(t *testing.T) {
	schema := mcp.ListMcpResourcesTool.InputSchema()
	if _, ok := schema.Properties["server_name"]; !ok {
		t.Error("schema missing 'server_name'")
	}
	if len(schema.Required) != 0 {
		t.Errorf("expected no required fields, got %v", schema.Required)
	}
}

func TestListMcpResourcesTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(mcp.ListMcpResourcesInput{})
	result, err := mcp.ListMcpResourcesTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── ReadMcpResourceTool ───────────────────────────────────────────────────────

func TestReadMcpResourceTool_Name(t *testing.T) {
	if mcp.ReadMcpResourceTool.Name() != "ReadMcpResource" {
		t.Errorf("expected ReadMcpResource, got %q", mcp.ReadMcpResourceTool.Name())
	}
}

func TestReadMcpResourceTool_IsConcurrencySafe_True(t *testing.T) {
	if !mcp.ReadMcpResourceTool.IsConcurrencySafe(nil) {
		t.Error("ReadMcpResource should be concurrency-safe")
	}
}

func TestReadMcpResourceTool_IsReadOnly_True(t *testing.T) {
	if !mcp.ReadMcpResourceTool.IsReadOnly(nil) {
		t.Error("ReadMcpResource should be read-only")
	}
}

func TestReadMcpResourceTool_InputSchema(t *testing.T) {
	schema := mcp.ReadMcpResourceTool.InputSchema()
	if _, ok := schema.Properties["server_name"]; !ok {
		t.Error("schema missing 'server_name'")
	}
	if _, ok := schema.Properties["uri"]; !ok {
		t.Error("schema missing 'uri'")
	}
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	if !reqMap["server_name"] || !reqMap["uri"] {
		t.Errorf("expected server_name and uri in Required, got %v", schema.Required)
	}
}

func TestReadMcpResourceTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "s1", URI: "file://test"})
	name := mcp.ReadMcpResourceTool.UserFacingName(in)
	if name != "ReadMcpResource(file://test)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestReadMcpResourceTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "s1", URI: "uri://x"})
	result, err := mcp.ReadMcpResourceTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

func TestAllMCPTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = mcp.ListMcpResourcesTool
	var _ tools.Tool = mcp.ReadMcpResourceTool
}
