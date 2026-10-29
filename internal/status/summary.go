// Package status reduces a terminal's OSC 7501 record set to the one line a
// tmux pane can show.
package status

import (
	"time"

	"github.com/adamvduke/pst"
)

// Summary is what muxlight publishes for a pane: the most urgent record.
type Summary struct {
	State   pst.State
	Kind    pst.Kind
	App     string
	Msg     string
	Updated time.Time
	// Records is how many records the pane holds in total.
	Records int
}

// Empty reports whether there is nothing to show.
func (s Summary) Empty() bool { return s.State == "" }

// urgency orders states so the one most likely to need the user wins.
var urgency = map[pst.State]int{
	pst.StateBlocked: 5,
	pst.StateError:   4,
	pst.StateDone:    3,
	pst.StateWorking: 2,
	pst.StateIdle:    1,
}

// Urgency returns the rank of state; unknown states rank lowest.
func Urgency(state pst.State) int { return urgency[state] }

// Summarize picks the most urgent record. Ties go to the most recently
// updated record. App falls back to the nearest ancestor's app, as the spec's
// app inheritance rule requires.
func Summarize(records []pst.Record) Summary {
	var best *pst.Record
	for i := range records {
		r := &records[i]
		if best == nil || better(r, best) {
			best = r
		}
	}
	if best == nil {
		return Summary{}
	}
	return Summary{
		State:   best.State,
		Kind:    best.Kind,
		App:     inheritedApp(best, records),
		Msg:     best.Msg,
		Updated: best.Updated,
		Records: len(records),
	}
}

func better(a, b *pst.Record) bool {
	ua, ub := Urgency(a.State), Urgency(b.State)
	if ua != ub {
		return ua > ub
	}
	return a.Updated.After(b.Updated)
}

func inheritedApp(r *pst.Record, all []pst.Record) string {
	if r.App != "" {
		return r.App
	}
	byID := make(map[string]string, len(all))
	for _, rec := range all {
		byID[rec.ID] = rec.App
	}
	for id := parent(r.ID); ; id = parent(id) {
		if app := byID[id]; app != "" {
			return app
		}
		if id == "" {
			return ""
		}
	}
}

// parent returns the parent id: "a/b" → "a", "a" → "" (the root record).
func parent(id string) string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '/' {
			return id[:i]
		}
	}
	return ""
}
