# gorefactor friction log

Difficulties hit while using the go-refactor-mcp (`gorefactor`) tools on
glisp. Newest at the bottom; each entry says what happened, what we did
instead, and what would have helped.

## 2026-10-27

### The server can't be pointed at another module mid-session

The `gorefactor` server on the workstation was started with its root at the
go-refactor-mcp checkout. `refactor-mcp` reads its module directory once at
startup (`-C`, then `$CLAUDE_PROJECT_DIR`, then the current directory), and no
tool call changes it, so its tools could not be used on glisp at all.

- **Workaround:** cloned glisp into the cloud container and ran the tools there.
- **Would help:** a tool (or an `inspect` argument) to open another module,
  or one server able to serve several module roots.

### A `.mcp.json` added during a session isn't loaded

Adding `.mcp.json` to glisp did not make `mcp__gorefactor__*` tools appear;
MCP servers attach when a session starts.

- **Workaround:** installed `cmd/refactor` and `cmd/refactor-mcp` with
  `go install …@latest` and drove them from the shell.
- **Would help:** nothing in gorefactor itself; worth a line in the README's
  setup notes so users expect to start a new session.

### The `refactor` CLI has no `check`

`refactor list` has every edit and `inspect`, but not `check`, so there was no
way to type-check or run the tests through the CLI.

- **Workaround:** wrote a small stdio MCP client ([`tools/gr.py`](tools/gr.py)) that starts
  `refactor-mcp -C <module>`, sends `initialize`, calls one tool and prints the
  result; used it for `check {"run_tests": true}` and then for every edit.
- **Would help:** `refactor check [run_tests=true]` in the CLI, or a
  `refactor mcp-call <tool> '<json>'` passthrough.
- **Resolved** in go-refactor-mcp `3f4b7d8`: `refactor check [run_tests=true]
  [packages=./...]`.

### `undo` doesn't survive between calls

With one server process per call, each call starts with empty undo history,
and re-loads the module.

- **Workaround:** relied on `git diff` / `git checkout` instead of `undo`.
- **Would help:** the CLI keeping undo state on disk (e.g. under `.git/` or a
  cache dir), or a long-running mode.
- **Resolved** in go-refactor-mcp `3f4b7d8`: `refactor undo` reverts the last
  edit made by an earlier `refactor` process, and leaves no files in the
  repo.

### `replace_statements` re-indents with tabs; `add_declaration` doesn't

glisp is indented with 4 spaces (not gofmt'd). `add_declaration` inserted the
new test "as written", keeping 4 spaces. `replace_statements` on `define_`
re-indented the body with a tab per level but kept the spaces already in the
content, so the nested line came out as `\t    body = …`, mixed tabs and
spaces.

- **Workaround:** a follow-up `replace_text` changing `\t    ` to `\t\t`.
  `define_` is now tab-indented in a file that otherwise uses spaces.
- **Would help:** the two tools indenting the same way: either both gofmt the
  edited declaration, or both follow the file's existing indentation and
  re-indent content consistently (spaces in content treated as one level).

### Tool names in the copied `CLAUDE.md` don't match this setup

The "Using gorefactor well" section names tools `mcp__gorefactor__…` and says
the server runs `go run ./cmd/refactor-mcp` from "this checkout". Through the
desktop bridge they are `mcp__remote-devices__gorefactor__…`, and glisp's
`.mcp.json` runs `…/cmd/refactor-mcp@latest`.

- **Would help:** wording in that section that doesn't assume the server's own
  repo or a particular tool-name prefix.

### Needs Go 1.26 (not a problem in the end)

go-refactor-mcp requires Go 1.26; the container has 1.24.7. With
`GOTOOLCHAIN=auto` (the default) `go run`/`go install …@latest` fetched
go1.26.10 automatically, so no hand-built Go was needed. Worth noting in the
README for machines with `GOTOOLCHAIN=local`.

### Picking up a fresh go-refactor-mcp commit

After the fix above was pushed, `go install …@latest` still installed the old
build: the module proxy resolves `@latest` from a cached list. `@master` failed
(the branch is `main`), and `GOPROXY=direct` with the commit hash downloaded
the code but then couldn't switch to Go 1.26, because fetching the toolchain
also goes through `GOPROXY`.

- **Workaround:** install the exact pseudo-version through the normal proxy:
  `go install …/cmd/refactor@v0.0.0-20261027143145-3f4b7d8dbecc`.
- **Would help:** tagging releases, so `@latest` (and `.mcp.json`'s
  `go run …@latest`) moves when the tools change; `refactor -version` /
  `refactor-mcp -version` reporting the commit, since both still print 0.2.0.
