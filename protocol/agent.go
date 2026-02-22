// Package protocol defines the shared types and message formats used between
// Forge (the server) and Hive (the local orchestrator). All communication
// between agents, the message bus, and the instruction system uses these types.
package protocol

import "time"

// CLIType identifies which AI coding CLI an agent wraps.
type CLIType string

const (
	CLIClaude CLIType = "claude"
	CLICodex  CLIType = "codex"
	CLIGemini CLIType = "gemini"
)

// Agent represents a single running AI CLI instance managed by Hive.
type Agent struct {
	ID        string    `json:"id"`
	CLIType   CLIType   `json:"cli_type"`
	Name      string    `json:"name"`       // human-readable label, e.g. "claude-alpha"
	IsAlpha   bool      `json:"is_alpha"`   // first of its type in the instruction block
	Worktree  string    `json:"worktree"`   // absolute path to this agent's worktree
	Branch    string    `json:"branch"`     // git branch this agent works on
	Status    AgentStatus `json:"status"`
	StartedAt time.Time `json:"started_at"`
}

// AgentStatus tracks the lifecycle of an agent.
type AgentStatus string

const (
	AgentIdle     AgentStatus = "idle"
	AgentWorking  AgentStatus = "working"
	AgentWaiting  AgentStatus = "waiting"  // waiting for another agent
	AgentReview   AgentStatus = "review"   // reviewing another agent's work
	AgentStopped  AgentStatus = "stopped"
)

// AgentGroup is a set of agents of the same CLI type, led by an alpha.
type AgentGroup struct {
	CLIType CLIType  `json:"cli_type"`
	Alpha   *Agent   `json:"alpha"`
	Members []*Agent `json:"members"`
}

// AgentRegistry holds all active agents indexed by ID.
type AgentRegistry struct {
	Agents map[string]*Agent      `json:"agents"`
	Groups map[CLIType]*AgentGroup `json:"groups"`
}

// NewAgentRegistry returns an empty registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		Agents: make(map[string]*Agent),
		Groups: make(map[CLIType]*AgentGroup),
	}
}

// Register adds an agent. The first agent of each CLIType becomes the alpha.
func (r *AgentRegistry) Register(a *Agent) {
	r.Agents[a.ID] = a

	group, exists := r.Groups[a.CLIType]
	if !exists {
		group = &AgentGroup{CLIType: a.CLIType}
		r.Groups[a.CLIType] = group
	}

	if group.Alpha == nil {
		a.IsAlpha = true
		group.Alpha = a
	}
	group.Members = append(group.Members, a)
}

// Alphas returns the alpha agent for each CLI type.
func (r *AgentRegistry) Alphas() []*Agent {
	var alphas []*Agent
	for _, g := range r.Groups {
		if g.Alpha != nil {
			alphas = append(alphas, g.Alpha)
		}
	}
	return alphas
}
