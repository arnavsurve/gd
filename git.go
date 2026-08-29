package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type fileStatus struct {
	path      string
	staged    bool
	unstaged  bool
	untracked bool
}

func (f fileStatus) statusLabel() string {
	if f.untracked {
		return "?"
	}
	var s string
	if f.staged {
		s += "S"
	}
	if f.unstaged {
		s += "M"
	}
	return s
}

func getChangedFiles() ([]fileStatus, error) {
	out, err := exec.Command("git", "status", "--porcelain", "-uall").Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	seen := map[string]*fileStatus{}
	var order []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1]
		path := line[3:]
		if idx := strings.Index(path, " -> "); idx != -1 {
			path = path[idx+4:]
		}
		fs, ok := seen[path]
		if !ok {
			fs = &fileStatus{path: path}
			seen[path] = fs
			order = append(order, path)
		}
		if x == '?' && y == '?' {
			fs.untracked = true
		} else {
			if x != ' ' && x != '?' {
				fs.staged = true
			}
			if y != ' ' && y != '?' {
				fs.unstaged = true
			}
		}
	}
	files := make([]fileStatus, 0, len(order))
	for _, p := range order {
		files = append(files, *seen[p])
	}
	return files, nil
}

func getCommitFiles(from, to string) ([]fileStatus, error) {
	out, err := exec.Command("git", "diff", "--name-only", from+".."+to).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s..%s: %w", from, to, err)
	}
	var files []fileStatus
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			files = append(files, fileStatus{path: line})
		}
	}
	return files, nil
}

func staleBaseWarning(base string) string {
	if base == "" || strings.Contains(base, "/") {
		return ""
	}
	if err := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+base).Run(); err != nil {
		return ""
	}
	remote := "origin/" + base
	if err := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/remotes/"+remote).Run(); err != nil {
		return ""
	}
	out, err := exec.Command("git", "rev-list", "--count", base+".."+remote).Output()
	if err != nil {
		return ""
	}
	behind, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || behind == 0 {
		return ""
	}
	return fmt.Sprintf("warning: %q is %d commits behind %q; use --base %s to match the remote diff", base, behind, remote, remote)
}

func getBaseFiles(base string) ([]fileStatus, error) {
	spec := base + "...HEAD"
	out, err := exec.Command("git", "diff", "--name-only", spec).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s: %w", spec, err)
	}
	var files []fileStatus
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			files = append(files, fileStatus{path: line})
		}
	}
	return files, nil
}

func filesEqual(a, b []fileStatus) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func getDiffOutput(f fileStatus, fullFile bool, contextLines int) string {
	ctx := fmt.Sprintf("-U%d ", contextLines)
	if fullFile {
		ctx = "-U99999 "
	}
	var cmds []string
	if commitFrom != "" {
		cmds = append(cmds, fmt.Sprintf("git diff %s%s..%s -- %q", ctx, commitFrom, commitTo, f.path))
	} else if flagBase != "" {
		cmds = append(cmds, fmt.Sprintf("git diff %s%s...HEAD -- %q", ctx, flagBase, f.path))
	} else {
		if f.unstaged {
			cmds = append(cmds, fmt.Sprintf("git diff %s-- %q", ctx, f.path))
		}
		if f.staged {
			cmds = append(cmds, fmt.Sprintf("git diff --staged %s-- %q", ctx, f.path))
		}
		if f.untracked {
			cmds = append(cmds, fmt.Sprintf("git diff --no-index %s-- /dev/null %q 2>/dev/null", ctx, f.path))
		}
	}
	var cmd string
	if len(cmds) == 1 {
		cmd = cmds[0]
	} else {
		cmd = "{ " + strings.Join(cmds, "; ") + "; }"
	}
	out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
	return string(out)
}
