package adapter

import (
	"fmt"
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// ClaudeAdapter handles Claude Code CLI specifics.
type ClaudeAdapter struct{}

func (a *ClaudeAdapter) Type() protocol.CLIType {
	return protocol.CLIClaude
}

func (a *ClaudeAdapter) Command() (string, []string) {
	return "claude", []string{
		"--print",                          // non-interactive output
		"--dangerously-skip-permissions",   // auto-approve tool use
	}
}

func (a *ClaudeAdapter) FormatPrompt(systemPrompt string) string {
	// Claude Code accepts the prompt directly via stdin or --prompt flag.
	return systemPrompt
}

func (a *ClaudeAdapter) FormatMessage(msg *protocol.Message) string {
	return fmt.Sprintf(
		"[FORGE MESSAGE from %s (%s)]\n%s\n[END MESSAGE]",
		msg.From, msg.Type, msg.Body,
	)
}

func (a *ClaudeAdapter) ParseOutput(line string) *OutputEvent {
	// Detect @mentions in Claude's output.
	if strings.HasPrefix(line, "@") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			return &OutputEvent{
				Type:   "mention",
				Target: strings.TrimPrefix(parts[0], "@"),
				Body:   parts[1],
			}
		}
	}
	return nil
}
