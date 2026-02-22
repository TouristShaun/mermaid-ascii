// Package adapter defines the interface and implementations for each
// AI CLI tool. Adapters handle the specifics of launching, configuring,
// and communicating with Claude Code, OpenAI Codex, and Google Gemini CLIs.
package adapter

import "github.com/AlexanderGrooff/mermaid-ascii/protocol"

// Adapter is the interface that all CLI adapters must implement.
type Adapter interface {
	// Type returns the CLI type this adapter handles.
	Type() protocol.CLIType

	// Command returns the executable name and base arguments.
	Command() (name string, args []string)

	// FormatPrompt wraps the system prompt in whatever format the CLI expects.
	FormatPrompt(systemPrompt string) string

	// FormatMessage wraps a bus message for injection into the CLI's stdin.
	FormatMessage(msg *protocol.Message) string

	// ParseOutput examines a line of output and extracts any structured data
	// (like @mentions to other agents).
	ParseOutput(line string) *OutputEvent
}

// OutputEvent is a structured event parsed from CLI output.
type OutputEvent struct {
	Type    string // "mention", "file_change", "error", "done"
	Target  string // for mentions: the target agent ID
	Body    string // the message or description
}

// Registry holds all available adapters.
var Registry = map[protocol.CLIType]Adapter{}

func init() {
	Registry[protocol.CLIClaude] = &ClaudeAdapter{}
	Registry[protocol.CLICodex] = &CodexAdapter{}
	Registry[protocol.CLIGemini] = &GeminiAdapter{}
}

// Get returns the adapter for the given CLI type.
func Get(cliType protocol.CLIType) Adapter {
	return Registry[cliType]
}
