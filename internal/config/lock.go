package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ResourceLock records ownership metadata and a content hash for a managed resource.
type ResourceLock struct {
	Hash string `toml:"hash"`
}

// LockFile represents the minimal ownership metadata persisted in .ai/sync.lock.
type LockFile struct {
	Agents     []string                           `toml:"agents"`
	Guidelines map[string]ResourceLock            `toml:"guidelines,omitempty"`
	Skills     map[string]ResourceLock            `toml:"skills,omitempty"`
	MCP        map[string]map[string]ResourceLock `toml:"mcp,omitempty"`
}

// NewLockFile creates an initialized LockFile with empty collections.
func NewLockFile() *LockFile {
	return &LockFile{
		Agents:     []string{},
		Guidelines: make(map[string]ResourceLock),
		Skills:     make(map[string]ResourceLock),
		MCP:        make(map[string]map[string]ResourceLock),
	}
}

// HashBytes computes the SHA-256 hex string of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashString computes the SHA-256 hex string of s.
func HashString(s string) string {
	return HashBytes([]byte(s))
}

// ReadFullLock reads and parses the complete .ai/sync.lock file.
// If the lock file was created by an older version containing only `agents = [...]`,
// it safely initializes empty resource maps without assuming ownership of existing resources.
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
	if lock.Guidelines == nil {
		lock.Guidelines = make(map[string]ResourceLock)
	}
	if lock.Skills == nil {
		lock.Skills = make(map[string]ResourceLock)
	}
	if lock.MCP == nil {
		lock.MCP = make(map[string]map[string]ResourceLock)
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

// ReadLock reads the selected agent IDs from .ai/sync.lock, preserving backward compatibility.
func ReadLock(path string) ([]string, error) {
	lock, err := ReadFullLock(path)
	if err != nil {
		return nil, err
	}
	return lock.Agents, nil
}

// WriteLock writes or updates the selected agent IDs in .ai/sync.lock,
// preserving any existing guideline, skill, or MCP ownership metadata.
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
