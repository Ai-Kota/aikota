package interactive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTreeModelBuildsFromDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("world"), 0o644)

	m := NewTreeModel(dir, DefaultTheme())
	if m.Root == nil {
		t.Fatal("expected root node")
	}
	if m.Root.Name != filepath.Base(dir) {
		t.Errorf("root name = %q, want %q", m.Root.Name, filepath.Base(dir))
	}
	if len(m.Nodes) == 0 {
		t.Error("expected at least one node")
	}
}

func TestTreeModelNavigation(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)

	m := NewTreeModel(dir, DefaultTheme())
	startPos := m.CursorPos

	// Navigate down
	model, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "down"})
	m2, ok := model.(*TreeModel)
	if !ok {
		t.Fatal("expected *TreeModel")
	}
	if m2.CursorPos != startPos+1 {
		t.Errorf("cursor = %d, want %d", m2.CursorPos, startPos+1)
	}

	// Navigate up
	model, _ = m2.Update(tea.KeyPressMsg{Code: 'u', Text: "up"})
	m3, ok := model.(*TreeModel)
	if !ok {
		t.Fatal("expected *TreeModel")
	}
	if m3.CursorPos != startPos {
		t.Errorf("cursor = %d, want %d", m3.CursorPos, startPos)
	}
}

func TestTreeModelToggleExpand(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)

	m := NewTreeModel(dir, DefaultTheme())
	if !m.Root.Expanded {
		t.Error("root should be expanded by default")
	}

	// Toggle expand on root
	model, _ := m.Update(tea.KeyPressMsg{Code: 'm', Text: "enter"})
	m2, ok := model.(*TreeModel)
	if !ok {
		t.Fatal("expected *TreeModel")
	}
	if m2.Root.Expanded {
		t.Error("root should be collapsed after toggle")
	}

	// Toggle again
	model, _ = m2.Update(tea.KeyPressMsg{Code: 'm', Text: "enter"})
	m3, ok := model.(*TreeModel)
	if !ok {
		t.Fatal("expected *TreeModel")
	}
	if !m3.Root.Expanded {
		t.Error("root should be expanded after second toggle")
	}
}

func TestTreeModelCollapseExpandAll(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)

	m := NewTreeModel(dir, DefaultTheme())
	if !m.Root.Expanded || len(m.Nodes) < 3 {
		t.Skip("tree structure differs")
	}

	// Collapse all
	model, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "left"})
	m2, ok := model.(*TreeModel)
	if !ok {
		t.Fatal("expected *TreeModel")
	}
	for _, n := range m2.Nodes {
		if n.IsDir && n.Expanded {
			t.Error("expected all dirs collapsed")
		}
	}

	// Expand all
	model, _ = m2.Update(tea.KeyPressMsg{Code: 'r', Text: "right"})
	m3, ok := model.(*TreeModel)
	if !ok {
		t.Fatal("expected *TreeModel")
	}
	for _, n := range m3.Nodes {
		if n.IsDir && !n.Expanded {
			t.Error("expected all dirs expanded")
		}
	}
}

func TestTreeModelSelectedPath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(f, []byte("test"), 0o644)

	m := NewTreeModel(dir, DefaultTheme())
	path := m.SelectedPath()
	if path == "" {
		t.Error("expected non-empty selected path")
	}
}

func TestTreeModelHelp(t *testing.T) {
	m := NewTreeModel(t.TempDir(), DefaultTheme())
	help := m.Help()
	if !strings.Contains(help, "Navigate") {
		t.Error("help should mention navigation")
	}
}
