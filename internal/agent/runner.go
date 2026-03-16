package agent

type RunSummary struct {
	Mode            string
	Results         []InstallResult
	ConfiguredCount int
}

func Run(agents []Agent, mode string) RunSummary {
	results := make([]InstallResult, 0, len(agents))
	configuredCount := 0

	for _, target := range agents {
		result := Install(target)
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
