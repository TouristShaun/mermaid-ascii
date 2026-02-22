// Package agent manages the lifecycle of AI CLI instances. It spawns
// processes, assigns worktrees, designates alphas, and handles graceful
// shutdown and instruction updates.
package agent

import (
	"fmt"
	"log"
	"sync"

	"github.com/AlexanderGrooff/mermaid-ascii/hive/internal/bridge"
	"github.com/AlexanderGrooff/mermaid-ascii/hive/internal/worktree"
	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// Manager orchestrates all running agents.
type Manager struct {
	mu       sync.Mutex
	client   *bridge.Client
	wtMgr    *worktree.Manager
	instrSet *protocol.InstructionSet
	registry *protocol.AgentRegistry
	runners  map[string]*Runner
}

// NewManager creates an agent manager.
func NewManager(client *bridge.Client, wtMgr *worktree.Manager, instrSet *protocol.InstructionSet) *Manager {
	return &Manager{
		client:   client,
		wtMgr:    wtMgr,
		instrSet: instrSet,
		registry: protocol.NewAgentRegistry(),
		runners:  make(map[string]*Runner),
	}
}

// SpawnAll creates and starts agents according to the instruction set.
// The first agent of each CLI type becomes the alpha.
func (m *Manager) SpawnAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, spec := range m.instrSet.Agents {
		for i := 0; i < spec.Count; i++ {
			agentID := fmt.Sprintf("%s-%d", spec.CLIType, i)
			name := string(spec.CLIType)
			if i == 0 {
				name += "-alpha"
			} else {
				name += fmt.Sprintf("-%d", i)
			}

			// Create worktree for this agent.
			wt, err := m.wtMgr.CreateForAgent(agentID, m.instrSet.Project.MainBranch)
			if err != nil {
				return fmt.Errorf("create worktree for %s: %w", agentID, err)
			}

			agent := &protocol.Agent{
				ID:       agentID,
				CLIType:  spec.CLIType,
				Name:     name,
				Worktree: wt.Path,
				Branch:   wt.Branch,
				Status:   protocol.AgentIdle,
			}
			m.registry.Register(agent)

			// Build the system prompt, incorporating the base prompt plus
			// agent-specific context (alpha status, worktree path, etc).
			sysPrompt := buildSystemPrompt(agent, spec, m.instrSet)

			// Start the CLI runner.
			runner, err := NewRunner(agent, sysPrompt, spec.Env)
			if err != nil {
				return fmt.Errorf("create runner for %s: %w", agentID, err)
			}
			m.runners[agentID] = runner

			if err := runner.Start(); err != nil {
				return fmt.Errorf("start runner %s: %w", agentID, err)
			}

			// Register with forge.
			if err := m.client.RegisterAgent(agent); err != nil {
				log.Printf("warning: failed to register %s with forge: %v", agentID, err)
			}

			log.Printf("spawned %s (alpha=%v, worktree=%s)", name, agent.IsAlpha, wt.Path)
		}
	}

	return nil
}

// StopAll gracefully stops all running agents.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, runner := range m.runners {
		log.Printf("stopping %s...", id)
		runner.Stop()
		// Clean up worktree.
		m.wtMgr.RemoveForAgent(id)
	}
}

// UpdateInstructions handles a hot-reload of instructions.
func (m *Manager) UpdateInstructions(newInstr *protocol.InstructionSet) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.instrSet = newInstr

	// Rebuild system prompts and signal runners to refresh.
	for _, spec := range newInstr.Agents {
		for _, member := range m.registry.Groups[spec.CLIType].Members {
			if runner, ok := m.runners[member.ID]; ok {
				sysPrompt := buildSystemPrompt(member, spec, newInstr)
				runner.UpdatePrompt(sysPrompt)
			}
		}
	}
}

func buildSystemPrompt(agent *protocol.Agent, spec protocol.AgentSpec, instrSet *protocol.InstructionSet) string {
	prompt := fmt.Sprintf(`You are %s, an AI coding agent in the Forge system.
CLI Type: %s
Agent ID: %s
Alpha: %v
Worktree: %s
Branch: %s
Project: %s

`, agent.Name, agent.CLIType, agent.ID, agent.IsAlpha, agent.Worktree, agent.Branch, instrSet.Project.Name)

	if agent.IsAlpha {
		prompt += `As the ALPHA agent for your CLI type, you coordinate work among your group members.
You can delegate sub-tasks, review their work, and merge their branches.

`
	}

	if spec.SystemPrompt != "" {
		prompt += spec.SystemPrompt + "\n\n"
	}

	prompt += "## Current Tasks\n\n"
	for _, task := range instrSet.Tasks {
		prompt += fmt.Sprintf("- [%s] %s: %s\n", task.Status, task.Title, task.Description)
	}

	if len(instrSet.Rules) > 0 {
		prompt += "\n## Rules\n\n"
		for _, rule := range instrSet.Rules {
			prompt += "- " + rule + "\n"
		}
	}

	prompt += `
## Communication
You can speak to other agents through the Forge message bus.
Send messages by writing to stdout in the format: @<agent-id> <message>
The system will intercept these and route them through the bus.
Messages from other agents will appear as system notifications.
`

	return prompt
}
