package tmux

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adamvduke/pst"
	"github.com/paipaiio/muxlight/internal/status"
)

type recorder struct {
	calls [][]string
	out   string
}

func (r *recorder) Run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	return []byte(r.out), nil
}

func TestPublishSetsAndUnsetsInOneCall(t *testing.T) {
	rec := &recorder{}
	p := NewPublisher(rec, "%7")
	s := status.Summary{State: pst.StateWorking, App: "claude-code", Msg: "use #[fg=red] here", Updated: time.Unix(42, 0)}
	if err := p.Publish(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("got %d tmux calls, want 1", len(rec.calls))
	}
	cmd := strings.Join(rec.calls[0], " ")
	for _, want := range []string{
		"set-option -p -t %7 @pst_state working",
		"set-option -p -u -t %7 @pst_kind", // no kind: unset, not empty
		"set-option -p -t %7 @pst_app claude-code",
		"set-option -p -t %7 @pst_updated 42",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q lacks %q", cmd, want)
		}
	}
	if !strings.Contains(cmd, "@pst_msg use ##[fg=red] here") {
		t.Errorf("msg not escaped for tmux: %q", cmd)
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"#[fg=red]", "##[fg=red]"},
		{"already ##", "already ####"},
		{"ends;", `ends\;`},
		{";", `\;`},
		{"mid;dle", "mid;dle"},
		{"#;", `##\;`},
	}
	for _, tt := range tests {
		if got := sanitize(tt.in); got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// What tmux stores comes back through list-panes; "#" must round-trip.
	for _, s := range []string{"a#b", "##", "###", "#[x]"} {
		if got := unsanitize(sanitize(s)); got != s {
			t.Errorf("round trip %q -> %q", s, got)
		}
	}
}

func TestPublishEmptyClears(t *testing.T) {
	rec := &recorder{}
	if err := NewPublisher(rec, "%1").Publish(context.Background(), status.Summary{}); err != nil {
		t.Fatal(err)
	}
	cmd := strings.Join(rec.calls[0], " ")
	if strings.Count(cmd, "set-option -p -u") != len(allOptions) {
		t.Fatalf("clear should unset every option, got %q", cmd)
	}
}

func TestListParsesAndSorts(t *testing.T) {
	rec := &recorder{out: strings.Join([]string{
		"main:0.0\t%0\t\t\t\t\t",                                          // no status: skipped
		"main:0.1\t%1\tdone\t\tcodex\t100\tAll tests pass",                // done
		"work:2.0\t%5\tblocked\tpermission\tclaude-code\t50\tRun rm? ##1", // blocked wins
		"work:2.1\t%6\tdone\t\tpi\t200\tnewer done",                       // newer done first
		"",
	}, "\n")}
	panes, err := List(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range panes {
		got = append(got, p.PaneID)
	}
	if want := "%5 %6 %1"; strings.Join(got, " ") != want {
		t.Fatalf("order = %v, want %s", got, want)
	}
	if panes[0].Kind != pst.KindPermission || panes[0].Msg != "Run rm? #1" || panes[0].Target != "work:2.0" {
		t.Fatalf("first pane parsed wrong: %+v", panes[0])
	}
}
