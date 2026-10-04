package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunExitCodes pins the exit-code contract CI callers depend on.
func TestRunExitCodes(t *testing.T) {
	trivialBase := writeTree(t, "tb", map[string]string{"a.go": "package p\nfunc f(){}\n"})
	trivialHead := writeTree(t, "th", map[string]string{"a.go": "package p\nfunc f(){}\n"})
	broadHead := map[string]string{}
	for i := 0; i < 8; i++ {
		broadHead[filepath.Join("pkg", "f"+string(rune('a'+i))+".go")] = "package p\nfunc X(){}\n"
	}
	broadBase := writeTree(t, "bb", nil)
	broad := writeTree(t, "bh", broadHead)

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no_review", []string{"--base", trivialBase, "--head", trivialHead}, 0},
		{"review_required", []string{"--base", broadBase, "--head", broad}, 1},
		{"exit_zero", []string{"--base", broadBase, "--head", broad, "--exit-zero"}, 0},
		{"missing_head", []string{"--base", trivialBase}, 2},
		{"no_args", nil, 2},
		{"both_modes", []string{"--base", trivialBase, "--head", trivialHead, "--repo", ".", "--base-ref", "main"}, 2},
		{"bad_flag", []string{"--nope"}, 2},
		{"missing_dir", []string{"--base", filepath.Join(t.TempDir(), "absent"), "--head", trivialHead}, 2},
		// A threshold switched off cannot fire; breadth is what flags this one.
		{"breadth_off", []string{"--base", broadBase, "--head", broad, "--theta-breadth", "off"}, 0},
		// Zero would flag every change; "off" is the only way to disable.
		{"zero_threshold", []string{"--base", trivialBase, "--head", trivialHead, "--theta-depth", "0"}, 2},
		{"bad_threshold", []string{"--base", trivialBase, "--head", trivialHead, "--theta-cog", "lots"}, 2},
		{"bad_level", []string{"--base", trivialBase, "--head", trivialHead, "--theta-significance", "severe"}, 2},
		{"level_name", []string{"--base", trivialBase, "--head", trivialHead, "--theta-significance", "high"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(c.args, &stdout, &stderr); got != c.want {
				t.Fatalf("exit = %d, want %d; stderr: %s", got, c.want, stderr.String())
			}
		})
	}
}

// TestReportContract checks the fields the GitHub Action and other CI read,
// and that --output writes the same report as stdout.
func TestReportContract(t *testing.T) {
	base := writeTree(t, "base", nil)
	headFiles := map[string]string{}
	for i := 0; i < 8; i++ {
		headFiles[filepath.Join("pkg", "f"+string(rune('a'+i))+".go")] = "package p\nfunc X(){}\n"
	}
	head := writeTree(t, "head", headFiles)
	outFile := filepath.Join(t.TempDir(), "report.json")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--base", base, "--head", head, "--output", outFile}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, stderr.String())
	}
	saved, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(saved), bytes.TrimSpace(stdout.Bytes())) {
		t.Fatal("--output file differs from stdout")
	}

	var rep map[string]any
	if err := json.Unmarshal(saved, &rep); err != nil {
		t.Fatal(err)
	}
	if rep["schemaVersion"] != float64(schemaVersion) {
		t.Errorf("schemaVersion = %v, want %d", rep["schemaVersion"], schemaVersion)
	}
	if rep["verdict"] != verdictReviewRequired || rep["requiresReview"] != true {
		t.Errorf("verdict = %v, requiresReview = %v", rep["verdict"], rep["requiresReview"])
	}
	if r, _ := rep["reasons"].([]any); len(r) != 1 || r[0] != "breadth-files" {
		t.Errorf("reasons = %v, want [breadth-files]", rep["reasons"])
	}
	scores, _ := rep["scores"].(map[string]any)
	for _, k := range []string{"editDepth", "newDepth", "depthTotal", "breadthFiles",
		"breadthModules", "cogDelta", "newFunctionComplexity", "significance", "blastRadius"} {
		if _, ok := scores[k]; !ok {
			t.Errorf("scores.%s missing; every measure is reported, null when unavailable", k)
		}
	}
	gate, _ := rep["gate"].(map[string]any)
	if gate["thetaDepth"] != float64(7) || gate["thetaModules"] != nil ||
		gate["thetaSignificance"] != "crucial" {
		t.Errorf("gate = %v, want the defaults (modules off)", gate)
	}
	facts, _ := rep["facts"].(map[string]any)
	if facts["unparsed"] != false || facts["trusted"] != false || facts["analysisFailed"] != false {
		t.Errorf("facts = %v", facts)
	}
}

// TestGitMode builds a small repository with a branch and checks that git
// mode diffs the branch against its merge base, not against the base branch
// tip, and fails closed when the base ref is missing.
func TestGitMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	sh := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(files map[string]string) {
		t.Helper()
		for p, content := range files {
			full := filepath.Join(repo, p)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	sh("init", "-q", "-b", "main")
	write(map[string]string{"a.go": "package p\nfunc f(){}\n"})
	sh("add", ".")
	sh("commit", "-q", "-m", "base")
	sh("checkout", "-q", "-b", "feature")
	write(map[string]string{"a.go": "package p\nfunc f(){ _ = 1 }\n"})
	sh("commit", "-q", "-am", "small change")

	// A broad change landing on main after the branch point must not count
	// against the feature branch.
	sh("checkout", "-q", "main")
	broad := map[string]string{}
	for i := 0; i < 8; i++ {
		broad[filepath.Join("pkg", "f"+string(rune('a'+i))+".go")] = "package p\nfunc X(){}\n"
	}
	write(broad)
	sh("add", ".")
	sh("commit", "-q", "-m", "broad change on main")
	sh("checkout", "-q", "feature")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--repo", repo, "--base-ref", "main"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s\n%s", code, stderr.String(), stdout.String())
	}
	var rep report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Files) != 1 || rep.Files[0].Path != "a.go" {
		t.Fatalf("files = %+v, want only a.go", rep.Files)
	}

	stderr.Reset()
	if code := run([]string{"--repo", repo, "--base-ref", "origin/main"}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing base ref: exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "origin/main") {
		t.Fatalf("error should name the missing ref, got: %s", stderr.String())
	}
}

// TestVersion checks --version prints the build-time version and nothing
// else, and needs no trees: release tooling runs it as a smoke test.
func TestVersion(t *testing.T) {
	old := version
	version = "1.2.3"
	defer func() { version = old }()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if got := stdout.String(); got != "1.2.3\n" {
		t.Fatalf("stdout %q, want %q", got, "1.2.3\n")
	}
}
