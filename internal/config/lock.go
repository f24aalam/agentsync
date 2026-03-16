package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type lockFile struct {
	Agents []string `toml:"agents"`
}

func ReadLock(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lock lockFile
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	return lock.Agents, nil
}

func WriteLock(path string, agents []string) error {
	quoted := make([]string, 0, len(agents))
	for _, agent := range agents {
		quoted = append(quoted, strconv.Quote(agent))
	}

	content := fmt.Sprintf("agents = [%s]\n", strings.Join(quoted, ", "))
	return os.WriteFile(path, []byte(content), 0o644)
}
