// Package tmux publishes pane status as tmux user options and reads it back.
//
// Options set on each pane:
//
//	@pst_state    idle | working | blocked | done | error
//	@pst_kind     permission | question | auth (blocked only)
//	@pst_app      the program's app id, e.g. claude-code
//	@pst_msg      one line of human-readable text
//	@pst_updated  unix seconds of the last change
package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/paipaiio/muxlight/internal/status"
)

// Option names, exported so docs and the status-line snippet stay in sync.
const (
	OptState   = "@pst_state"
	OptKind    = "@pst_kind"
	OptApp     = "@pst_app"
	OptMsg     = "@pst_msg"
	OptUpdated = "@pst_updated"
)

var allOptions = []string{OptState, OptKind, OptApp, OptMsg, OptUpdated}

// Runner runs one tmux invocation. It exists so tests can record commands.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner runs the tmux binary found on PATH. Inside a pane, tmux finds
// the right server through $TMUX.
type ExecRunner struct{}

// Run executes tmux with args and returns its stdout.
func (ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return out, nil
}

// Publisher writes one pane's summary into that pane's options.
type Publisher struct {
	runner Runner
	pane   string
}

// NewPublisher returns a Publisher for pane, a tmux pane id such as "%3".
func NewPublisher(runner Runner, pane string) *Publisher {
	return &Publisher{runner: runner, pane: pane}
}

// Publish sets the pane's options to s, or unsets them all when s is empty.
// All options change in one tmux invocation so a status line never shows a
// half-updated pane.
func (p *Publisher) Publish(ctx context.Context, s status.Summary) error {
	if s.Empty() {
		return p.Clear(ctx)
	}
	values := map[string]string{
		OptState:   string(s.State),
		OptKind:    string(s.Kind),
		OptApp:     sanitize(s.App),
		OptMsg:     sanitize(s.Msg),
		OptUpdated: strconv.FormatInt(s.Updated.Unix(), 10),
	}
	var args []string
	for _, name := range allOptions {
		if len(args) > 0 {
			args = append(args, ";")
		}
		if v := values[name]; v != "" {
			args = append(args, "set-option", "-p", "-t", p.pane, name, v)
		} else {
			args = append(args, "set-option", "-p", "-u", "-t", p.pane, name)
		}
	}
	if _, err := p.runner.Run(ctx, args...); err != nil {
		return fmt.Errorf("publish status for pane %s: %w", p.pane, err)
	}
	return nil
}

// Clear unsets every muxlight option on the pane.
func (p *Publisher) Clear(ctx context.Context) error {
	return clearPane(ctx, p.runner, p.pane)
}

func clearPane(ctx context.Context, runner Runner, pane string) error {
	var args []string
	for _, name := range allOptions {
		if len(args) > 0 {
			args = append(args, ";")
		}
		args = append(args, "set-option", "-p", "-u", "-t", pane, name)
	}
	if _, err := runner.Run(ctx, args...); err != nil {
		return fmt.Errorf("clear status for pane %s: %w", pane, err)
	}
	return nil
}

// sanitize keeps text from changing how tmux draws the status line or parses
// the command. tmux parses "#[...]" in expanded status text as a style
// directive and draws "##" as a literal "#", so every "#" is doubled.
// "#(...)" in an option value is never run as a command. tmux also reads an
// argument ending in ";" as a command separator, dropping the ";" or, for a
// lone ";", failing the whole call, so a trailing ";" is written as "\;".
// (A text that itself ends in "\;" loses that backslash; tmux has no way to
// express it.) Control characters never get here: the OSC 7501 parser
// already rejects reports that contain them.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "#", "##")
	if strings.HasSuffix(s, ";") {
		s = s[:len(s)-1] + `\;`
	}
	return s
}

// unsanitize reverses sanitize for text read back with list-panes.
func unsanitize(s string) string {
	return strings.ReplaceAll(s, "##", "#")
}
