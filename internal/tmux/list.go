package tmux

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adamvduke/pst"
	"github.com/paipaiio/muxlight/internal/status"
)

// PaneStatus is one pane's published status.
type PaneStatus struct {
	Target  string // session:window.pane
	PaneID  string // %N
	State   pst.State
	Kind    pst.Kind
	App     string
	Msg     string
	Updated time.Time
}

// listFormat puts msg last: OSC 7501 text never holds a tab, so splitting on
// the first six tabs is safe.
var listFormat = strings.Join([]string{
	"#{session_name}:#{window_index}.#{pane_index}",
	"#{pane_id}",
	"#{" + OptState + "}",
	"#{" + OptKind + "}",
	"#{" + OptApp + "}",
	"#{" + OptUpdated + "}",
	"#{" + OptMsg + "}",
}, "\t")

// List returns every pane on the server that has a status, most urgent
// first and, within the same state, most recent first.
func List(ctx context.Context, runner Runner) ([]PaneStatus, error) {
	out, err := runner.Run(ctx, "list-panes", "-a", "-F", listFormat)
	if err != nil {
		return nil, fmt.Errorf("list panes: %w", err)
	}
	panes := parseList(string(out))
	sort.SliceStable(panes, func(i, j int) bool {
		ui, uj := status.Urgency(panes[i].State), status.Urgency(panes[j].State)
		if ui != uj {
			return ui > uj
		}
		return panes[i].Updated.After(panes[j].Updated)
	})
	return panes, nil
}

func parseList(out string) []PaneStatus {
	var panes []PaneStatus
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 7)
		if len(f) != 7 || f[2] == "" {
			continue
		}
		secs, _ := strconv.ParseInt(f[5], 10, 64)
		panes = append(panes, PaneStatus{
			Target:  f[0],
			PaneID:  f[1],
			State:   pst.State(f[2]),
			Kind:    pst.Kind(f[3]),
			App:     unsanitize(f[4]),
			Updated: time.Unix(secs, 0),
			Msg:     unsanitize(f[6]),
		})
	}
	return panes
}

// ClearPane unsets the status of any pane, for example after the user has
// looked at a done record.
func ClearPane(ctx context.Context, runner Runner, pane string) error {
	return clearPane(ctx, runner, pane)
}
