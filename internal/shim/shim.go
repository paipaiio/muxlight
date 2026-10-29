// Package shim runs a program in its own pseudo-terminal so muxlight can act as
// that program's terminal for OSC 7501.
//
// A program checks for OSC 7501 support by sending the query (OSC 7501 ; ?)
// followed by a device attributes request (CSI c). If the DA reply comes back
// first, it decides the terminal has no support. tmux answers CSI c at once
// and never answers the query, so programs inside tmux stay silent. The shim
// sees the query first and replies before it forwards the CSI c to tmux, so
// the program hears "supported" first. Everything else passes through
// unchanged.
package shim

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/adamvduke/pst"
	"github.com/creack/pty"
	"golang.org/x/term"
)

// queryReply is the minimal feature-detection reply the spec allows: "?"
// with no states or kinds means the fixed revision 0.4 lists.
var queryReply = []byte("\x1b]7501;?\x1b\\")

// Run starts argv in a new pty wired to the current terminal and calls
// onReport for each valid OSC 7501 report the program writes. It returns
// the program's exit code.
func Run(argv []string, onReport func(pst.Report)) (int, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return 1, fmt.Errorf("start %s: %w", argv[0], err)
	}
	defer func() { _ = ptmx.Close() }() // the program has exited; nothing to report

	stopResize := followWindowSize(ptmx)
	defer stopResize()
	stopSignals := forwardSignals(cmd.Process)
	defer stopSignals()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		old, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return 1, fmt.Errorf("set raw mode: %w", err)
		}
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), old) }() // best effort on the way out
	}

	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	pump(ptmx, os.Stdout, onReport)

	return exitCode(cmd.Wait())
}

// pump copies program output to out, answering queries and reporting
// reports on the way. It returns when the program closes the pty.
func pump(ptmx io.ReadWriter, out io.Writer, onReport func(pst.Report)) {
	var sc pst.Scanner
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			sc.Feed(buf[:n], func(seq []byte) { handle(seq, ptmx, onReport) })
			if _, werr := out.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return // EIO once the program exits and the pty closes
		}
	}
}

func handle(seq []byte, ptmx io.Writer, onReport func(pst.Report)) {
	r, err := pst.ParseSequence(seq)
	switch {
	case errors.Is(err, pst.ErrQuery):
		_, _ = ptmx.Write(queryReply)
	case err == nil:
		onReport(r)
	}
	// Discarded and ignored reports are dropped, as the spec requires.
}

func followWindowSize(ptmx *os.File) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH // size the pty once before the program draws
	return func() { signal.Stop(ch); close(ch) }
}

func forwardSignals(p *os.Process) (stop func()) {
	ch := make(chan os.Signal, 1)
	// In raw mode the keyboard sends Ctrl-C as a byte, so SIGINT only arrives
	// from kill(1) or when stdin is not a terminal; either way it is meant for
	// the program.
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range ch {
			_ = p.Signal(sig)
		}
	}()
	return func() { signal.Stop(ch); close(ch) }
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exit.ExitCode(), nil
	}
	return 1, fmt.Errorf("wait: %w", err)
}
