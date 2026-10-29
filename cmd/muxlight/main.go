// Command muxlight brings OSC 7501 program status to tmux.
//
//	muxlight run <command> [args...]   run an agent so it reports status to its pane
//	muxlight ls                        list panes that have a status, most urgent first
//	muxlight clear [pane]              forget a pane's status (default: this pane)
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/paipaiio/muxlight/internal/tmux"
)

var version = "dev"

const usage = `muxlight brings OSC 7501 program status to tmux.

Usage:
  muxlight run <command> [args...]   run an agent so it reports status to its pane
  muxlight ls                        list panes that have a status, most urgent first
  muxlight clear [pane]              forget a pane's status (default: this pane)
  muxlight version
`

// env is everything a command touches outside the process, so tests can
// replace it.
type env struct {
	runner tmux.Runner
	stdin  *os.File
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
	// exec replaces the process with argv. It must not return on success.
	exec func(argv []string) error
}

func defaultEnv() env {
	return env{
		runner: tmux.ExecRunner{},
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		getenv: os.Getenv,
		exec:   execDirect,
	}
}

func main() {
	os.Exit(run(os.Args[1:], defaultEnv()))
}

// run dispatches one command line and returns the exit code.
func run(args []string, e env) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, usage)
		return 2
	}
	var code int
	var err error
	switch args[0] {
	case "run":
		code, err = cmdRun(args[1:], e)
	case "ls":
		err = cmdList(e)
	case "clear":
		err = cmdClear(args[1:], e)
	case "version", "--version":
		_, err = fmt.Fprintln(e.stdout, "muxlight", version)
	case "help", "-h", "--help":
		_, err = fmt.Fprint(e.stdout, usage)
	default:
		_, _ = fmt.Fprintf(e.stderr, "muxlight: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintln(e.stderr, "muxlight:", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}
