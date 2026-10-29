package status

import (
	"testing"
	"time"

	"github.com/adamvduke/pst"
)

func rec(id string, state pst.State, app string, at int) pst.Record {
	return pst.Record{
		Report:  pst.Report{ID: id, State: state, App: app, Msg: string(state) + " " + id},
		Updated: time.Unix(int64(at), 0),
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name      string
		records   []pst.Record
		wantState pst.State
		wantApp   string
		wantMsg   string
	}{
		{"empty", nil, "", "", ""},
		{"single root", []pst.Record{rec("", pst.StateWorking, "claude-code", 1)}, pst.StateWorking, "claude-code", "working "},
		{
			"blocked child beats working root",
			[]pst.Record{rec("", pst.StateWorking, "deploy", 1), rec("eu", pst.StateBlocked, "", 2)},
			pst.StateBlocked, "deploy", "blocked eu",
		},
		{
			"error beats done",
			[]pst.Record{rec("a", pst.StateDone, "x", 5), rec("b", pst.StateError, "x", 1)},
			pst.StateError, "x", "error b",
		},
		{
			"tie goes to most recent",
			[]pst.Record{rec("a", pst.StateDone, "x", 1), rec("b", pst.StateDone, "x", 9)},
			pst.StateDone, "x", "done b",
		},
		{
			"app inherited from grandparent",
			[]pst.Record{rec("", pst.StateIdle, "build", 1), rec("a/b", pst.StateBlocked, "", 2)},
			pst.StateBlocked, "build", "blocked a/b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.records)
			if got.State != tt.wantState || got.App != tt.wantApp || got.Msg != tt.wantMsg {
				t.Fatalf("Summarize() = %+v, want state=%q app=%q msg=%q", got, tt.wantState, tt.wantApp, tt.wantMsg)
			}
			if got.Records != len(tt.records) {
				t.Fatalf("Records = %d, want %d", got.Records, len(tt.records))
			}
		})
	}
}

func TestParent(t *testing.T) {
	for id, want := range map[string]string{"": "", "a": "", "a/b": "a", "a/b/c": "a/b"} {
		if got := parent(id); got != want {
			t.Errorf("parent(%q) = %q, want %q", id, got, want)
		}
	}
}
