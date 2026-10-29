package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/paipaiio/muxlight/internal/tmux"
)

func cmdList(w io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	panes, err := tmux.List(ctx, tmux.ExecRunner{})
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		_, err := fmt.Fprintln(w, "no pane has a status")
		return err
	}
	// tabwriter buffers everything until Flush, so write errors surface there.
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PANE\tSTATE\tAPP\tAGE\tMESSAGE")
	now := time.Now()
	for _, p := range panes {
		state := string(p.State)
		if p.Kind != "" {
			state += "/" + string(p.Kind)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.Target, state, p.App, age(now.Sub(p.Updated)), p.Msg)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write list: %w", err)
	}
	return nil
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

func cmdClear(args []string) error {
	pane := os.Getenv("TMUX_PANE")
	if len(args) > 0 {
		pane = args[0]
	}
	if pane == "" {
		return errors.New("clear: not inside tmux; pass a pane id such as %3")
	}
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	return tmux.ClearPane(ctx, tmux.ExecRunner{}, pane)
}
