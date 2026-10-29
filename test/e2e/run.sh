#!/bin/sh
# End-to-end test on a private tmux server. It never touches your own tmux
# sessions: everything runs on the socket named below, with a clean env.
#
#   1. control: the fake agent alone in tmux must find no OSC 7501 support,
#      because tmux answers CSI c and never answers the query;
#   2. under `muxlight run` it must find support, and the pane options must go
#      working -> blocked/permission -> done, with "#" neutralized;
#   3. `muxlight ls` must list the pane.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
muxlight="$root/bin/muxlight"
sock=muxlight-e2e
work=$(mktemp -d)
T="tmux -L $sock"
fail=0

check() { # name got want
	if [ "$2" = "$3" ]; then echo "  ok    $1 = $2"; else echo "  FAIL  $1 = '$2', want '$3'"; fail=1; fi
}
opt() { $T display -p -t "$pane" "#{$1}"; }

env -i HOME="$HOME" PATH="$PATH" USER="${USER:-}" LANG=en_US.UTF-8 TERM=xterm-256color \
	$T -f /dev/null new-session -d -s e2e -x 120 -y 30 -c "$work" 'sh'
trap '$T kill-server 2>/dev/null || true' EXIT
pane=$($T display -p -t e2e '#{pane_id}')
sleep 0.3

echo "control: fake agent without muxlight"
$T send-keys -t e2e "python3 -I '$here/fake_agent.py' '$work/control'" Enter
sleep 3
check "detection" "$(cat "$work/control")" "unsupported"

echo "under muxlight run"
$T send-keys -t e2e "'$muxlight' run python3 -I '$here/fake_agent.py' '$work/shim'" Enter
sleep 1.2
check "detection" "$(cat "$work/shim")" "supported"
check "state" "$(opt @pst_state)" "working"
check "app" "$(opt @pst_app)" "fake-agent"
sleep 2
check "state" "$(opt @pst_state)" "blocked"
check "kind" "$(opt @pst_kind)" "permission"
check "msg (raw option, # doubled)" "$(opt @pst_msg)" "Run rm -rf build? ##[fg=red]"
sleep 2.5
check "state after exit" "$(opt @pst_state)" "done"
check "kind cleared" "$(opt @pst_kind)" ""

echo "muxlight ls"
sockpath=$($T display -p '#{socket_path}')
listing=$(TMUX="$sockpath,0,0" "$muxlight" ls)
echo "$listing" | sed 's/^/    /'
case "$listing" in *"e2e:0.0"*done*fake-agent*"All 42 tests pass"*) echo "  ok    ls shows the pane" ;; *) echo "  FAIL  ls"; fail=1 ;; esac

exit $fail
