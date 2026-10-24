package agent

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/f24aalam/agentsync/internal/config"
)

// Case A: CRLF line endings
// Ensure unrelated sections and their existing line endings remain byte-for-byte unchanged where possible.
// Repeated installs must not cause formatting drift.
func TestTOML_CRLFLineEndingsPreserved(t *testing.T) {
	initialTOML := "# Codex configuration\r\nmodel = \"o3-mini\"\r\n\r\n[profiles.development]\r\nmodel = \"gpt-4o\"\r\n\r\n[mcp_servers.postgres]\r\ncommand = \"old-pg\"\r\n"
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	if !strings.Contains(updated, "\r\n") {
		t.Fatalf("expected CRLF line endings preserved, got LF only:\n%q", updated)
	}
	if strings.Contains(strings.ReplaceAll(updated, "\r\n", ""), "\n") {
		t.Fatalf("found mixed line endings in CRLF document:\n%q", updated)
	}

	var decoded map[string]any
	if _, err := toml.Decode(updated, &decoded); err != nil {
		t.Fatalf("updated TOML is invalid: %v", err)
	}

	// Repeated install must be 100% byte-for-byte identical
	repeated, err := updateTomlMCPServers(updated, servers)
	if err != nil {
		t.Fatalf("repeated update failed: %v", err)
	}
	if repeated != updated {
		t.Fatalf("drift detected on repeated run with CRLF:\nFirst:\n%q\nSecond:\n%q", updated, repeated)
	}
}

// Case B: Multiline basic strings containing apparent table headers
func TestTOML_MultilineBasicStringsContainingFakeTableHeaders(t *testing.T) {
	initialTOML := `model = "o3-mini"
description = """
This text contains something that looks like a table:
[mcp_servers.fake]
but it is only text.
"""

[profiles.development]
model = "gpt-4o"
`
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx", Args: []string{"-y", "pg"}},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	if !strings.Contains(updated, "[mcp_servers.fake]") {
		t.Fatalf("multiline basic string was corrupted, missing fake table text:\n%s", updated)
	}

	var decoded map[string]any
	if _, err := toml.Decode(updated, &decoded); err != nil {
		t.Fatalf("generated invalid TOML: %v\nContent:\n%s", err, updated)
	}
	mcpServers, _ := decoded["mcp_servers"].(map[string]any)
	if _, exists := mcpServers["fake"]; exists {
		t.Fatalf("fake table inside multiline string was parsed as an actual table!")
	}
	if _, exists := mcpServers["postgres"]; !exists {
		t.Fatalf("postgres server was not added!")
	}
}

// Case C: Multiline literal strings containing apparent table headers
func TestTOML_MultilineLiteralStringsContainingFakeTableHeaders(t *testing.T) {
	initialTOML := `notes = '''
[mcp_servers.not_real]
This is still string content.
'''

[profiles.development]
model = "gpt-4o"
`
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx"},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	if !strings.Contains(updated, "[mcp_servers.not_real]") {
		t.Fatalf("multiline literal string was corrupted:\n%s", updated)
	}

	var decoded map[string]any
	if _, err := toml.Decode(updated, &decoded); err != nil {
		t.Fatalf("generated invalid TOML: %v", err)
	}
	mcpServers, _ := decoded["mcp_servers"].(map[string]any)
	if _, exists := mcpServers["not_real"]; exists {
		t.Fatalf("not_real table inside literal string was parsed as actual table!")
	}
}

// Case D: Quoted table names
// [mcp_servers."server.with.dots"]
// Ensure quoted names are handled safely or explicitly rejected without modifying the existing file.
func TestTOML_QuotedTableNamesHandledSafely(t *testing.T) {
	initialTOML := `# Configuration with quoted table names
model = "o3-mini"

[mcp_servers."server.with.dots"]
command = "example"
args = ["--flag"]

[mcp_servers.'literal.server']
command = "example2"

[profiles."staging.us-east-1"]
model = "custom"
`
	// Unrelated server addition
	servers := map[string]config.MCPServer{
		"new_tool": {Command: "new-cmd"},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	var decoded map[string]any
	if _, err := toml.Decode(updated, &decoded); err != nil {
		t.Fatalf("generated invalid TOML: %v\nContent:\n%s", err, updated)
	}

	mcpServers, ok := decoded["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("missing mcp_servers table in decoded TOML")
	}

	// Quoted unmanaged server must remain intact
	if _, exists := mcpServers["server.with.dots"]; !exists {
		t.Fatalf("quoted server 'server.with.dots' was lost or corrupted!\n%s", updated)
	}
	if _, exists := mcpServers["literal.server"]; !exists {
		t.Fatalf("literal quoted server 'literal.server' was lost!\n%s", updated)
	}
	// New server added
	if _, exists := mcpServers["new_tool"]; !exists {
		t.Fatalf("new_tool server was not added!\n%s", updated)
	}

	// Profiles section preserved
	profiles, ok := decoded["profiles"].(map[string]any)
	if !ok {
		t.Fatalf("profiles table was lost!")
	}
	if _, exists := profiles["staging.us-east-1"]; !exists {
		t.Fatalf("quoted profile 'staging.us-east-1' was lost!")
	}

	// Now test updating the quoted server
	updateQuoted := map[string]config.MCPServer{
		"server.with.dots": {Command: "updated-example", Args: []string{"--new-flag"}},
	}
	updated2, err := updateTomlMCPServers(updated, updateQuoted)
	if err != nil {
		t.Fatalf("updating quoted server failed: %v", err)
	}
	var decoded2 map[string]any
	if _, err := toml.Decode(updated2, &decoded2); err != nil {
		t.Fatalf("updated2 is invalid TOML: %v\nContent:\n%s", err, updated2)
	}
	mcp2 := decoded2["mcp_servers"].(map[string]any)
	dotSrv := mcp2["server.with.dots"].(map[string]any)
	if dotSrv["command"] != "updated-example" {
		t.Fatalf("quoted server 'server.with.dots' was not updated! Got: %v\nContent:\n%s", dotSrv["command"], updated2)
	}

	// Repeated install must not cause drift
	repeated, err := updateTomlMCPServers(updated2, updateQuoted)
	if err != nil {
		t.Fatalf("repeated update failed: %v", err)
	}
	if repeated != updated2 {
		t.Fatalf("drift detected:\nFirst:\n%s\nSecond:\n%s", updated2, repeated)
	}
}

// Case E: Unrelated settings survive MCP updates
func TestTOML_UnrelatedSettingsPreserved(t *testing.T) {
	initialTOML := `model = "example-model"
approval_policy = "on-request"

[profiles.development]
model = "another-model"
`
	servers := map[string]config.MCPServer{
		"my_server": {Command: "run-server"},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var decoded map[string]any
	if _, err := toml.Decode(updated, &decoded); err != nil {
		t.Fatalf("invalid TOML: %v", err)
	}

	if decoded["model"] != "example-model" {
		t.Errorf("model changed: %v", decoded["model"])
	}
	if decoded["approval_policy"] != "on-request" {
		t.Errorf("approval_policy changed: %v", decoded["approval_policy"])
	}

	profiles, ok := decoded["profiles"].(map[string]any)
	if !ok {
		t.Fatalf("profiles table missing")
	}
	dev, ok := profiles["development"].(map[string]any)
	if !ok {
		t.Fatalf("profiles.development missing")
	}
	if dev["model"] != "another-model" {
		t.Errorf("profiles.development.model changed: %v", dev["model"])
	}
}

// Case F: Existing user comments and blank lines preserved
func TestTOML_CommentsAndBlankLinesPreserved(t *testing.T) {
	initialTOML := `# Header comment
# Line 2 comment

model = "gpt-4" # inline comment

# Comment before section
[unrelated_section]
# Setting comment
key = "val"

# Footer comment
`
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx"},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	comments := []string{
		"# Header comment",
		"# Line 2 comment",
		"# inline comment",
		"# Comment before section",
		"# Setting comment",
		"# Footer comment",
	}
	for _, c := range comments {
		if !strings.Contains(updated, c) {
			t.Errorf("comment %q was lost:\n%s", c, updated)
		}
	}
}

// Case G: Malformed existing TOML returns error without modifying file
func TestTOML_MalformedExistingTOMLReturnsError(t *testing.T) {
	malformed := `model = "unclosed string
[mcp_servers]
`
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx"},
	}

	_, err := updateTomlMCPServers(malformed, servers)
	if err == nil {
		t.Fatalf("expected error on malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse existing TOML config") {
		t.Errorf("expected parse error, got: %v", err)
	}
}
