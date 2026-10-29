// Command muxlight brings OSC 7501 program status to tmux.
//
//	muxlight run <command> [args...]   run an agent so it reports status to its pane
//	muxlight ls                        list panes that have a status, most urgent first
//	muxlight clear [pane]              forget a pane's status (default: this pane)
package main

import (
	"fmt"
	"os"
)

var version = "dev"

const usage = `muxlight brings OSC 7501 program status to tmux.

Usage:
  muxlight run <command> [args...]   run an agent so it reports status to its pane
  muxlight ls                        list panes that have a status, most urgent first
  muxlight clear [pane]              forget a pane's status (default: this pane)
  muxlight version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var code int
	var err error
	switch os.Args[1] {
	case "run":
		code, err = cmdRun(os.Args[2:])
	case "ls":
		err = cmdList(os.Stdout)
	case "clear":
		err = cmdClear(os.Args[2:])
	case "version", "--version":
		fmt.Println("muxlight", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "muxlight: unknown command %q\n\n%s", os.Args[1], usage)
		code = 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "muxlight:", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
