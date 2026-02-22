// Package worktree manages git worktrees for individual agents. Each agent
// gets its own worktree so they can work on the same repo simultaneously
// without conflicts. The alpha agent's worktree can merge from others.
package worktree

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Info describes a worktree assigned to an agent.
type Info struct {
	AgentID string
	Path    string
	Branch  string
}

// Manager handles worktree lifecycle for a local repo.
type Manager struct {
	repoPath    string
	worktreeDir string
}

// NewManager creates a worktree manager for the given repo.
func NewManager(repoPath string) *Manager {
	absPath, _ := filepath.Abs(repoPath)
	return &Manager{
		repoPath:    absPath,
		worktreeDir: filepath.Join(absPath, ".forge-worktrees"),
	}
}

// CreateForAgent makes a new worktree for the given agent, branching from baseBranch.
func (m *Manager) CreateForAgent(agentID, baseBranch string) (*Info, error) {
	branch := "agent/" + agentID
	wtPath := filepath.Join(m.worktreeDir, agentID)

	// Create the branch from base.
	cmd := exec.Command("git", "-C", m.repoPath, "branch", branch, baseBranch)
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already exists") {
		return nil, fmt.Errorf("create branch %s: %s: %w", branch, string(out), err)
	}

	// Add the worktree.
	cmd = exec.Command("git", "-C", m.repoPath, "worktree", "add", wtPath, branch)
	out, err = cmd.CombinedOutput()
	if err != nil {
		// If worktree already exists, that's fine.
		if !strings.Contains(string(out), "already exists") {
			return nil, fmt.Errorf("add worktree: %s: %w", string(out), err)
		}
	}

	return &Info{
		AgentID: agentID,
		Path:    wtPath,
		Branch:  branch,
	}, nil
}

// RemoveForAgent cleans up a worktree.
func (m *Manager) RemoveForAgent(agentID string) error {
	wtPath := filepath.Join(m.worktreeDir, agentID)

	cmd := exec.Command("git", "-C", m.repoPath, "worktree", "remove", wtPath, "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove worktree: %s: %w", string(out), err)
	}
	return nil
}

// List returns all agent worktrees.
func (m *Manager) List() ([]Info, error) {
	cmd := exec.Command("git", "-C", m.repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list: %s: %w", string(out), err)
	}

	var results []Info
	var current Info
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				results = append(results, current)
			}
			current = Info{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch refs/heads/")
			current.Branch = ref
			if strings.HasPrefix(ref, "agent/") {
				current.AgentID = strings.TrimPrefix(ref, "agent/")
			}
		}
	}
	if current.Path != "" {
		results = append(results, current)
	}
	return results, nil
}

// MergeAgent merges an agent's branch into the target branch (usually main).
func (m *Manager) MergeAgent(agentID, targetBranch string) error {
	agentBranch := "agent/" + agentID

	cmd := exec.Command("git", "-C", m.repoPath, "checkout", targetBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %s: %s: %w", targetBranch, string(out), err)
	}

	cmd = exec.Command("git", "-C", m.repoPath, "merge", agentBranch, "--no-ff",
		"-m", fmt.Sprintf("Merge agent/%s into %s", agentID, targetBranch))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("merge: %s: %w", string(out), err)
	}
	return nil
}
