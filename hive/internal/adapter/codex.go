package adapter

import (
	"fmt"
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// CodexAdapter handles OpenAI Codex CLI specifics.
type CodexAdapter struct{}

func (a *CodexAdapter) Type() protocol.CLIType {
	return protocol.CLICodex
}

func (a *CodexAdapter) Command() (string, []string) {
	return "codex", []string{
		"--full-auto", // non-interactive mode
	}
}

func (a *CodexAdapter) FormatPrompt(systemPrompt string) string {
	// Codex CLI accepts prompts via stdin.
	return systemPrompt
}

func (a *CodexAdapter) FormatMessage(msg *protocol.Message) string {
	return fmt.Sprintf(
		"[FORGE MESSAGE from %s (%s)]\n%s\n[END MESSAGE]",
		msg.From, msg.Type, msg.Body,
	)
}

func (a *CodexAdapter) ParseOutput(line string) *OutputEvent {
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
