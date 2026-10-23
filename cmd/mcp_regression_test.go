package cmd

import (
	"strings"
	"testing"

	stepflow "github.com/f24aalam/stepflow/pkg"
)

// Scenario 1: Missing MCP server gets added automatically without prompt.
func TestRegression_MCP_MissingServerAddedAutomatically(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".cursor/mcp.json", `{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  }
}`)
	env.PopulateStandardAI([]string{"cursor"})

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		for _, s := range steps {
			if strings.Contains(s.Key(), "mcp") {
				t.Fatalf("unexpected prompt for missing MCP server: %s", s.Key())
			}
		}
		return stepflow.Result{}, nil
	}

	// Interactive install without --yes
	_, _, err := env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	env.AssertFileContains(".cursor/mcp.json", "filesystem")
	env.AssertFileContains(".cursor/mcp.json", "postgres")
}

// Scenario 2: install --yes adds missing servers without skipping destination.
func TestRegression_MCP_InstallYesAddsMissingServers(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".cursor/mcp.json", `{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  }
}`)
	env.PopulateStandardAI([]string{"cursor"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y failed: %v", err)
	}

	env.AssertFileContains(".cursor/mcp.json", "filesystem")
	env.AssertFileContains(".cursor/mcp.json", "postgres")
}

// Scenario 3: Existing managed conflict is overwritten with --yes.
func TestRegression_MCP_ExistingManagedConflictOverwrittenWithYes(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"codex"})

	// First install sets up managed postgres
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	env.AssertFileContains(".codex/config.toml", "@modelcontextprotocol/server-postgres")

	// Now update .ai/mcp.toml with new postgres arguments
	env.WriteFile(".ai/mcp.toml", `
[servers.postgres]
type = "local"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres", "--v2"]
`)

	// Run install --yes: managed postgres should update to --v2
	_, _, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	env.AssertFileContains(".codex/config.toml", "--v2")
}

// Scenario 4: Existing unmanaged conflict is preserved with --yes.
// Scenario 9: No destination is skipped merely because one entry conflicts.
func TestRegression_MCP_ExistingUnmanagedConflictPreservedWithYes(t *testing.T) {
	env := newIntegrationEnv(t)

	// Pre-existing unmanaged config with custom conflicting postgres and unrelated user tool
	env.WriteFile(".codex/config.toml", `# Codex configuration
model = "o3-mini"

[mcp_servers.postgres]
command = "my-custom-postgres-proxy"

[mcp_servers.my_tool]
command = "my-tool"
`)

	// .ai/mcp.toml has postgres and filesystem
	env.PopulateStandardAI([]string{"codex"})

	// Running install --yes must NOT overwrite unmanaged postgres,
	// but MUST add missing filesystem, and MUST preserve my_tool and model setting.
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y failed: %v", err)
	}

	codex := env.ReadFile(".codex/config.toml")
	if !strings.Contains(codex, `command = "my-custom-postgres-proxy"`) {
		t.Errorf("unmanaged conflicting postgres was overwritten by --yes! Content:\n%s", codex)
	}
	if !strings.Contains(codex, "my_tool") {
		t.Errorf("unrelated my_tool was lost! Content:\n%s", codex)
	}
	if !strings.Contains(codex, `model = "o3-mini"`) {
		t.Errorf("unrelated model setting was lost! Content:\n%s", codex)
	}
	if !strings.Contains(codex, "@modelcontextprotocol/server-filesystem") {
		t.Errorf("missing filesystem was not added because postgres was conflicting! Content:\n%s", codex)
	}
}

// Scenario 5: Interactive approval overwrites only conflicting entries.
func TestRegression_MCP_InteractiveApprovalOverwritesOnlyConflictingEntries(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".cursor/mcp.json", `{
  "custom_key": "val",
  "mcpServers": {
    "postgres": { "command": "old-postgres" },
    "user_server": { "command": "keep-me" }
  }
}`)
	env.PopulateStandardAI([]string{"cursor"})

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	prompted := false
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			if strings.Contains(s.Key(), "mcp") {
				prompted = true
				res[s.Key()] = "Yes" // Approve
			} else {
				res[s.Key()] = "Yes"
			}
		}
		return res, nil
	}

	_, _, err := env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if !prompted {
		t.Fatalf("expected prompt for conflicting unmanaged server postgres")
	}

	cursor := env.ReadFile(".cursor/mcp.json")
	if !strings.Contains(cursor, "@modelcontextprotocol/server-postgres") {
		t.Errorf("approved postgres was not updated! Content:\n%s", cursor)
	}
	if !strings.Contains(cursor, "keep-me") {
		t.Errorf("user_server was lost! Content:\n%s", cursor)
	}
	if !strings.Contains(cursor, "custom_key") {
		t.Errorf("custom_key was lost! Content:\n%s", cursor)
	}
	if !strings.Contains(cursor, "@modelcontextprotocol/server-filesystem") {
		t.Errorf("missing filesystem was not added! Content:\n%s", cursor)
	}
}

// Scenario 6: Interactive rejection preserves conflicting entries but allows safe additions.
func TestRegression_MCP_InteractiveRejectionPreservesConflictingEntriesAllowsSafeAdditions(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".cursor/mcp.json", `{
  "custom_key": "val",
  "mcpServers": {
    "postgres": { "command": "old-postgres" },
    "user_server": { "command": "keep-me" }
  }
}`)
	env.PopulateStandardAI([]string{"cursor"})

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			if strings.Contains(s.Key(), "mcp") {
				res[s.Key()] = "No" // Reject
			} else {
				res[s.Key()] = "Yes"
			}
		}
		return res, nil
	}

	_, _, err := env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	cursor := env.ReadFile(".cursor/mcp.json")
	if !strings.Contains(cursor, `"command": "old-postgres"`) {
		t.Errorf("rejected postgres was overwritten! Content:\n%s", cursor)
	}
	if !strings.Contains(cursor, "keep-me") {
		t.Errorf("user_server was lost! Content:\n%s", cursor)
	}
	if !strings.Contains(cursor, "custom_key") {
		t.Errorf("custom_key was lost! Content:\n%s", cursor)
	}
	if !strings.Contains(cursor, "@modelcontextprotocol/server-filesystem") {
		t.Errorf("missing filesystem was not added on rejection! Content:\n%s", cursor)
	}
}
