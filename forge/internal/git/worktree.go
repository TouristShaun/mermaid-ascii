package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeInfo describes a git worktree created for an agent.
type WorktreeInfo struct {
	AgentID string
	Path    string
	Branch  string
	RepoName string
}

// WorktreeManager handles creating and cleaning up worktrees from a clone.
type WorktreeManager struct {
	cloneDir    string // directory where we keep the main clone
	worktreeDir string // directory where worktrees live
}

// NewWorktreeManager creates a manager for a specific repo clone.
func NewWorktreeManager(cloneDir, worktreeDir string) *WorktreeManager {
	return &WorktreeManager{
		cloneDir:    cloneDir,
		worktreeDir: worktreeDir,
	}
}

// Create makes a new worktree for an agent on a dedicated branch.
func (wm *WorktreeManager) Create(agentID, baseBranch string) (*WorktreeInfo, error) {
	branch := fmt.Sprintf("agent/%s", agentID)
	wtPath := filepath.Join(wm.worktreeDir, agentID)

	if err := os.MkdirAll(wm.worktreeDir, 0750); err != nil {
		return nil, fmt.Errorf("mkdir worktree dir: %w", err)
	}

	// Create branch from base.
	cmd := exec.Command("git", "-C", wm.cloneDir, "branch", branch, baseBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Branch may already exist, that's ok.
		if !strings.Contains(string(out), "already exists") {
			return nil, fmt.Errorf("create branch: %s: %w", string(out), err)
		}
	}

	// Add worktree.
	cmd = exec.Command("git", "-C", wm.cloneDir, "worktree", "add", wtPath, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("add worktree: %s: %w", string(out), err)
	}

	return &WorktreeInfo{
		AgentID: agentID,
		Path:    wtPath,
		Branch:  branch,
	}, nil
}

// Remove cleans up a worktree.
func (wm *WorktreeManager) Remove(agentID string) error {
	wtPath := filepath.Join(wm.worktreeDir, agentID)

	cmd := exec.Command("git", "-C", wm.cloneDir, "worktree", "remove", wtPath, "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove worktree: %s: %w", string(out), err)
	}
	return nil
}

// List returns all worktrees.
func (wm *WorktreeManager) List() ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "-C", wm.cloneDir, "worktree", "list", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %s: %w", string(out), err)
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
			// Extract agent ID from branch name.
			if strings.HasPrefix(current.Branch, "agent/") {
				current.AgentID = strings.TrimPrefix(current.Branch, "agent/")
			}
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees, nil
}

// Sync fetches latest and rebases a worktree's branch onto the base branch.
func (wm *WorktreeManager) Sync(agentID, baseBranch string) error {
	wtPath := filepath.Join(wm.worktreeDir, agentID)

	cmd := exec.Command("git", "-C", wtPath, "fetch", "origin")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fetch: %s: %w", string(out), err)
	}

	cmd = exec.Command("git", "-C", wtPath, "rebase", "origin/"+baseBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rebase: %s: %w", string(out), err)
	}
	return nil
}
