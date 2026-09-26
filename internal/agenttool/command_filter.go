// Package agenttool provides built-in agent tools.
// This file implements command filtering (allowlist/denylist) for bash tool calls.
package agenttool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CommandFilter enforces allowlist/denylist rules on bash commands.
type CommandFilter struct {
	allowed []string
	denied  []string
	patterns []*regexp.Regexp
}

// CommandFilterConfig holds the configuration for command filtering.
type CommandFilterConfig struct {
	AllowedCommands []string
	DeniedCommands  []string
	DenyPatterns    []string
}

// NewCommandFilter creates a command filter from the given config.
func NewCommandFilter(cfg *CommandFilterConfig) (*CommandFilter, error) {
	f := &CommandFilter{}
	if cfg == nil {
		return f, nil
	}
	f.allowed = cfg.AllowedCommands
	f.denied = cfg.DeniedCommands
	for _, pattern := range cfg.DenyPatterns {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("command filter: compile pattern %q: %w", pattern, err)
		}
		f.patterns = append(f.patterns, re)
	}
	return f, nil
}

// AllowCommand checks if a command string is allowed by the filter.
func (f *CommandFilter) AllowCommand(cmd string) (bool, string) {
	if cmd == "" {
		return true, ""
	}
	for _, re := range f.patterns {
		if re.MatchString(cmd) {
			return false, fmt.Sprintf("command denied by pattern: %s", cmd)
		}
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return true, ""
	}
	cmdName := parts[0]
	base := filepath.Base(cmdName)
	for _, deny := range f.denied {
		if strings.EqualFold(base, deny) || strings.EqualFold(cmdName, deny) {
			return false, fmt.Sprintf("command denied: %s", cmdName)
		}
	}
	if len(f.allowed) > 0 {
		found := false
		for _, allow := range f.allowed {
			if strings.EqualFold(base, allow) || strings.EqualFold(cmdName, allow) {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("command not in allowlist: %s", cmdName)
		}
	}
	return true, ""
}

// LoadCommandFilterFromEnv reads filter config from environment variables.
func LoadCommandFilterFromEnv() (*CommandFilter, error) {
	allowed := os.Getenv("AIKOTA_ALLOW_COMMANDS")
	denied := os.Getenv("AIKOTA_DENY_COMMANDS")
	patterns := os.Getenv("AIKOTA_DENY_PATTERNS")
	cfg := &CommandFilterConfig{}
	if allowed != "" {
		cfg.AllowedCommands = strings.Split(allowed, ",")
		for i := range cfg.AllowedCommands {
			cfg.AllowedCommands[i] = strings.TrimSpace(cfg.AllowedCommands[i])
		}
	}
	if denied != "" {
		cfg.DeniedCommands = strings.Split(denied, ",")
		for i := range cfg.DeniedCommands {
			cfg.DeniedCommands[i] = strings.TrimSpace(cfg.DeniedCommands[i])
		}
	}
	if patterns != "" {
		cfg.DenyPatterns = strings.Split(patterns, ",")
		for i := range cfg.DenyPatterns {
			cfg.DenyPatterns[i] = strings.TrimSpace(cfg.DenyPatterns[i])
		}
	}
	return NewCommandFilter(cfg)
}
