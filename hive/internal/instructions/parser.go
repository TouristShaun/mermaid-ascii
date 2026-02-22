// Package instructions handles parsing and watching instruction files.
// Instructions are YAML files that declare which agents to spawn,
// what tasks they should work on, and coordination rules.
package instructions

import (
	"fmt"
	"os"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
	"gopkg.in/yaml.v3"
)

// LoadFile reads and parses an instruction YAML file.
func LoadFile(path string) (*protocol.InstructionSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes YAML instruction data.
func Parse(data []byte) (*protocol.InstructionSet, error) {
	var instrSet protocol.InstructionSet
	if err := yaml.Unmarshal(data, &instrSet); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Validate required fields.
	if instrSet.Project.Name == "" {
		return nil, fmt.Errorf("project.name is required")
	}
	if len(instrSet.Agents) == 0 {
		return nil, fmt.Errorf("at least one agent spec is required")
	}

	// Set defaults.
	for i := range instrSet.Agents {
		if instrSet.Agents[i].Count == 0 {
			instrSet.Agents[i].Count = 1
		}
	}
	if instrSet.Project.MainBranch == "" {
		instrSet.Project.MainBranch = "main"
	}

	return &instrSet, nil
}
