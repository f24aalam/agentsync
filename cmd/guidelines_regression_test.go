package cmd

import (
	"strings"
	"testing"

	agentpkg "github.com/f24aalam/agentsync/internal/agent"
	stepflow "github.com/f24aalam/stepflow/pkg"
)

// Scenario 1: Identical managed content causes no prompt or write.
func TestRegression_Guidelines_IdenticalManagedCausesNoPrompt(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"codex"})

	// First install sets up AGENTS.md
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	targets := []agentpkg.Agent{mustGetAgent("codex")}
	conflicts, err := agentpkg.DetectInstallConflicts(targets, env.RootDir)
	if err != nil {
		t.Fatalf("detect conflicts failed: %v", err)
	}

	for _, c := range conflicts {
		if c.Kind == agentpkg.KindGuidelines {
			t.Fatalf("expected no guidelines conflict when identical, got %+v", c)
		}
	}
}

// Scenario 2: Different managed content prompts.
// Scenario 3: Approval updates only the managed section.
// Scenario 9: LF/CRLF and unrelated content remain preserved.
func TestRegression_Guidelines_DifferentManagedPromptsAndApprovalUpdatesOnlyManagedSection(t *testing.T) {
	env := newIntegrationEnv(t)

	// User content outside managed section with CRLF line endings
	initialContent := "# User Header\r\n\r\nCustom user preamble.\r\n\r\n<!-- BEGIN AGENTSYNC MANAGED -->\r\nOld managed content\r\n<!-- END AGENTSYNC MANAGED -->\r\n\r\nFooter notes by developer.\r\n"
	env.WriteFile("AGENTS.md", initialContent)

	env.WriteFile(".ai/guidelines/core.md", "# New Guidelines\nDo this.\n")
	env.WriteFile(".ai/sync.lock", `agents = ["codex"]`+"\n")

	targets := []agentpkg.Agent{mustGetAgent("codex")}
	conflicts, err := agentpkg.DetectInstallConflicts(targets, env.RootDir)
	if err != nil {
		t.Fatalf("detect conflicts failed: %v", err)
	}

	if len(conflicts) != 1 || conflicts[0].Kind != agentpkg.KindGuidelines {
		t.Fatalf("expected 1 guidelines conflict, got: %+v", conflicts)
	}
	if !conflicts[0].IsManaged {
		t.Fatalf("expected IsManaged to be true for valid managed section")
	}

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	prompted := false
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			if s.Key() == conflicts[0].StepKey {
				prompted = true
				res[s.Key()] = "Yes" // Approve
			}
		}
		return res, nil
	}

	_, _, err = env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if !prompted {
		t.Fatalf("expected interactive prompt for modified managed section")
	}

	content := env.ReadFile("AGENTS.md")
	if !strings.Contains(content, "# User Header") || !strings.Contains(content, "Custom user preamble.") {
		t.Errorf("preamble outside markers was lost! Content:\n%s", content)
	}
	if !strings.Contains(content, "Footer notes by developer.") {
		t.Errorf("footer outside markers was lost! Content:\n%s", content)
	}
	if !strings.Contains(content, "New Guidelines") {
		t.Errorf("new managed content was not injected! Content:\n%s", content)
	}
	if strings.Contains(content, "Old managed content") {
		t.Errorf("old managed content was not replaced! Content:\n%s", content)
	}
	if !strings.Contains(content, "\r\n") {
		t.Errorf("CRLF line endings were not preserved! Content:\n%q", content)
	}
}

// Scenario 4: Rejection leaves the whole file unchanged.
func TestRegression_Guidelines_RejectionLeavesFileUnchanged(t *testing.T) {
	env := newIntegrationEnv(t)

	initialContent := "# User Header\n\n<!-- BEGIN AGENTSYNC MANAGED -->\nOld managed content\n<!-- END AGENTSYNC MANAGED -->\n\nFooter.\n"
	env.WriteFile("AGENTS.md", initialContent)

	env.WriteFile(".ai/guidelines/core.md", "# New Guidelines\nDo this.\n")
	env.WriteFile(".ai/sync.lock", `agents = ["codex"]`+"\n")

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			res[s.Key()] = "No" // Reject
		}
		return res, nil
	}

	_, _, err := env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	content := env.ReadFile("AGENTS.md")
	if content != initialContent {
		t.Fatalf("rejected install modified guidelines file!\nExpected:\n%s\nGot:\n%s", initialContent, content)
	}
}

// Scenario 5: --yes updates valid managed content without prompt.
func TestRegression_Guidelines_YesUpdatesValidManagedContent(t *testing.T) {
	env := newIntegrationEnv(t)

	initialContent := "# User Header\n\n<!-- BEGIN AGENTSYNC MANAGED -->\nOld content\n<!-- END AGENTSYNC MANAGED -->\n"
	env.WriteFile("AGENTS.md", initialContent)

	env.WriteFile(".ai/guidelines/core.md", "# Fresh Guidelines\nUpdated.\n")
	env.WriteFile(".ai/sync.lock", `agents = ["codex"]`+"\n")

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		t.Fatalf("unexpected prompt with --yes")
		return nil, nil
	}

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y failed: %v", err)
	}

	content := env.ReadFile("AGENTS.md")
	if !strings.Contains(content, "# Fresh Guidelines") {
		t.Errorf("expected updated guidelines with --yes, got:\n%s", content)
	}
	if !strings.Contains(content, "# User Header") {
		t.Errorf("user content was lost with --yes, got:\n%s", content)
	}
}

// Scenario 6: Existing unmarked file requires adoption approval; --yes must not silently adopt it.
func TestRegression_Guidelines_UnmarkedFileRequiresAdoptionApproval(t *testing.T) {
	env := newIntegrationEnv(t)

	unmarkedOriginal := "# Unmarked Developer Guidelines\nDo not touch without asking.\n"
	env.WriteFile("AGENTS.md", unmarkedOriginal)

	env.WriteFile(".ai/guidelines/core.md", "# AI Guidelines\nSynced content.\n")
	env.WriteFile(".ai/sync.lock", `agents = ["codex"]`+"\n")

	// 1. With --yes, unmarked file MUST NOT be adopted
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y returned error: %v", err)
	}

	content := env.ReadFile("AGENTS.md")
	if content != unmarkedOriginal {
		t.Fatalf("SAFETY VIOLATION: --yes silently modified unmarked AGENTS.md!\nGot:\n%s", content)
	}

	// 2. Interactive approval adopts it
	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			res[s.Key()] = "Yes" // Approve adoption
		}
		return res, nil
	}

	_, _, err = env.RunInstall()
	if err != nil {
		t.Fatalf("interactive install returned error: %v", err)
	}

	contentAdopted := env.ReadFile("AGENTS.md")
	if !strings.Contains(contentAdopted, "# Unmarked Developer Guidelines") {
		t.Errorf("original content was wiped during adoption!\n%s", contentAdopted)
	}
	if !strings.Contains(contentAdopted, "# AI Guidelines") {
		t.Errorf("managed guidelines were not added during adoption!\n%s", contentAdopted)
	}
}

// Scenario 8: Shared AGENTS.md prompts once, and rejection prevents all agents from rewriting it.
func TestRegression_Guidelines_SharedAgentsMDPromptsOnceAndOneRejectionStopsAll(t *testing.T) {
	env := newIntegrationEnv(t)

	initialContent := "# Shared User Guidelines\n<!-- BEGIN AGENTSYNC MANAGED -->\nv1\n<!-- END AGENTSYNC MANAGED -->\n"
	env.WriteFile("AGENTS.md", initialContent)

	env.WriteFile(".ai/guidelines/core.md", "# v2\n")
	env.PopulateStandardAI([]string{"codex", "github-copilot", "opencode"})

	targets := []agentpkg.Agent{
		mustGetAgent("codex"),
		mustGetAgent("github-copilot"),
		mustGetAgent("opencode"),
	}
	conflicts, err := agentpkg.DetectInstallConflicts(targets, env.RootDir)
	if err != nil {
		t.Fatalf("detect conflicts failed: %v", err)
	}

	guidelinePrompts := 0
	for _, c := range conflicts {
		if c.Kind == agentpkg.KindGuidelines {
			guidelinePrompts++
		}
	}
	if guidelinePrompts != 1 {
		t.Fatalf("expected exactly 1 consolidated conflict prompt for shared AGENTS.md, got %d", guidelinePrompts)
	}

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			if strings.Contains(s.Key(), "guidelines") {
				res[s.Key()] = "No" // Reject
			} else {
				res[s.Key()] = "Yes"
			}
		}
		return res, nil
	}

	_, _, err = env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	after := env.ReadFile("AGENTS.md")
	if after != initialContent {
		t.Fatalf("shared AGENTS.md was modified after rejection!\nGot:\n%s", after)
	}
}
