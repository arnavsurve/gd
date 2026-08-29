package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func inDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func TestStaleBaseWarning(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "checkout", "-b", "anyone")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "file.txt")
	runGit(t, repo, "commit", "-m", "first")
	runGit(t, repo, "push", "-u", "origin", "anyone")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-am", "second")
	runGit(t, repo, "push")
	runGit(t, repo, "reset", "--hard", "HEAD~1")

	inDirectory(t, repo)
	want := `warning: "anyone" is 1 commits behind "origin/anyone"; use --base origin/anyone to match the remote diff`
	if got := staleBaseWarning("anyone"); got != want {
		t.Fatalf("staleBaseWarning() = %q, want %q", got, want)
	}
	if got := staleBaseWarning("origin/anyone"); got != "" {
		t.Fatalf("explicit remote ref warning = %q, want empty", got)
	}
}

func TestStaleBaseWarningIgnoresLocalOnlyBranch(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "checkout", "-b", "local")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "file.txt")
	runGit(t, repo, "commit", "-m", "first")

	inDirectory(t, repo)
	if got := staleBaseWarning("local"); got != "" {
		t.Fatalf("local-only branch warning = %q, want empty", got)
	}
}
