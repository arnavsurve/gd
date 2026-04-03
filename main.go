package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	_ "github.com/arnavsurve/gd/themes"
)

var flagMain bool
var flagSemantic bool
var flagNoSemantic bool

var commitFrom string
var commitTo string

func parseCommitArg(arg string) (from, to string, err error) {
	arg = strings.TrimPrefix(arg, "@")
	if parts := strings.SplitN(arg, "..", 2); len(parts) == 2 {
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return "", "", fmt.Errorf("invalid range start: %s", parts[0])
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", "", fmt.Errorf("invalid range end: %s", parts[1])
		}
		if start >= end {
			return "", "", fmt.Errorf("range start must be less than end: %d..%d", start, end)
		}
		return fmt.Sprintf("HEAD~%d", end), fmt.Sprintf("HEAD~%d", start), nil
	}
	n, err := strconv.Atoi(arg)
	if err != nil {
		return "", "", fmt.Errorf("invalid commit number: %s", arg)
	}
	return fmt.Sprintf("HEAD~%d", n), "HEAD", nil
}

func stdinIsPipe() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

func runPager() {
	loadConfig()
	initTheme(true)

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
		os.Exit(1)
	}
	if len(data) == 0 {
		return
	}

	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}

	rendered, _ := renderDiff(string(data), width, "")
	fmt.Print(rendered)
}

func main() {
	if stdinIsPipe() {
		runPager()
		return
	}

	flag.BoolVar(&flagMain, "main", false, "diff against main branch")
	flag.BoolVar(&flagSemantic, "semantic", false, "enable semantic diff mode (requires sem CLI)")
	flag.BoolVar(&flagNoSemantic, "no-semantic", false, "disable semantic diff mode")
	flag.Parse()

	if args := flag.Args(); len(args) > 0 && strings.HasPrefix(args[0], "@") {
		var err error
		commitFrom, commitTo, err = parseCommitArg(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	loadConfig()
	initTheme(false)

	semantic := false
	if cfg.Semantic != nil {
		semantic = *cfg.Semantic
	}
	if flagSemantic {
		semantic = true
	}
	if flagNoSemantic {
		semantic = false
	}
	if semantic && !semAvailable() {
		fmt.Fprintln(os.Stderr, "semantic mode requires the sem CLI: brew install ataraxy-labs/tap/sem-diff")
		os.Exit(1)
	}

	var files []fileStatus
	var err error
	if commitFrom != "" {
		files, err = getCommitFiles(commitFrom, commitTo)
	} else if flagMain {
		files, err = getMainFiles()
	} else {
		files, err = getChangedFiles()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	p := tea.NewProgram(initialModel(files, semantic), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
