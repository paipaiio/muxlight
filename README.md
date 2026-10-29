# muxlight

**OSC 7501 program status for tmux.** See which of your coding agents is working, waiting on you, or done, from any tmux pane.

> Status: prototype (week 1).

## Why

[OSC 7501](https://www.superlogical.com/rex/docs/build/program-status) lets a program tell its terminal whether it is `idle`, `working`, `blocked` on the user, `done`, or failed. Claude Code, Codex, opencode, and many other agents already speak it.

Inside tmux they stay silent. An agent checks for support by sending the query `OSC 7501 ; ?` followed by `CSI c`. tmux answers `CSI c` at once and never answers the query, so the agent decides there is no support. tmux has [declined native support for now](https://github.com/tmux/tmux/issues/5713).

`muxlight run` puts a thin pseudo-terminal between tmux and the agent. It answers the query before tmux answers `CSI c`, passes every byte through unchanged, and copies each status report into tmux pane options that your status line, pane borders, and scripts can read.

## Use

```sh
muxlight run claude          # inside tmux; outside tmux it just runs the command
muxlight ls                  # every pane with a status, most urgent first
muxlight clear [pane]        # forget a pane's status
```

```
PANE     STATE               APP          AGE  MESSAGE
work:2.0 blocked/permission  claude-code  12s  Run rm -rf build?
main:0.1 done                codex        3m   All 42 tests pass
```

Each pane gets these user options while its agent runs. `working` and `blocked` are dropped when the agent exits; `done` and `error` stay until you clear them.

| Option | Value |
|---|---|
| `@pst_state` | `idle`, `working`, `blocked`, `done`, `error` |
| `@pst_kind` | `permission`, `question`, `auth` (blocked only) |
| `@pst_app` | the program's id, e.g. `claude-code` |
| `@pst_msg` | one line of text; `#` is stored as `##` so it cannot inject tmux styles |
| `@pst_updated` | unix seconds of the last change |

Example `~/.tmux.conf`:

```tmux
set -g pane-border-status top
set -g pane-border-format ' #{pane_index} #{?#{==:#{@pst_state},blocked},#[fg=red]● needs you: #{@pst_msg},#{@pst_state}} '
bind-key a display-popup -E 'muxlight ls; read -r _'
```

## Build and test

```sh
go build -o bin/muxlight ./cmd/muxlight
go test -race ./...
./test/e2e/run.sh        # runs on a private tmux server, never touches yours
```

The end-to-end test uses `test/e2e/fake_agent.py`, which detects OSC 7501 the way real agents do. Without muxlight it finds no support inside tmux; under `muxlight run` it does, and the pane goes `working → blocked/permission → done`.

## Credits

Parsing and the record store come from [adamvduke/pst](https://github.com/adamvduke/pst) (MIT). Third-party licenses are listed in [NOTICE](NOTICE).

## License

[Apache License 2.0](LICENSE).
