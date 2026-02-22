package protocol

import "time"

// MessageType classifies what a message is about on the bus.
type MessageType string

const (
	// Agent-to-agent communication
	MsgChat        MessageType = "chat"        // free-form discussion
	MsgCodeReview  MessageType = "code_review"  // request or deliver a code review
	MsgSuggestion  MessageType = "suggestion"   // propose an enhancement
	MsgTaskAssign  MessageType = "task_assign"  // alpha delegates a sub-task
	MsgTaskUpdate  MessageType = "task_update"  // progress report on a task
	MsgTaskDone    MessageType = "task_done"    // task completed

	// System messages
	MsgInstruction  MessageType = "instruction"    // new or updated instruction set
	MsgWorktreeSync MessageType = "worktree_sync"  // signal to rebase/merge
	MsgAgentJoin    MessageType = "agent_join"      // new agent came online
	MsgAgentLeave   MessageType = "agent_leave"     // agent went offline

	// Mesh / inter-app messages
	MsgMeshDiscover MessageType = "mesh_discover"  // app discovery
	MsgMeshRequest  MessageType = "mesh_request"   // cross-app request
	MsgMeshResponse MessageType = "mesh_response"  // cross-app response
	MsgMeshEnhance  MessageType = "mesh_enhance"   // suggest enhancement to peer app
)

// Message is the envelope that travels over the bus.
type Message struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	From      string      `json:"from"`       // agent ID or "system"
	To        string      `json:"to"`         // agent ID, "all", or "alphas"
	Channel   string      `json:"channel"`    // topic / room
	Body      string      `json:"body"`       // the payload (markdown/text)
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	ReplyTo   string      `json:"reply_to,omitempty"` // ID of message being replied to
}

// Channel names used by the system.
const (
	ChanSystem  = "system"       // system-level broadcasts
	ChanAgents  = "agents"       // all-agent channel
	ChanAlphas  = "alphas"       // alpha-only coordination
	ChanMesh    = "mesh"         // inter-app mesh traffic
	ChanReview  = "code-review"  // code review requests
)

// Subscription represents an agent's interest in a channel.
type Subscription struct {
	AgentID  string `json:"agent_id"`
	Channel  string `json:"channel"`
}
