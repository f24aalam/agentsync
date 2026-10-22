package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
)

// LockFile represents the minimal ownership metadata persisted in .ai/sync.lock.
type LockFile struct {
	Agents []string            `toml:"agents"`
	Skills map[string][]string `toml:"skills,omitempty"`
	MCP    map[string][]string `toml:"mcp,omitempty"`
}

// NewLockFile creates an initialized LockFile with empty collections.
func NewLockFile() *LockFile {
	return &LockFile{
		Agents: []string{},
		Skills: make(map[string][]string),
		MCP:    make(map[string][]string),
	}
}

// IsSkillPackageManaged reports whether a skill package in destSkillsDir is recorded as managed.
func (l *LockFile) IsSkillPackageManaged(destSkillsDir, skillName string) bool {
	if l == nil || l.Skills == nil {
		return false
	}
	cleanDir := filepath.Clean(destSkillsDir)
	for dir, packages := range l.Skills {
		if filepath.Clean(dir) == cleanDir {
			for _, pkg := range packages {
				if pkg == skillName {
					return true
				}
			}
		}
	}
	return false
}

// RecordSkillPackage records a skill package as managed.
func (l *LockFile) RecordSkillPackage(destSkillsDir, skillName string) {
	if l == nil {
		return
	}
	if l.Skills == nil {
		l.Skills = make(map[string][]string)
	}
	cleanDir := filepath.Clean(destSkillsDir)
	for _, existing := range l.Skills[cleanDir] {
		if existing == skillName {
			return
		}
	}
	l.Skills[cleanDir] = append(l.Skills[cleanDir], skillName)
	slices.Sort(l.Skills[cleanDir])
}

// HasTrackedSkills reports whether destSkillsDir has any managed skill packages.
func (l *LockFile) HasTrackedSkills(destSkillsDir string) bool {
	if l == nil || l.Skills == nil {
		return false
	}
	cleanDir := filepath.Clean(destSkillsDir)
	for dir, packages := range l.Skills {
		if filepath.Clean(dir) == cleanDir && len(packages) > 0 {
			return true
		}
	}
	return false
}

// IsMCPManaged reports whether an MCP server name in destFile is recorded as managed.
func (l *LockFile) IsMCPManaged(destFile, serverName string) bool {
	if l == nil || l.MCP == nil {
		return false
	}
	cleanDest := filepath.Clean(destFile)
	for destKey, servers := range l.MCP {
		if filepath.Clean(destKey) == cleanDest {
			for _, srv := range servers {
				if srv == serverName {
					return true
				}
			}
		}
	}
	return false
}

// RecordMCP records an MCP server name in destFile as managed.
func (l *LockFile) RecordMCP(destFile, serverName string) {
	if l == nil {
		return
	}
	if l.MCP == nil {
		l.MCP = make(map[string][]string)
	}
	cleanDest := filepath.Clean(destFile)
	for _, existing := range l.MCP[cleanDest] {
		if existing == serverName {
			return
		}
	}
	l.MCP[cleanDest] = append(l.MCP[cleanDest], serverName)
	slices.Sort(l.MCP[cleanDest])
}

// ReadFullLock reads and parses the complete .ai/sync.lock file.
func ReadFullLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lock LockFile
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("cannot parse lock file %s: %w", path, err)
	}

	if lock.Agents == nil {
		lock.Agents = []string{}
	}
	if lock.Skills == nil {
		lock.Skills = make(map[string][]string)
	}
	if lock.MCP == nil {
		lock.MCP = make(map[string][]string)
	}

	return &lock, nil
}

// WriteFullLock writes the complete LockFile to path in TOML format.
// If destination content is identical to the serialized lock, write is skipped to preserve mtime.
func WriteFullLock(path string, lock *LockFile) error {
	if lock == nil {
		lock = NewLockFile()
	}
	if lock.Agents == nil {
		lock.Agents = []string{}
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(lock); err != nil {
		return fmt.Errorf("failed to encode sync.lock: %w", err)
	}

	newData := buf.Bytes()
	if existingData, err := os.ReadFile(path); err == nil && bytes.Equal(existingData, newData) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, newData, 0o644)
}

// ReadLock reads the selected agent IDs from .ai/sync.lock.
func ReadLock(path string) ([]string, error) {
	lock, err := ReadFullLock(path)
	if err != nil {
		return nil, err
	}
	return lock.Agents, nil
}

// WriteLock writes or updates the selected agent IDs in .ai/sync.lock,
// preserving any existing skill or MCP ownership metadata.
func WriteLock(path string, agents []string) error {
	lock, err := ReadFullLock(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		lock = NewLockFile()
	}

	if agents == nil {
		lock.Agents = []string{}
	} else {
		lock.Agents = agents
	}

	return WriteFullLock(path, lock)
}
