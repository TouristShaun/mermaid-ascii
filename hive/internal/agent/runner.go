package agent

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// Runner manages a single CLI process (claude, codex, or gemini).
type Runner struct {
	agent     *protocol.Agent
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	prompt    string
	env       map[string]string
	mu        sync.Mutex
	stopped   bool
	onMessage func(from, to, body string) // callback for intercepted @mentions
}

// NewRunner creates a runner for the given agent.
func NewRunner(agent *protocol.Agent, systemPrompt string, env map[string]string) (*Runner, error) {
	return &Runner{
		agent:  agent,
		prompt: systemPrompt,
		env:    env,
	}, nil
}

// Start launches the CLI process in the agent's worktree.
func (r *Runner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cmdName, args := r.cliCommand()
	r.cmd = exec.Command(cmdName, args...)
	r.cmd.Dir = r.agent.Worktree

	// Set environment variables.
	r.cmd.Env = os.Environ()
	for k, v := range r.env {
		r.cmd.Env = append(r.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var err error
	r.stdin, err = r.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	r.stdout, err = r.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	r.cmd.Stderr = os.Stderr

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmdName, err)
	}

	// Monitor stdout for @mentions and output.
	go r.monitorOutput()

	// Send the initial system prompt.
	r.sendInput(r.prompt)

	return nil
}

// Stop gracefully terminates the CLI process.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopped {
		return
	}
	r.stopped = true

	// Try to send exit command first.
	r.sendInput("/exit")

	if r.stdin != nil {
		r.stdin.Close()
	}
	if r.cmd != nil && r.cmd.Process != nil {
		r.cmd.Process.Signal(os.Interrupt)
		r.cmd.Wait()
	}
}

// SendMessage sends a message to the CLI process via stdin.
func (r *Runner) SendMessage(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sendInput(msg)
}

// UpdatePrompt sends updated instructions to the running agent.
func (r *Runner) UpdatePrompt(newPrompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.prompt = newPrompt
	notice := fmt.Sprintf(`
=== INSTRUCTION UPDATE ===
Your instructions have been updated. Please finish your current step gracefully,
then follow the new instructions below:

%s

=== END UPDATE ===
`, newPrompt)
	r.sendInput(notice)
}

// OnMessage sets the callback for intercepted @mention messages.
func (r *Runner) OnMessage(fn func(from, to, body string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onMessage = fn
}

func (r *Runner) sendInput(text string) {
	if r.stdin != nil {
		io.WriteString(r.stdin, text+"\n")
	}
}

func (r *Runner) monitorOutput() {
	scanner := bufio.NewScanner(r.stdout)
	for scanner.Scan() {
		line := scanner.Text()

		// Intercept @mentions for bus routing.
		if strings.HasPrefix(line, "@") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				target := strings.TrimPrefix(parts[0], "@")
				if r.onMessage != nil {
					r.onMessage(r.agent.ID, target, parts[1])
					continue
				}
			}
		}

		// Log the output.
		log.Printf("[%s] %s", r.agent.Name, line)
	}
}

// cliCommand returns the command name and arguments for the agent's CLI type.
func (r *Runner) cliCommand() (string, []string) {
	switch r.agent.CLIType {
	case protocol.CLIClaude:
		return "claude", []string{
			"--print",
			"--dangerously-skip-permissions",
		}
	case protocol.CLICodex:
		return "codex", []string{
			"--full-auto",
		}
	case protocol.CLIGemini:
		return "gemini", []string{}
	default:
		return "echo", []string{"unsupported CLI type: " + string(r.agent.CLIType)}
	}
}
