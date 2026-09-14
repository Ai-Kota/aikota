package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDiscoverUsesProjectConventions: a repository already declares how it is
// built and tested, so discovery reads that rather than requiring config.
func TestDiscoverUsesProjectConventions(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.25\n")

	cmds, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("got %d commands, want build+test: %v", len(cmds), cmds)
	}
	// Build must come first: if the code does not compile, test output is noise.
	if cmds[0].Name != "go build" || cmds[1].Name != "go test" {
		t.Errorf("order = %s, %s; want go build then go test", cmds[0].Name, cmds[1].Name)
	}
}

func TestDiscoverOtherEcosystems(t *testing.T) {
	cases := []struct{ marker, wantBin string }{
		{"Cargo.toml", "cargo"},
		{"package.json", "npm"},
		{"pytest.ini", "pytest"},
	}
	for _, tc := range cases {
		t.Run(tc.marker, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, tc.marker, "")
			cmds, err := Discover(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(cmds) == 0 || cmds[0].Bin != tc.wantBin {
				t.Errorf("Discover(%s) = %v, want %s", tc.marker, cmds, tc.wantBin)
			}
		})
	}
}

// TestDiscoverEmptyProject: no recognisable marker means no checks, which the
// caller must be able to distinguish from "checks that all pass".
func TestDiscoverEmptyProject(t *testing.T) {
	cmds, err := Discover(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 0 {
		t.Errorf("got %v, want none", cmds)
	}
}

// TestOverrideWinsAndCanBeEmpty: an explicit declaration is authoritative in
// both directions — it can add checks a convention would miss, and it can
// declare that there are none.
func TestOverrideWinsAndCanBeEmpty(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.25\n")
	write(t, root, ".aikota/verify.json", `{"commands":[{"name":"lint","bin":"mylint","args":["-strict"]}]}`)

	cmds, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || cmds[0].Bin != "mylint" {
		t.Fatalf("override ignored: %v", cmds)
	}

	write(t, root, ".aikota/verify.json", `{"commands":[]}`)
	cmds, err = Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 0 {
		t.Errorf("an empty override must mean no checks, got %v", cmds)
	}
}

func TestDiscoverRejectsMalformedOverride(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".aikota/verify.json", "{not json")
	if _, err := Discover(root); err == nil {
		t.Fatal("a malformed override should surface, not be silently ignored")
	}
}

// TestRunPassesAndFails covers the two verdicts the model acts on, using real
// processes rather than fakes: the point of this package is that it runs things.
func TestRunPassesAndFails(t *testing.T) {
	root := t.TempDir()

	ok := Run(context.Background(), root, []Command{{Name: "ok", Bin: "go", Args: []string{"version"}}})
	if !ok.Passed {
		t.Errorf("expected pass, got failed=%v output=%s", ok.Failed, ok.Output)
	}
	if !strings.Contains(ok.Output, "$ go version") {
		t.Errorf("output should echo the command, got: %s", ok.Output)
	}

	bad := Run(context.Background(), root, []Command{{Name: "bad", Bin: "go", Args: []string{"definitely-not-a-subcommand"}}})
	if bad.Passed {
		t.Error("expected a failure")
	}
	if bad.Failed == nil || bad.Failed.Name != "bad" {
		t.Errorf("Failed = %+v, want the bad command", bad.Failed)
	}
}

// TestRunStopsAtFirstFailure: once the build breaks, the test output is noise.
func TestRunStopsAtFirstFailure(t *testing.T) {
	root := t.TempDir()
	res := Run(context.Background(), root, []Command{
		{Name: "first", Bin: "go", Args: []string{"definitely-not-a-subcommand"}},
		{Name: "second", Bin: "go", Args: []string{"version"}},
	})
	if len(res.Ran) != 1 || res.Ran[0].Name != "first" {
		t.Errorf("Ran = %v, want only the first command", res.Ran)
	}
}

// TestRunMissingBinaryIsAFailureNotAnError: a missing tool is something the
// model can often fix, so it must reach the model as output.
func TestRunMissingBinaryIsAFailureNotAnError(t *testing.T) {
	res := Run(context.Background(), t.TempDir(), []Command{
		{Name: "ghost", Bin: "definitely-not-a-real-binary-xyz", Args: nil},
	})
	if res.Passed {
		t.Fatal("a missing binary must not count as a pass")
	}
	if !strings.Contains(res.Output, "not installed") {
		t.Errorf("output should explain the missing binary, got: %s", res.Output)
	}
}

// TestRunNoCommandsExplainsItself: silence here would read as success.
func TestRunNoCommandsExplainsItself(t *testing.T) {
	res := Run(context.Background(), t.TempDir(), nil)
	if res.Passed {
		t.Error("no checks is not a pass")
	}
	if res.Reason == "" {
		t.Error("a run that could not happen must say why")
	}
}

func TestTruncateTailKeepsTheEnd(t *testing.T) {
	long := strings.Repeat("a", 100) + "THE-FAILURE"
	got, truncated := truncateTail(long, 32)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if !strings.Contains(got, "THE-FAILURE") {
		t.Errorf("truncation must keep the tail, got: %s", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncation must be marked, got: %s", got)
	}
}
