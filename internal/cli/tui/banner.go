package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Ai-Kota/aikota/internal/provider"
	"github.com/Ai-Kota/aikota/internal/selfupdate"
)

// This file builds the startup splash shown at the top of the transcript: the
// aikota braille logo painted in a vertical rainbow gradient, with the session's
// basic configuration (model, provider, protocol, thinking effort, directory)
// laid out beside it. It is seeded once by withSession so it scrolls up as the
// conversation grows, like a shell's login banner.

// logoLines is the aikota braille-art logo, one string per row. Each braille
// cell is a 2x4 dot grid, so this spells "aikota" across 36x8 dots: a five-dot
// wide lowercase face on a shared x-height, one dot of letter spacing.
//
// It replaces the wordmark art this project inherited from its upstream, which
// was drawn for the previous name and could not be relabelled by substitution —
// the artwork had to be redrawn. Regenerate from the same face if the name ever
// changes again.
var logoLines = []string{
	"⠠⠤⡀⠠⡅ ⡇⡠ ⡠⠤⡀⢴⠤ ⠠⠤⡀",
	"⠪⠭⠇⠠⠧ ⠏⠢ ⠣⠤⠃⠘⠤⠂⠪⠭⠇",
}

// logoColors is the top-to-bottom gradient painted across the logo rows
// (ANSI 256-color cube): amber into cyan.
var logoColors = []string{
	"214", "45",
}
// renderBanner paints the logo gradient and joins it with a config panel showing
// the session basics. Its only I/O is a single cheap read of the local
// update-check cache (no network — CachedLatest); it never panics, so it is safe
// to build eagerly at startup.
func renderBanner(theme Theme, opts Options, cwd string) string {
	var logo strings.Builder
	for i, line := range logoLines {
		if i > 0 {
			logo.WriteByte('\n')
		}
		c := logoColors[i%len(logoColors)]
		logo.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(line))
	}

	title := lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Bold(true)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGray))
	value := lipgloss.NewStyle().Foreground(lipgloss.Color(colorUser)).Bold(true)

	rows := [][2]string{
		{"Version", firstNonEmpty(opts.Version, "dev")},
		{"Model", firstNonEmpty(opts.Model, "—")},
		{"Provider", firstNonEmpty(opts.ProviderName, "—")},
		{"Protocol", firstNonEmpty(provider.ProtocolLabel(opts.Protocol), "—")},
		{"Thinking", firstNonEmpty(string(opts.ThinkingLevel), "off")},
		{"Directory", firstNonEmpty(cwd, "—")},
	}

	// When the cached latest-release check says a newer version exists, append a
	// highlighted "→ vX.Y.Z" and an upgrade hint to the Version row. The check is
	// read from the local cache only (no network here); a background refresh keeps
	// it current for the next launch. dev/unparseable versions never trigger this.
	upgradeHint := ""
	if latest, _ := selfupdate.CachedLatest(); latest != "" {
		if avail, comparable := selfupdate.UpdateAvailable(opts.Version, latest); comparable && avail {
			newVer := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(latest)
			rows[0][1] = rows[0][1] + "  →  " + newVer
			upgradeHint = label.Render(strings.Repeat(" ", 11)) +
				lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("Run aikota update to upgrade")
		}
	}

	var info strings.Builder
	info.WriteString(title.Render("aikota") + "  " + theme.System.Render("Terminal AI coding assistant") + "\n\n")
	for i, r := range rows {
		if i > 0 {
			info.WriteByte('\n')
		}
		info.WriteString(label.Render(fmt.Sprintf("%-10s ", r[0])) + value.Render(r[1]))
	}
	if upgradeHint != "" {
		info.WriteString("\n" + upgradeHint)
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, logo.String(), "   ", info.String())
}

// firstNonEmpty returns s when it is non-empty, otherwise the fallback.
func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
