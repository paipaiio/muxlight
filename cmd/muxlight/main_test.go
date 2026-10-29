package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adamvduke/pst"
	"github.com/paipaiio/muxlight/internal/status"
	"github.com/paipaiio/muxlight/internal/tmux"
)

// fakeRunner records tmux invocations. It is safe for the publisher's
// goroutine. gate, if set, blocks the first call until it is closed.
type fakeRunner struct {
	mu    sync.Mutex
	calls []string
	out   string
	err   error
	gate  chan struct{}
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	gate := f.gate
	f.gate = nil
	f.calls = append(f.calls, strings.Join(args, " "))
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return []byte(f.out), f.err
}

func (f *fakeRunner) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func testEnv(t *testing.T, runner tmux.Runner, vars map[string]string) (env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	var stdout, stderr bytes.Buffer
	return env{
		runner: runner,
		stdin:  in,
		stdout: &stdout,
		stderr: &stderr,
		getenv: func(k string) string { return vars[k] },
		exec: func([]string) error {
			t.Fatal("exec must not be called")
			return nil
		},
	}, &stdout, &stderr
}

func TestRunDispatch(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"no args", nil, 2, "", "Usage:"},
		{"version", []string{"version"}, 0, "muxlight dev", ""},
		{"help", []string{"--help"}, 0, "Usage:", ""},
		{"unknown", []string{"frob"}, 2, "", `unknown command "frob"`},
		{"run without command", []string{"run"}, 2, "", "missing command"},
		{"clear outside tmux", []string{"clear"}, 1, "", "not inside tmux"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, stdout, stderr := testEnv(t, &fakeRunner{}, nil)
			if code := run(tt.args, e); code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout %q lacks %q", stdout, tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr %q lacks %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestListCommand(t *testing.T) {
	r := &fakeRunner{out: "w:1.0\t%3\tblocked\tpermission\tclaude-code\t0\tRun rm? ##1\n"}
	e, stdout, _ := testEnv(t, r, nil)
	if code := run([]string{"ls"}, e); code != 0 {
		t.Fatalf("code = %d", code)
	}
	for _, want := range []string{"PANE", "w:1.0", "blocked/permission", "claude-code", "Run rm? #1"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("ls output lacks %q:\n%s", want, stdout)
		}
	}

	e, stdout, _ = testEnv(t, &fakeRunner{}, nil)
	if code := run([]string{"ls"}, e); code != 0 || !strings.Contains(stdout.String(), "no pane has a status") {
		t.Errorf("empty ls: code %d, output %q", code, stdout)
	}

	e, _, stderr := testEnv(t, &fakeRunner{err: errors.New("no server running")}, nil)
	if code := run([]string{"ls"}, e); code != 1 || !strings.Contains(stderr.String(), "no server running") {
		t.Errorf("failing ls: code %d, stderr %q", code, stderr)
	}
}

func TestClearCommand(t *testing.T) {
	r := &fakeRunner{}
	e, _, _ := testEnv(t, r, map[string]string{"TMUX_PANE": "%4"})
	if code := run([]string{"clear"}, e); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if calls := r.snapshot(); len(calls) != 1 || !strings.Contains(calls[0], "set-option -p -u -t %4 @pst_state") {
		t.Fatalf("calls = %q", calls)
	}
	r = &fakeRunner{}
	e, _, _ = testEnv(t, r, nil)
	if code := run([]string{"clear", "%9"}, e); code != 0 || !strings.Contains(r.snapshot()[0], "-t %9") {
		t.Fatalf("explicit pane: code %d, calls %q", code, r.snapshot())
	}
}

func TestRunCommandPublishesStatus(t *testing.T) {
	r := &fakeRunner{}
	e, stdout, stderr := testEnv(t, r, map[string]string{"TMUX_PANE": "%7"})
	script := `printf 'out\n\033]7501;state=working:app=sh\033\\'; ` +
		`printf '\033]7501;state=done:app=sh:msg=b2s=\033\\'; exit 4`
	code := run([]string{"run", "sh", "-c", script}, e)
	if code != 4 {
		t.Fatalf("code = %d, want 4 (stderr %q)", code, stderr)
	}
	if !strings.Contains(stdout.String(), "out") {
		t.Errorf("program output not passed through: %q", stdout)
	}
	calls := r.snapshot()
	if len(calls) < 2 {
		t.Fatalf("calls = %q", calls)
	}
	if !strings.Contains(calls[0], "set-option -p -u -t %7 @pst_state") {
		t.Errorf("first call should clear the pane: %q", calls[0])
	}
	last := calls[len(calls)-1]
	for _, want := range []string{"@pst_state done", "@pst_app sh", "@pst_msg ok"} {
		if !strings.Contains(last, want) {
			t.Errorf("last call %q lacks %q", last, want)
		}
	}
}

func TestRunCommandOutsideTmuxExecs(t *testing.T) {
	e, _, stderr := testEnv(t, &fakeRunner{}, nil)
	var got []string
	e.exec = func(argv []string) error { got = argv; return errors.New("exec failed") }
	if code := run([]string{"run", "claude", "--resume"}, e); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if strings.Join(got, " ") != "claude --resume" || !strings.Contains(stderr.String(), "exec failed") {
		t.Fatalf("exec argv %q, stderr %q", got, stderr)
	}
}

func TestAsyncPublisherKeepsOnlyLatest(t *testing.T) {
	gate := make(chan struct{})
	r := &fakeRunner{gate: gate}
	a := newAsyncPublisher(tmux.NewPublisher(r, "%1"))
	summary := func(app string) status.Summary {
		return status.Summary{State: pst.StateWorking, App: app, Updated: time.Unix(1, 0)}
	}

	a.send(summary("first")) // the loop takes it and blocks on the gate
	waitFor(t, func() bool { return len(r.snapshot()) == 1 })
	a.send(summary("stale"))
	a.send(summary("latest")) // replaces "stale" while the loop is busy
	close(gate)
	a.close(summary("final"))

	var apps []string
	for _, c := range r.snapshot() {
		for _, app := range []string{"first", "stale", "latest", "final"} {
			if strings.Contains(c, "@pst_app "+app) {
				apps = append(apps, app)
			}
		}
	}
	if got := strings.Join(apps, ","); got != "first,latest,final" {
		t.Fatalf("published %s, want first,latest,final", got)
	}
}

func TestAsyncPublisherReportsErrorsOnce(t *testing.T) {
	r := &fakeRunner{err: errors.New("boom")}
	a := newAsyncPublisher(tmux.NewPublisher(r, "%1"))
	a.send(status.Summary{State: pst.StateIdle})
	a.close(status.Summary{})
	var buf bytes.Buffer
	a.reportErrors(&buf)
	if got := buf.String(); !strings.Contains(got, "2 status update(s) failed") || !strings.Contains(got, "boom") {
		t.Fatalf("report = %q", got)
	}

	ok := newAsyncPublisher(tmux.NewPublisher(&fakeRunner{}, "%1"))
	ok.close(status.Summary{})
	buf.Reset()
	ok.reportErrors(&buf)
	if buf.Len() != 0 {
		t.Fatalf("no failures, but reported %q", buf.String())
	}
}

func TestAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		5 * time.Second:  "5s",
		90 * time.Second: "1m",
		3 * time.Hour:    "3h",
	} {
		if got := age(d); got != want {
			t.Errorf("age(%v) = %q, want %q", d, got, want)
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
