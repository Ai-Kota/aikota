package tui

import (
	"strings"
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/charmbracelet/x/ansi"
)

// TestRenderBannerShowsLogoAndConfig pins what the startup splash actually puts
// on screen. The logo art cannot be reviewed by reading the source, so this
// renders it (run with -v) and asserts the parts that are text.
func TestRenderBannerShowsLogoAndConfig(t *testing.T) {
	opts := Options{
		Model:         "test-model",
		ProviderName:  "test-provider",
		Protocol:      "openai",
		Version:       "v9.9.9",
		ThinkingLevel: agentcore.ThinkingMedium,
	}
	got := ansi.Strip(renderBanner(DefaultTheme(), opts, "/tmp/project"))

	for _, want := range []string{
		"aikota",
		"Terminal AI coding assistant",
		"test-model",
		"test-provider",
		"openai",
		"medium",
		"/tmp/project",
		"v9.9.9",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("banner missing %q", want)
		}
	}

	// The logo is braille art on the left of the config panel. Assert on the
	// braille block rather than a character list, so this cannot be misread as
	// pinning particular artwork: the art is expected to change with the name.
	hasBraille := false
	for _, r := range got {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Error("banner has no braille art")
	}

	// Print it so the art can be eyeballed; the assertion above only proves it
	// is present, not that it looks like anything.
	t.Logf("rendered banner:\n%s", got)
}

// TestRenderBannerHandlesEmptyConfig: the banner is drawn before a run is
// resolved, so it must survive an Options with nothing filled in.
func TestRenderBannerHandlesEmptyConfig(t *testing.T) {
	got := ansi.Strip(renderBanner(DefaultTheme(), Options{}, ""))
	if !strings.Contains(got, "aikota") {
		t.Error("banner lost its title with empty options")
	}
	if !strings.Contains(got, "—") {
		t.Error("empty values should render as the placeholder, not blank")
	}
}
