package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

var flagMain bool

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

func main() {
	flag.BoolVar(&flagMain, "main", false, "diff against main branch")
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
	initTheme()

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
	p := tea.NewProgram(initialModel(files), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
