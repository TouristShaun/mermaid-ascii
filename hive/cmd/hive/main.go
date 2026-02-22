// hive is the local orchestrator that runs on your MacBook Air. It manages
// AI CLI instances (Claude Code, OpenAI Codex, Google Gemini), creates git
// worktrees for each, and bridges them to the Forge server for communication.
//
// Usage:
//   hive start -f instructions.yaml    # spin up agents per instruction file
//   hive status                         # show all running agents
//   hive instruct -f instructions.yaml  # hot-reload instructions
//   hive talk <agent-id> "message"      # send a message to an agent
//   hive stop                           # gracefully stop all agents
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AlexanderGrooff/mermaid-ascii/hive/internal/agent"
	"github.com/AlexanderGrooff/mermaid-ascii/hive/internal/bridge"
	"github.com/AlexanderGrooff/mermaid-ascii/hive/internal/instructions"
	"github.com/AlexanderGrooff/mermaid-ascii/hive/internal/worktree"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "start":
		cmdStart(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "instruct":
		cmdInstruct(os.Args[2:])
	case "talk":
		cmdTalk(os.Args[2:])
	case "stop":
		cmdStop(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `hive — AI agent orchestrator for Forge

Commands:
  start   -f <instructions.yaml> [-forge <url>] [-repo <path>]
          Spin up agents per the instruction file

  status  [-forge <url>]
          Show all running agents and their worktrees

  instruct -f <instructions.yaml> [-forge <url>] [-grace <seconds>]
          Hot-reload instructions (agents finish current step first)

  talk    <agent-id> "message" [-forge <url>]
          Send a message to a specific agent

  stop    [-forge <url>]
          Gracefully stop all agents
`)
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	instrFile := fs.String("f", "instructions.yaml", "path to instruction file")
	forgeURL := fs.String("forge", "http://localhost:8420", "forge server URL")
	repoPath := fs.String("repo", ".", "path to the local git repo")
	fs.Parse(args)

	// Parse instructions.
	instrSet, err := instructions.LoadFile(*instrFile)
	if err != nil {
		log.Fatalf("load instructions: %v", err)
	}

	// Connect to forge.
	client := bridge.NewClient(*forgeURL)
	if err := client.HealthCheck(); err != nil {
		log.Fatalf("cannot reach forge at %s: %v", *forgeURL, err)
	}
	log.Printf("connected to forge at %s", *forgeURL)

	// Set up worktree manager.
	wtMgr := worktree.NewManager(*repoPath)

	// Create agent manager.
	mgr := agent.NewManager(client, wtMgr, instrSet)

	// Spawn agents per instruction spec.
	if err := mgr.SpawnAll(); err != nil {
		log.Fatalf("spawn agents: %v", err)
	}

	log.Printf("all agents started. press Ctrl+C to stop.")

	// Wait for shutdown signal.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("shutting down agents...")
	mgr.StopAll()
	log.Println("done")
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	forgeURL := fs.String("forge", "http://localhost:8420", "forge server URL")
	fs.Parse(args)

	client := bridge.NewClient(*forgeURL)
	agents, err := client.ListAgents()
	if err != nil {
		log.Fatalf("list agents: %v", err)
	}

	out, _ := json.MarshalIndent(agents, "", "  ")
	fmt.Println(string(out))
}

func cmdInstruct(args []string) {
	fs := flag.NewFlagSet("instruct", flag.ExitOnError)
	instrFile := fs.String("f", "instructions.yaml", "path to instruction file")
	forgeURL := fs.String("forge", "http://localhost:8420", "forge server URL")
	grace := fs.Int("grace", 30, "grace period in seconds")
	fs.Parse(args)

	instrSet, err := instructions.LoadFile(*instrFile)
	if err != nil {
		log.Fatalf("load instructions: %v", err)
	}

	client := bridge.NewClient(*forgeURL)
	if err := client.UpdateInstructions(instrSet, *grace); err != nil {
		log.Fatalf("update instructions: %v", err)
	}
	log.Printf("instructions v%d broadcast with %ds grace period", instrSet.Version, *grace)
}

func cmdTalk(args []string) {
	fs := flag.NewFlagSet("talk", flag.ExitOnError)
	forgeURL := fs.String("forge", "http://localhost:8420", "forge server URL")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 2 {
		log.Fatal("usage: hive talk <agent-id> \"message\"")
	}

	client := bridge.NewClient(*forgeURL)
	if err := client.SendMessage(remaining[0], remaining[1]); err != nil {
		log.Fatalf("send message: %v", err)
	}
	log.Printf("message sent to %s", remaining[0])
}

func cmdStop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	forgeURL := fs.String("forge", "http://localhost:8420", "forge server URL")
	fs.Parse(args)

	client := bridge.NewClient(*forgeURL)
	if err := client.BroadcastStop(); err != nil {
		log.Fatalf("broadcast stop: %v", err)
	}
	log.Println("stop signal broadcast to all agents")
}
