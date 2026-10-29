package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/adamvduke/pst"
	"github.com/paipaiio/muxlight/internal/shim"
	"github.com/paipaiio/muxlight/internal/status"
	"github.com/paipaiio/muxlight/internal/tmux"
)

const publishTimeout = 2 * time.Second

func cmdRun(args []string, e env) (int, error) {
	if len(args) == 0 {
		return 2, errors.New("run: missing command")
	}
	pane := e.getenv("TMUX_PANE")
	if pane == "" {
		// Outside tmux the real terminal answers the query itself, and a
		// second reply would reach the program as stray input.
		return 1, e.exec(args)
	}

	store := pst.NewStore()
	pub := newAsyncPublisher(tmux.NewPublisher(e.runner, pane))
	pub.publish(status.Summary{}) // drop a previous run's record

	code, err := shim.RunWith(args, e.stdin, e.stdout, func(r pst.Report) {
		if len(store.Apply(r)) > 0 {
			pub.send(status.Summarize(store.All()))
		}
	})
	store.ProcessExited() // spec lifetime rule: working and blocked go, done and error stay
	pub.close(status.Summarize(store.All()))
	pub.reportErrors(e.stderr)
	return code, err
}

func execDirect(args []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return fmt.Errorf("run: %w", syscall.Exec(path, args, os.Environ()))
}

// asyncPublisher keeps tmux calls off the output path. Only the latest
// summary matters, so a pending one is replaced rather than queued.
//
// Errors are held until the program exits: writing to stderr while an agent
// draws its full-screen UI would corrupt that UI.
type asyncPublisher struct {
	publisher *tmux.Publisher
	latest    chan status.Summary
	done      chan struct{}

	mu       sync.Mutex
	firstErr error
	failures int
}

func newAsyncPublisher(p *tmux.Publisher) *asyncPublisher {
	a := &asyncPublisher{publisher: p, latest: make(chan status.Summary, 1), done: make(chan struct{})}
	go a.loop()
	return a
}

func (a *asyncPublisher) send(s status.Summary) {
	for {
		select {
		case a.latest <- s:
			return
		default:
			select {
			case <-a.latest: // drop the stale one
			default:
			}
		}
	}
}

func (a *asyncPublisher) loop() {
	defer close(a.done)
	for s := range a.latest {
		a.publish(s)
	}
}

// close publishes final synchronously after everything already sent.
func (a *asyncPublisher) close(final status.Summary) {
	close(a.latest)
	<-a.done
	a.publish(final)
}

func (a *asyncPublisher) publish(s status.Summary) {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := a.publisher.Publish(ctx, s); err != nil {
		a.mu.Lock()
		if a.firstErr == nil {
			a.firstErr = err
		}
		a.failures++
		a.mu.Unlock()
	}
}

// reportErrors writes a one-line summary of failed tmux updates, if any.
func (a *asyncPublisher) reportErrors(w io.Writer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failures > 0 {
		_, _ = fmt.Fprintf(w, "muxlight: %d status update(s) failed; first: %v\n", a.failures, a.firstErr)
	}
}
