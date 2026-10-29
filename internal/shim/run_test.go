package shim

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/adamvduke/pst"
	"golang.org/x/term"
)

// helperEnv makes the test binary act as the program under the shim instead
// of running tests. RunWith starts os.Args[0], which is this binary.
const helperEnv = "MUXLIGHT_SHIM_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "agent":
		os.Exit(helperAgent())
	case "size":
		width, height, err := term.GetSize(0)
		if err != nil {
			os.Exit(20)
		}
		fmt.Printf("size=%dx%d\r\n", height, width) // rows x cols, like pty.Winsize
		os.Exit(0)
	case "sigterm":
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		time.Sleep(5 * time.Second)
		os.Exit(21)
	default:
		os.Exit(22)
	}
}

// helperAgent detects OSC 7501 the way real agents do: query, then CSI c,
// then wait for whichever reply comes first. Under the shim nothing answers
// CSI c, so only the shim's reply can arrive.
func helperAgent() int {
	old, err := term.MakeRaw(0)
	if err != nil {
		return 10
	}
	defer func() { _ = term.Restore(0, old) }()

	_, _ = os.Stdout.WriteString("hello from helper\r\n\x1b]7501;?\x07\x1b[c")
	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := os.Stdin.Read(buf)
		got <- buf[:n]
	}()
	select {
	case b := <-got:
		if !bytes.Contains(b, []byte("\x1b]7501;?")) {
			return 11
		}
	case <-time.After(3 * time.Second):
		return 12
	}
	emit("state=working:app=helper")
	emit("state=blocked:kind=question:app=helper:msg=" + base64.StdEncoding.EncodeToString([]byte("Pick one")))
	emit("state=done:app=helper")
	return 7
}

func emit(body string) { _, _ = os.Stdout.WriteString("\x1b]7501;" + body + "\x1b\\") }

// runHelper runs this test binary under the shim in the given helper mode,
// with /dev/null as input, as when tmux runs it without a terminal.
func runHelper(t *testing.T, mode string) (code int, out string, reports []pst.Report) {
	t.Helper()
	t.Setenv(helperEnv, mode)
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	var buf bytes.Buffer
	code, err = RunWith([]string{os.Args[0]}, in, &buf, func(r pst.Report) { reports = append(reports, r) })
	if err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	return code, buf.String(), reports
}

func TestRunWithAnswersQueryAndReports(t *testing.T) {
	code, out, reports := runHelper(t, "agent")
	if code != 7 {
		t.Fatalf("exit code = %d, want 7 (10-12 mean the helper failed to detect support)", code)
	}
	if !strings.Contains(out, "hello from helper") {
		t.Errorf("program output not passed through: %q", out)
	}
	var states []string
	for _, r := range reports {
		states = append(states, string(r.State))
	}
	if got := strings.Join(states, ","); got != "working,blocked,done" {
		t.Fatalf("states = %s, want working,blocked,done", got)
	}
	if reports[1].Kind != pst.KindQuestion || reports[1].Msg != "Pick one" {
		t.Errorf("blocked report = %+v", reports[1])
	}
}

func TestRunWithUsesDefaultSizeWithoutTerminal(t *testing.T) {
	code, out, _ := runHelper(t, "size")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	want := fmt.Sprintf("size=%dx%d", defaultSize.Rows, defaultSize.Cols)
	if !strings.Contains(out, want) {
		t.Fatalf("output %q lacks %q", out, want)
	}
}

func TestRunWithReportsSignalExit(t *testing.T) {
	code, _, _ := runHelper(t, "sigterm")
	if want := 128 + int(syscall.SIGTERM); code != want {
		t.Fatalf("exit code = %d, want %d", code, want)
	}
}

func TestRunWithStartErrors(t *testing.T) {
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	for _, argv := range [][]string{nil, {"/nonexistent/muxlight-test-binary"}} {
		code, err := RunWith(argv, in, &bytes.Buffer{}, func(pst.Report) {})
		if err == nil || code != 1 {
			t.Errorf("RunWith(%v) = %d, %v; want 1 and an error", argv, code, err)
		}
	}
}
