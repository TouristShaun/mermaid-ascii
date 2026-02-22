// Package git provides repository management for the forge server.
// It handles bare repo creation, cloning, worktree management, and
// branch operations that agents use for pair programming.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo represents a git repository managed by the forge.
type Repo struct {
	ID       string
	Name     string
	BarePath string // path to the bare repo on the server
}

// Manager handles repository lifecycle operations.
type Manager struct {
	baseDir string // root directory for all repos
}

// NewManager creates a new repo manager rooted at baseDir.
func NewManager(baseDir string) *Manager {
	return &Manager{baseDir: baseDir}
}

// Init creates a new bare repository.
func (m *Manager) Init(name string) (*Repo, error) {
	barePath := filepath.Join(m.baseDir, name+".git")
	if err := os.MkdirAll(barePath, 0750); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	cmd := exec.Command("git", "init", "--bare", barePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git init --bare: %s: %w", string(out), err)
	}

	return &Repo{
		ID:       name,
		Name:     name,
		BarePath: barePath,
	}, nil
}

// List returns all repositories.
func (m *Manager) List() ([]*Repo, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var repos []*Repo
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".git") {
			name := strings.TrimSuffix(e.Name(), ".git")
			repos = append(repos, &Repo{
				ID:       name,
				Name:     name,
				BarePath: filepath.Join(m.baseDir, e.Name()),
			})
		}
	}
	return repos, nil
}

// Delete removes a repository.
func (m *Manager) Delete(name string) error {
	barePath := filepath.Join(m.baseDir, name+".git")
	return os.RemoveAll(barePath)
}

// RepoPath returns the filesystem path for a repo name.
func (m *Manager) RepoPath(name string) string {
	return filepath.Join(m.baseDir, name+".git")
}
