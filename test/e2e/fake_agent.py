"""A fake coding agent that detects OSC 7501 the way real agents do.

It sends the query followed by CSI c. If the DA reply arrives before the
OSC 7501 reply, it concludes there is no support and reports nothing.
Otherwise it walks through working -> blocked -> done, pausing so a test can
look at the pane in each state. It writes "supported" or "unsupported" to the
file given as its first argument.
"""
import base64, os, select, sys, termios, time, tty

def report(state, msg, kind=None):
    body = f"state={state}:app=fake-agent:msg={base64.b64encode(msg.encode()).decode()}"
    if kind:
        body += f":kind={kind}"
    os.write(1, f"\x1b]7501;{body}\x1b\\".encode())

def detect(fd, timeout=2.0):
    os.write(1, b"\x1b]7501;?\x1b\\\x1b[c")
    got, end = b"", time.time() + timeout
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.1)
        if r:
            got += os.read(fd, 256)
        i, j = got.find(b"\x1b]7501;?"), got.find(b"c", got.find(b"\x1b["))
        if i >= 0 and (j < 0 or i < got.find(b"\x1b[")):
            return True
        if got.find(b"\x1b[") >= 0 and j >= 0:
            return False
    return False

fd = sys.stdin.fileno()
old = termios.tcgetattr(fd)
tty.setraw(fd)
try:
    supported = detect(fd)
finally:
    termios.tcsetattr(fd, termios.TCSADRAIN, old)
open(sys.argv[1], "w").write("supported" if supported else "unsupported")
if supported:
    report("working", "Refactoring the parser"); time.sleep(2)
    report("blocked", "Run rm -rf build? #[fg=red]", kind="permission"); time.sleep(2)
    report("done", "All 42 tests pass")
print("fake agent finished")
