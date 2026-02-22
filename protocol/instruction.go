package protocol

import "time"

// InstructionSet is the top-level instruction document that the human operator
// writes. It declares which agents to spawn, what they should work on, and
// how they should coordinate.
type InstructionSet struct {
	Version   int                `json:"version" yaml:"version"`
	UpdatedAt time.Time          `json:"updated_at" yaml:"updated_at"`
	Project   ProjectConfig      `json:"project" yaml:"project"`
	Agents    []AgentSpec        `json:"agents" yaml:"agents"`
	Tasks     []TaskSpec         `json:"tasks" yaml:"tasks"`
	Rules     []string           `json:"rules,omitempty" yaml:"rules,omitempty"`
}

// ProjectConfig holds repo-level settings.
type ProjectConfig struct {
	Name       string `json:"name" yaml:"name"`
	Repo       string `json:"repo" yaml:"repo"`         // path or URL
	MainBranch string `json:"main_branch" yaml:"main_branch"`
}

// AgentSpec declares an agent to be spawned. The first agent of each cli_type
// listed becomes the alpha for that type.
type AgentSpec struct {
	CLIType     CLIType           `json:"cli_type" yaml:"cli_type"`
	Count       int               `json:"count" yaml:"count"`         // how many instances
	SystemPrompt string           `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// TaskSpec defines a unit of work to be assigned.
type TaskSpec struct {
	ID          string   `json:"id" yaml:"id"`
	Title       string   `json:"title" yaml:"title"`
	Description string   `json:"description" yaml:"description"`
	AssignTo    string   `json:"assign_to,omitempty" yaml:"assign_to,omitempty"` // agent ID, cli type, or "alphas"
	DependsOn   []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Priority    int      `json:"priority,omitempty" yaml:"priority,omitempty"`
	Status      TaskStatus `json:"status" yaml:"status"`
}

// TaskStatus tracks progress.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskAssigned   TaskStatus = "assigned"
	TaskInProgress TaskStatus = "in_progress"
	TaskInReview   TaskStatus = "in_review"
	TaskDone       TaskStatus = "done"
)

// InstructionUpdate is sent when the operator changes instructions mid-session.
type InstructionUpdate struct {
	NewInstructions InstructionSet `json:"new_instructions"`
	ChangesSummary  string         `json:"changes_summary"` // human-readable diff
	GracePeriodSec  int           `json:"grace_period_sec"` // agents finish current step before switching
}
