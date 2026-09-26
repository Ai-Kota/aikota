// This file implements a file tree visualization component for the TUI.
// It renders a collapsible tree of files and directories with keyboard navigation.
package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// TreeNode represents a single entry in the file tree.
type TreeNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*TreeNode
	Expanded bool
	Selected bool
}

// TreeModel is the Bubble Tea model for the file tree panel.
type TreeModel struct {
	Root      *TreeNode
	Width     int
	Height    int
	CursorPos int         // linear index into flat node list
	Nodes     []*TreeNode // flat list for navigation
	theme     Theme
	rootPath  string
}

// NewTreeModel creates a new file tree model rooted at dir.
func NewTreeModel(dir string, theme Theme) *TreeModel {
	m := &TreeModel{
		theme:    theme,
		rootPath: dir,
	}
	m.buildTree(dir)
	m.rebuildFlatList()
	return m
}

// buildTree recursively builds the tree from the filesystem.
func (m *TreeModel) buildTree(dir string) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	m.Root = m.buildNode(dir, info)
}

// buildNode creates a TreeNode for a single filesystem entry.
func (m *TreeModel) buildNode(path string, info os.FileInfo) *TreeNode {
	node := &TreeNode{
		Name:     info.Name(),
		Path:     path,
		IsDir:    info.IsDir(),
		Expanded: true,
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err == nil {
			for _, entry := range entries {
				entryPath := filepath.Join(path, entry.Name())
				entryInfo, err := entry.Info()
				if err != nil {
					continue
				}
				node.Children = append(node.Children, m.buildNode(entryPath, entryInfo))
			}
		}
	}
	return node
}

// rebuildFlatList creates a linear list of visible nodes for navigation.
func (m *TreeModel) rebuildFlatList() {
	m.Nodes = nil
	m.collectVisible(m.Root)
	if len(m.Nodes) > 0 && m.CursorPos >= len(m.Nodes) {
		m.CursorPos = len(m.Nodes) - 1
	}
}

// collectVisible recursively collects visible nodes into the flat list.
func (m *TreeModel) collectVisible(node *TreeNode) {
	m.Nodes = append(m.Nodes, node)
	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			m.collectVisible(child)
		}
	}
}

// View renders the file tree as text.
func (m *TreeModel) View() tea.View {
	if m.Root == nil || len(m.Nodes) == 0 {
		return tea.View{Content: "  (empty directory)"}
	}
	var b strings.Builder
	m.renderNode(m.Root, &b, 0)
	return tea.View{Content: b.String()}
}

// renderNode recursively renders a node and its children.
func (m *TreeModel) renderNode(node *TreeNode, b *strings.Builder, depth int) {
	prefix := strings.Repeat("  ", depth)
	if depth > 0 {
		prefix += "├── "
	}

	cursor := " "
	if m.Nodes[m.CursorPos] == node {
		cursor = ">"
	}

	name := node.Name
	line := prefix + cursor + " " + name
	if node.Selected {
		line = m.theme.Accent.Render(line)
	} else if node.IsDir {
		line = m.theme.ToolHeader.Render(line)
	}
	b.WriteString(line + "\n")

	if node.IsDir && node.Expanded {
		for _, child := range node.Children {
			m.renderNode(child, b, depth+1)
		}
	}
}

// Update handles user input for tree navigation.
func (m *TreeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			m.navigateUp()
		case "down":
			m.navigateDown()
		case "enter":
			m.toggleExpand()
		case "right":
			m.expandAll()
		case "left":
			m.collapseAll()
		case " ":
			m.toggleSelected()
		}
		m.rebuildFlatList()
	}
	return m, nil
}

// Init is required by the tea.Model interface. The tree model has no initialization side effects.
func (m *TreeModel) Init() tea.Cmd {
	return nil
}

// navigateUp moves the cursor up one visible node.
func (m *TreeModel) navigateUp() {
	if m.CursorPos > 0 {
		m.CursorPos--
	}
}

// navigateDown moves the cursor down one visible node.
func (m *TreeModel) navigateDown() {
	if m.CursorPos < len(m.Nodes)-1 {
		m.CursorPos++
	}
}

// toggleExpand expands or collapses the current node.
func (m *TreeModel) toggleExpand() {
	if m.CursorPos >= 0 && m.CursorPos < len(m.Nodes) {
		node := m.Nodes[m.CursorPos]
		if node.IsDir {
			node.Expanded = !node.Expanded
		}
	}
}

// expandAll expands all directories.
func (m *TreeModel) expandAll() {
	m.walkExpand(m.Root, true)
	m.rebuildFlatList()
}

// collapseAll collapses all directories.
func (m *TreeModel) collapseAll() {
	m.walkExpand(m.Root, false)
	m.rebuildFlatList()
}

// walkExpand recursively sets expanded state.
func (m *TreeModel) walkExpand(node *TreeNode, expanded bool) {
	if node.IsDir {
		node.Expanded = expanded
	}
	for _, child := range node.Children {
		m.walkExpand(child, expanded)
	}
}

// toggleSelected marks/unmarks the current node as selected.
func (m *TreeModel) toggleSelected() {
	if m.CursorPos >= 0 && m.CursorPos < len(m.Nodes) {
		node := m.Nodes[m.CursorPos]
		node.Selected = !node.Selected
	}
}

// SelectedPath returns the path of the currently selected node.
func (m *TreeModel) SelectedPath() string {
	if m.CursorPos >= 0 && m.CursorPos < len(m.Nodes) {
		return m.Nodes[m.CursorPos].Path
	}
	return ""
}

// Help returns keyboard shortcuts.
func (m *TreeModel) Help() string {
	return "↑↓ Navigate  Enter Toggle  ←→ Collapse/Expand All  Space Select"
}
