package version

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInfoFormat(t *testing.T) {
	// Pins the exact string a user or script sees; build the expectation from
	// the vars so the test does not rot when a default changes.
	want := "ctxpack " + Version + " (" + runtime.GOOS + "/" + runtime.GOARCH +
		", " + runtime.Version() + ", commit " + BuildCommit + ", built " + BuildDate + ")"
	if got := Info(); got != want {
		t.Errorf("Info():\n got: %q\nwant: %q", got, want)
	}
}

func TestInfoShape(t *testing.T) {
	pattern := `^ctxpack \S+ \(\S+/\S+, go\S+, commit \S+, built \S+\)$`
	if !regexp.MustCompile(pattern).MatchString(Info()) {
		t.Errorf("Info() = %q does not match %s", Info(), pattern)
	}
}

func TestInfoDeterministic(t *testing.T) {
	if a, b := Info(), Info(); a != b {
		t.Errorf("Info() is not deterministic: %q vs %q", a, b)
	}
}

func TestInfoUsesOverrides(t *testing.T) {
	// These vars exist to be overwritten by -ldflags, so prove the function
	// actually reads them rather than a cached string.
	ov, oc, od := Version, BuildCommit, BuildDate
	defer func() { Version, BuildCommit, BuildDate = ov, oc, od }()
	Version, BuildCommit, BuildDate = "9.9.9", "abc1234", "2026-01-02"
	got := Info()
	for _, want := range []string{"ctxpack 9.9.9", "commit abc1234", "built 2026-01-02"} {
		if !strings.Contains(got, want) {
			t.Errorf("Info() = %q, missing %q", got, want)
		}
	}
}

func TestUserAgent(t *testing.T) {
	ov := Version
	defer func() { Version = ov }()
	Version = "1.2.3"
	if got := UserAgent(); got != "ctxpack/1.2.3" {
		t.Errorf("UserAgent() = %q, want ctxpack/1.2.3", got)
	}
}

func TestVersionLooksLikeSemver(t *testing.T) {
	if !regexp.MustCompile(`^v?\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Errorf("Version %q is not a plausible semantic version", Version)
	}
}

func TestModuleMatchesGomod(t *testing.T) {
	// If the module is ever renamed, this const goes stale silently. Reading
	// go.mod makes that a build-time failure instead.
	b, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Skipf("go.mod not reachable: %v", err)
	}
	var mod string
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			mod = f[1]
			break
		}
	}
	if mod == "" {
		t.Fatal("no module line found in go.mod")
	}
	if mod != Module {
		t.Errorf("go.mod declares %q but version.Module is %q", mod, Module)
	}
}

func TestBuildIdentityDefaultsAreSentinels(t *testing.T) {
	// A dev build must not claim to be a release. These are the values a
	// source checkout gets when nothing was passed to -ldflags.
	if BuildCommit != "dev" {
		t.Errorf("BuildCommit = %q, want the dev sentinel", BuildCommit)
	}
	if BuildDate != "unknown" {
		t.Errorf("BuildDate = %q, want the unknown sentinel", BuildDate)
	}
	if Info() == "" || !strings.Contains(Info(), "commit dev") {
		t.Errorf("Info() = %q, want it to carry the dev commit", Info())
	}
}
