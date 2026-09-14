package pkgmgr

import (
	"path/filepath"
	"testing"
)

// TestHomeHonorsAIKOTAHOME verifies Home prefers AIKOTA_HOME over the default.
func TestHomeHonorsAIKOTAHOME(t *testing.T) {
	t.Setenv("AIKOTA_HOME", "/custom/aikota")
	if got := Home(); got != "/custom/aikota" {
		t.Errorf("Home() = %q, want /custom/aikota", got)
	}
}

// TestTypeDirsUnderHome verifies the plugins/commands/themes dirs nest under
// $AIKOTA_HOME.
func TestTypeDirsUnderHome(t *testing.T) {
	t.Setenv("AIKOTA_HOME", "/custom/aikota")
	// Expectations go through filepath.Join so they match the joined form the
	// package returns on a platform whose separator is not '/'.
	root := filepath.FromSlash("/custom/aikota")
	cases := map[PackageType]string{
		TypeExtension: filepath.Join(root, "plugins"),
		TypePrompt:    filepath.Join(root, "commands"),
		TypeTheme:     filepath.Join(root, "themes"),
	}
	for typ, want := range cases {
		if got := DirForType(typ); got != want {
			t.Errorf("DirForType(%s) = %q, want %q", typ, got, want)
		}
	}
}

// TestSkillsDirHonorsOverride verifies skills use AIKOTA_SKILLS_DIR, not $AIKOTA_HOME.
func TestSkillsDirHonorsOverride(t *testing.T) {
	t.Setenv("AIKOTA_SKILLS_DIR", "/custom/skills")
	if got := SkillsDir(); got != "/custom/skills" {
		t.Errorf("SkillsDir() = %q, want /custom/skills", got)
	}
	if got := DirForType(TypeSkill); got != "/custom/skills" {
		t.Errorf("DirForType(skill) = %q, want /custom/skills", got)
	}
}

// TestSkillsDirDefault verifies skills default to ~/.agents/skills.
func TestSkillsDirDefault(t *testing.T) {
	t.Setenv("AIKOTA_SKILLS_DIR", "")
	t.Setenv("HOME", "/home/tester")
	// os.UserHomeDir reads USERPROFILE on Windows and HOME everywhere else, so
	// set both for the fixture home to apply on either platform.
	t.Setenv("USERPROFILE", filepath.FromSlash("/home/tester"))
	want := filepath.Join(filepath.FromSlash("/home/tester"), ".agents", "skills")
	if got := SkillsDir(); got != want {
		t.Errorf("SkillsDir() = %q, want %q", got, want)
	}
}

// TestDirForUnknownType verifies an unknown type yields "".
func TestDirForUnknownType(t *testing.T) {
	if got := DirForType(PackageType("bogus")); got != "" {
		t.Errorf("DirForType(bogus) = %q, want empty", got)
	}
}
