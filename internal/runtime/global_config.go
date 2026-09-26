// Package runtime provides the agent execution loop.
// This file centralizes timeout configuration to avoid scattered constants.
package runtime

import "time"

// GlobalTimeoutConfig centralizes all timeout values used across the runtime.
// Having a single source of truth makes it easier to tune and test timeouts.
type GlobalTimeoutConfig struct {
	// BashDefaultTimeout is the default timeout for shell commands.
	BashDefaultTimeout time.Duration
	// BashMaxTimeout caps any user-requested timeout.
	BashMaxTimeout time.Duration
	// StreamTimeout bounds the HTTP stream to the LLM provider.
	StreamTimeout time.Duration
	// PluginInitTimeout bounds the plugin initialize handshake.
	PluginInitTimeout time.Duration
	// HookTimeout bounds per-hook execution.
	HookTimeout time.Duration
	// ContextWindow is the maximum context size in tokens (0 = use model default).
	ContextWindow int
}

// DefaultGlobalTimeoutConfig returns sensible defaults matching current behavior.
func DefaultGlobalTimeoutConfig() *GlobalTimeoutConfig {
	return &GlobalTimeoutConfig{
		BashDefaultTimeout: 2 * time.Minute,
		BashMaxTimeout:     10 * time.Minute,
		StreamTimeout:      5 * time.Minute,
		PluginInitTimeout:  10 * time.Second,
		HookTimeout:        60 * time.Second,
		ContextWindow:      0,
	}
}

// TimeoutConfig exposes the shared configuration to callers.
var TimeoutConfig = DefaultGlobalTimeoutConfig()
