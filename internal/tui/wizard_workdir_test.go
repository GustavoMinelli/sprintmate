package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/GustavoMinelli/sprintmate/internal/config"
)

func workdirWizard() wizard {
	w := newWizard(config.Default(), false)
	w.step = stepWorkdir
	return w
}

func TestWorkdirStepRejectsBadPath(t *testing.T) {
	w := workdirWizard()

	// empty -> error
	w.workdir.SetValue("")
	w, _ = w.Update(keyPress("enter"))
	if w.err == "" {
		t.Fatal("empty folder should error")
	}

	// nonexistent -> error
	w.workdir.SetValue("/definitely/not/here/xyz")
	w, _ = w.Update(keyPress("enter"))
	if w.err == "" {
		t.Fatal("missing dir should error")
	}
}

func TestWorkdirStepDrillsAndConfirms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, "projetos", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := workdirWizard()
	w.workdir.SetValue("~/proj")

	// Enter drills into the highlighted folder, appending a separator.
	w, _ = w.Update(keyPress("enter"))
	if got, want := w.workdir.Value(), filepath.Join(home, "projetos")+"/"; got != want {
		t.Fatalf("enter should drill into %q, got %q", want, got)
	}

	// At a valid folder, Enter finishes the wizard and stores the clean abs path.
	_, cmd := w.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter on a valid folder should finish (return a command)")
	}
	done, ok := cmd().(wizardDoneMsg)
	if !ok {
		t.Fatalf("expected wizardDoneMsg, got %#v", cmd())
	}
	if done.cfg.Workdir != filepath.Join(home, "projetos") {
		t.Errorf("workdir = %q, want %q", done.cfg.Workdir, filepath.Join(home, "projetos"))
	}
}

func TestFinishWritesWorkdir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	w := workdirWizard()
	w.inputs[0].SetValue("https://empresa.atlassian.net")
	w.inputs[1].SetValue("voce@empresa.com")
	w.inputs[2].SetValue("tok")
	w.workdir.SetValue(dir)

	_, cmd := w.finish()
	done := cmd().(wizardDoneMsg)
	if done.cfg.Workdir != filepath.Clean(dir) {
		t.Errorf("workdir = %q, want %q", done.cfg.Workdir, filepath.Clean(dir))
	}
}

func TestResolveDirAnchorsRelativeToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := resolveDir("projetos/demo"); got != filepath.Join(home, "projetos/demo") {
		t.Errorf("relative path = %q, want under home", got)
	}
	if got := resolveDir("/abs/path"); got != "/abs/path" {
		t.Errorf("absolute path should pass through, got %q", got)
	}
}

func TestFilledViewFitsTerminal(t *testing.T) {
	w := workdirWizard()
	w.width, w.height = 100, 30
	out := w.View(mascot{})
	if h := lipgloss.Height(out); h > 30 {
		t.Errorf("rendered height %d exceeds terminal 30", h)
	}
	if width := lipgloss.Width(out); width > 100 {
		t.Errorf("rendered width %d exceeds terminal 100", width)
	}
	if !strings.Contains(out, "Buscar pasta") {
		t.Error("filled workdir view should include the search panel")
	}
}

func TestDirCompletion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, d := range []string{"projetos", "projetos-old", "outros"} {
		if err := os.Mkdir(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// A partial leaf filters the parent's subdirs (case-insensitive prefix).
	base, leaf, matches := dirCompletion("~/proj")
	if base != home || leaf != "proj" {
		t.Fatalf("base/leaf = %q/%q, want %q/proj", base, leaf, home)
	}
	if strings.Join(matches, ",") != "projetos,projetos-old" {
		t.Errorf("matches = %v, want projetos, projetos-old", matches)
	}

	// A trailing slash lists every subdir of that directory.
	if _, leaf, m := dirCompletion("~/"); leaf != "" || len(m) != 3 {
		t.Errorf("trailing-slash matches = %v (leaf %q), want 3", m, leaf)
	}

	// An empty value falls back to HOME's subdirs.
	if b, _, m := dirCompletion(""); b != home || len(m) != 3 {
		t.Errorf("empty value base=%q matches=%v, want HOME with 3", b, m)
	}
}

func TestRenderDirCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A trailing slash lists the folder's subdirectories.
	out := renderDirCompletion(dir+"/", 40, 10)
	if !strings.Contains(out, "✓") || !strings.Contains(out, "sub/") {
		t.Errorf("completion missing the subdir:\n%s", out)
	}
	// A leaf that matches nothing reports it instead of listing files.
	if !strings.Contains(renderDirCompletion(dir+"/zzz", 40, 10), "nenhuma pasta") {
		t.Error("non-matching leaf should report no matches")
	}
	// empty input lists HOME, not the (project) working dir
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "homefolder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(renderDirCompletion("", 40, 10), "homefolder") {
		t.Error("empty input should list the home directory")
	}
}
