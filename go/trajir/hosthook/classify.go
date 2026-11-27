package hosthook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
)

// hostReadOnly lists each host's built-in tools that only read. Everything
// else, shell and file writes included, stays NON_IDEMPOTENT_WRITE. The
// class only changes what is gated on replay. It never blocks a first run.
var hostReadOnly = map[string][]string{
	// https://code.claude.com/docs/en/settings#tools-available-to-claude
	"claude": {"Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch", "NotebookRead"},
	// https://geminicli.com/docs/tools/
	"gemini": {"read_file", "read_many_files", "list_directory", "glob", "search_file_content", "google_web_search", "web_fetch"},
	// https://cursor.com/docs/agent/hooks (preToolUse matcher values)
	"cursor": {"Read", "Grep"},
	// Grok CLI built-ins that only read. run_terminal_command is open-world
	// and is not listed. Matcher aliases such as Read are not the payload name.
	"grok": {"read_file", "hashline_read", "grep", "hashline_grep", "list_dir", "web_search", "web_fetch"},
	// Codex and Copilot CLI tools are shell and patch based. No defaults.
}

// hostAgentSpawn lists built-ins that start another agent. effects.json may
// still name a different class. These names are not open-world primitives.
var hostAgentSpawn = map[string][]string{
	"grok": {"spawn_subagent"},
}

type effectsFile struct {
	Tools map[string]string `json:"tools"`
}

// NewClassifier returns the effect classifier for one host and project.
// Order: <project>/.trajir/effects.json, then the host defaults, then
// effects.ClassifyTool (MCP annotations, fail closed). A bad effects.json is
// an error, and so is an entry that makes an open-world tool such as Bash safer.
func NewClassifier(host, projectDir string) (func(string, map[string]any) effects.EffectClass, error) {
	table := map[string]effects.EffectClass{}
	for _, name := range hostReadOnly[host] {
		table[name] = effects.READ_ONLY
	}
	for _, name := range hostAgentSpawn[host] {
		table[name] = effects.AGENT_SPAWN
	}
	path := filepath.Join(projectDir, ".trajir", "effects.json")
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		var f effectsFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for name, class := range f.Tools {
			c := effects.EffectClass(class)
			if !knownClass(c) {
				return nil, fmt.Errorf("%s: tool %q has unknown effect class %q", path, name, class)
			}
			if err := effects.AssertOpenWorldEffect(name, c, false); err != nil {
				return nil, fmt.Errorf("%s: %s is an open-world tool and stays NON_IDEMPOTENT_WRITE", path, name)
			}
			table[name] = c
		}
	}
	return func(name string, annotations map[string]any) effects.EffectClass {
		if c, ok := table[name]; ok {
			return c
		}
		return effects.ClassifyTool(name, annotations)
	}, nil
}

func knownClass(c effects.EffectClass) bool {
	switch c {
	case effects.PURE, effects.READ_ONLY, effects.IDEMPOTENT_WRITE,
		effects.NON_IDEMPOTENT_WRITE, effects.AGENT_SPAWN, effects.SENSITIVE:
		return true
	}
	return false
}
