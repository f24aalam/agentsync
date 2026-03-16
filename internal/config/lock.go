package config

import (
	"bytes"
	"os"

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
	lock := lockFile{Agents: agents}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(lock); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
