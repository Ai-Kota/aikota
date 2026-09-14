// Package verify runs a project's own checks — build, test, lint — and reports
// the outcome, so an agent can learn whether its changes actually work instead
// of asserting success from its own reading of the diff.
//
// The commands are discovered from the project's own files rather than
// configured per-project: a repository already declares how it is built and
// tested, and re-declaring that in aikota's config would be a second source of
// truth to keep in sync. A project that needs something different can say so in
// .aikota/verify.json.
//
// Everything here is language-agnostic: discovery is a set of marker-file
// rules, and execution is spawning a binary. Nothing assumes Go.
package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Command is one project check: a program and its arguments, run in the
// project root. There is deliberately no shell: a check must not be able to
// depend on shell state, and quoting is the caller's problem to avoid.
type Command struct {
	Name string   `json:"name"` // human label, e.g. "go test"
	Bin  string   `json:"bin"`  // e.g. "go"
	Args []string `json:"args"` // e.g. ["./..."]
}

// String renders the command as the user would type it.
func (c Command) String() string {
	if len(c.Args) == 0 {
		return c.Bin
	}
	out := c.Bin
	for _, a := range c.Args {
		out += " " + a
	}
	return out
}

// Result is the outcome of a verification run.
type Result struct {
	// Passed is true only when every command succeeded.
	Passed bool
	// Ran lists the commands that were executed, in order. A fail-fast run
	// stops at the first failure, so this can be shorter than the discovered set.
	Ran []Command
	// Failed is the command that failed, if any.
	Failed *Command
	// Output is the combined output of the run, tail-truncated to a budget.
	Output string
	// Truncated reports whether Output was cut.
	Truncated bool
	// Duration is the wall-clock time of the whole run.
	Duration time.Duration
	// Reason explains a run that could not be performed at all (no commands
	// discovered, the binary is missing). Empty for a real pass or fail.
	Reason string
}

const (
	// DefaultTimeout bounds one command. A hanging check must not hang the
	// agent, which is the same reason bash has a timeout.
	DefaultTimeout = 5 * time.Minute

	// outputBudget caps what is handed back to the model. Test output is
	// frequently enormous, and the tail is what carries the failures.
	outputBudget = 32 * 1024

	// overrideFile lets a project state its checks explicitly.
	overrideFile = ".aikota/verify.json"
)

// override is the shape of .aikota/verify.json.
type override struct {
	Commands []Command `json:"commands"`
}

// Discover returns the check commands for the project rooted at root, or an
// empty slice when the project declares no recognisable checks.
//
// An explicit .aikota/verify.json wins over every convention: a project that
// spells out its checks means it.
func Discover(root string) ([]Command, error) {
	if cmds, ok, err := loadOverride(root); err != nil {
		return nil, err
	} else if ok {
		return cmds, nil
	}

	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, rel))
		return err == nil
	}

	var cmds []Command
	switch {
	case exists("go.mod"):
		// Build before test: if the code does not compile, the test output is
		// noise, and fail-fast means test never runs.
		cmds = append(cmds,
			Command{Name: "go build", Bin: "go", Args: []string{"build", "./..."}},
			Command{Name: "go test", Bin: "go", Args: []string{"test", "./..."}},
		)
	case exists("Cargo.toml"):
		cmds = append(cmds, Command{Name: "cargo test", Bin: "cargo", Args: []string{"test"}})
	case exists("package.json"):
		cmds = append(cmds, Command{Name: "npm test", Bin: "npm", Args: []string{"test", "--silent"}})
	case exists("pyproject.toml"), exists("pytest.ini"), exists("setup.py"):
		cmds = append(cmds, Command{Name: "pytest", Bin: "pytest", Args: []string{"-q"}})
	}
	return cmds, nil
}

func loadOverride(root string) ([]Command, bool, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(overrideFile)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var o override
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, false, fmt.Errorf("verify: %s is not valid JSON: %w", overrideFile, err)
	}
	// A present-but-empty override is a deliberate "this project has no checks",
	// which must not silently fall back to a convention.
	return o.Commands, true, nil
}

// Run executes the commands in order, stopping at the first failure.
//
// Commands that cannot be started at all (the binary is not on PATH) are
// reported as a failure of that command rather than a Go error: the model can
// usually fix a missing tool, and an error return would hide the reason.
func Run(ctx context.Context, root string, cmds []Command) Result {
	started := time.Now()
	if len(cmds) == 0 {
		return Result{
			Passed:   false,
			Reason:   "no checks discovered for this project (looked for go.mod, Cargo.toml, package.json, pyproject.toml, and " + overrideFile + ")",
			Duration: time.Since(started),
		}
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout*time.Duration(len(cmds)))
	defer cancel()

	var buf bytes.Buffer
	res := Result{Ran: make([]Command, 0, len(cmds))}

	for i := range cmds {
		c := cmds[i]
		fmt.Fprintf(&buf, "$ %s\n", c.String())

		out, err := runOne(ctx, root, c)
		buf.Write(out)
		// A command that could not be started at all produces no output, so the
		// terminator check has to tolerate an empty slice.
		if len(out) > 0 && out[len(out)-1] != '\n' {
			buf.WriteByte('\n')
		}
		res.Ran = append(res.Ran, c)

		if err != nil {
			res.Failed = &c
			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(&buf, "(%s is not installed or not on PATH)\n", c.Bin)
			} else if ctx.Err() != nil {
				fmt.Fprintf(&buf, "(timed out after %s)\n", DefaultTimeout)
			}
			break
		}
	}

	res.Passed = res.Failed == nil
	res.Duration = time.Since(started)
	res.Output, res.Truncated = truncateTail(buf.String(), outputBudget)
	return res
}

func runOne(ctx context.Context, root string, c Command) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.Bin, c.Args...)
	cmd.Dir = root
	return cmd.CombinedOutput()
}

// truncateTail keeps the end of s when it exceeds budget, because a failing
// check states its reason at the end. The cut is marked so the model knows the
// output it received is partial.
func truncateTail(s string, budget int) (string, bool) {
	if len(s) <= budget {
		return s, false
	}
	keep := s[len(s)-budget:]
	if nl := bytes.IndexByte([]byte(keep), '\n'); nl >= 0 {
		keep = keep[nl+1:]
	}
	return fmt.Sprintf("... [output truncated, showing the last %d bytes]\n%s", budget, keep), true
}
