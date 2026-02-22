package adapter

import (
	"fmt"
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// GeminiAdapter handles Google Gemini CLI specifics.
type GeminiAdapter struct{}

func (a *GeminiAdapter) Type() protocol.CLIType {
	return protocol.CLIGemini
}

func (a *GeminiAdapter) Command() (string, []string) {
	return "gemini", []string{}
}

func (a *GeminiAdapter) FormatPrompt(systemPrompt string) string {
	// Gemini CLI accepts prompts via stdin.
	return systemPrompt
}

func (a *GeminiAdapter) FormatMessage(msg *protocol.Message) string {
	return fmt.Sprintf(
		"[FORGE MESSAGE from %s (%s)]\n%s\n[END MESSAGE]",
		msg.From, msg.Type, msg.Body,
	)
}

func (a *GeminiAdapter) ParseOutput(line string) *OutputEvent {
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
