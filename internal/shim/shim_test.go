package shim

import (
	"bytes"
	"io"
	"testing"

	"github.com/adamvduke/pst"
)

// fakePTY replays output in fixed chunks and records what the shim writes
// back to the program.
type fakePTY struct {
	chunks [][]byte
	input  bytes.Buffer
}

func (f *fakePTY) Read(p []byte) (int, error) {
	if len(f.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.chunks[0])
	f.chunks = f.chunks[1:]
	return n, nil
}

func (f *fakePTY) Write(p []byte) (int, error) { return f.input.Write(p) }

func TestPumpAnswersQueryAndPassesBytesThrough(t *testing.T) {
	// Query split across reads, then a report, as a real program might write.
	stream := []byte("\x1b]75" + "01;?\x07\x1b[c" + "hello\x1b]7501;state=working:app=claude-code\x1b\\")
	f := &fakePTY{chunks: [][]byte{stream[:4], stream[4:12], stream[12:]}}
	var out bytes.Buffer
	var reports []pst.Report

	pump(f, &out, func(r pst.Report) { reports = append(reports, r) })

	if !bytes.Equal(out.Bytes(), stream) {
		t.Errorf("output changed:\n got %q\nwant %q", out.Bytes(), stream)
	}
	if got := f.input.String(); got != string(queryReply) {
		t.Errorf("reply to program = %q, want %q", got, queryReply)
	}
	if len(reports) != 1 || reports[0].State != pst.StateWorking || reports[0].App != "claude-code" {
		t.Errorf("reports = %+v", reports)
	}
}

func TestPumpDropsInvalidReports(t *testing.T) {
	f := &fakePTY{chunks: [][]byte{
		[]byte("\x1b]7501;app=x\x07"),                 // no state: ignored
		[]byte("\x1b]7501;state=dancing\x07"),         // unknown state: ignored
		[]byte("\x1b]7501;state=idle:msg=a\x07"),      // undecodable base64: discarded
		[]byte("\x1b]7501;state=error:msg=!!!!\x07"),  // bad byte: pair skipped, report kept
		[]byte("\x1b]7501;state=done:msg=b2s=\x1b\\"), // valid: "ok"
	}}
	var reports []pst.Report
	pump(f, io.Discard, func(r pst.Report) { reports = append(reports, r) })
	if len(reports) != 2 || reports[0].State != pst.StateError || reports[0].Msg != "" ||
		reports[1].State != pst.StateDone || reports[1].Msg != "ok" {
		t.Fatalf("reports = %+v", reports)
	}
	if f.input.Len() != 0 {
		t.Fatalf("shim wrote %q to the program without a query", f.input.String())
	}
}
