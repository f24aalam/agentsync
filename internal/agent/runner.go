package agent

import (
	"errors"
	"os"
	"path/filepath"
)

type RunSummary struct {
	Mode            string
	Results         []InstallResult
	ConfiguredCount int
}

func Run(agents []Agent, mode string) RunSummary {
	results := make([]InstallResult, 0, len(agents))
	configuredCount := 0

	// Install skills once for all agents using the shared directory
	if len(agents) > 0 {
		sharedDirs := UniqueSkillsDirs(agents)
		for _, sharedDir := range sharedDirs {
			if err := os.MkdirAll(sharedDir, 0o755); err != nil {
				// If we fail to create the shared directory, mark all agents as failed
				for i := range results {
					results[i].Steps = append(results[i].Steps, StepResult{
						Name:   "Skills",
						Target: sharedDir,
						Status: StepStatusError,
						Err:    err,
					})
				}
			} else {
				// Copy skills to the shared directory
				entries, err := os.ReadDir(".ai/skills")
				if err != nil {
					if !errors.Is(err, os.ErrNotExist) {
						for i := range results {
							results[i].Steps = append(results[i].Steps, StepResult{
								Name:   "Skills",
								Target: sharedDir,
								Status: StepStatusError,
								Err:    err,
							})
						}
					}
				} else {
					for _, entry := range entries {
						if !entry.IsDir() {
							continue
						}

						src := filepath.Join(".ai/skills", entry.Name())
						dst := filepath.Join(sharedDir, entry.Name())
						if err := copyDir(src, dst); err != nil {
							for i := range results {
								results[i].Steps = append(results[i].Steps, StepResult{
									Name:   "Skills",
									Target: sharedDir,
									Status: StepStatusError,
									Err:    err,
								})
							}

							break
						}
					}
					// Mark skills as installed for all agents
					for i := range results {
						results[i].Steps = append(results[i].Steps, StepResult{
							Name:   "Skills",
							Target: results[i].Agent.SkillsDir,
							Status: StepStatusOK,
						})
					}
				}
			}
		}
	}

	// Install guidelines and MCP for each agent individually
	for _, target := range agents {
		result := Install(target)
		// Replace the skipped skills step with the actual result from above
		// Find the skills step and update its status
		for i, step := range result.Steps {
			if step.Name == "Skills" && step.Status == StepStatusSkipped {
				// Look for the corresponding OK status from shared installation
				for _, res := range results {
					if res.Agent.ID == target.ID {
						for _, s := range res.Steps {
							if s.Name == "Skills" {
								result.Steps[i] = s

								break
							}
						}

						break
					}
				}

				break
			}
		}

		results = append(results, result)
		if result.Succeeded() {
			configuredCount++
		}
	}

	return RunSummary{
		Mode:            mode,
		Results:         results,
		ConfiguredCount: configuredCount,
	}
}
