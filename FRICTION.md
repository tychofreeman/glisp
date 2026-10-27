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

## Fixing the roadmap bugs (stacked PRs)

One entry per PR, written before committing it.

### PR: functions as values

- `add_case` with only `line` (the `switch`) put the new clause **after
  `default:`**. That's what its description says ("at the end of the
  switch"), but a clause after `default` reads badly; `where: "before"` at the
  `default` line put it in the right place.
- `add_case` indented the clause body by appending the content's own
  indentation to the file's: content written with a tab came out as
  4 spaces + tab (`    \treturn value`). Writing the content with 4 spaces
  came out right. Same family as the `replace_statements` tab issue above, but
  the opposite rule (`replace_statements` re-indents with tabs).
- `batch` with `"report": "summary"` worked well: three dependent edits in one
  call, names (`List.Eval`, `ProcessTokens`) stayed valid across steps.

### PR: lexical scope

- `replace_statements` indents differently depending on how the statements
  are named: with `function` (replacing `define_`'s whole body) it used tabs,
  in a file indented with spaces; with `start_line`/`end_line` (inside
  `Parse`) the replacement matched the file's 4-space indentation.
- Otherwise smooth: a four-step `batch` (`add_declaration` with `before`,
  `replace_statements` by lines and by function, `replace_text`) applied in one
  call, and the line range I read before the batch was correctly moved past the
  18 lines the first step inserted.

### PR: lambda arity

- `insert_statements` added the `strings` import itself (and said so in a
  note), but indented it with a tab among imports indented with 4 spaces.
  `add_declaration` likewise added `fmt` and `strings` to the test file
  without being asked, which was convenient.
- `insert_statements` kept the content's own 4-space indentation, like
  `add_declaration` and line-range `replace_statements`, unlike
  `replace_statements` by `function`.
- No refusals. `replace_text` was the right tool for the one-line macro change:
  the same `return b(macroScope, macroParams)` appears twice, so `old` needed
  the `case Function:` line above it to be unique.

### PR: eq on lists

- Same import issue as the last PR: `add_declaration` added `reflect` with a
  tab among space-indented imports; fixed with `replace_text`.
- Nothing else: `add_declaration` with `after: "eq"` placed the helper right
  after `eq`, and `replace_text` changed the one comparison.

### PR: silent wrong answers (`plus`, `car`, `cdr`)

- The new test used an `IsNil` matcher that go-matchers doesn't have.
  `add_declaration` quietly turned into a 2-step batch and declared a stub
  `IsNil` (`panic("not implemented")`) — in **`glisp.go`**, the production
  file, not the test file that used it. The output's only hint was "APPLIED
  batch: 2 steps"; the tests then failed with "panic: not implemented".
  Supplying the real `IsNil` with `add_declaration` on `glisp_test.go`
  replaced the stub and left it in the test file, which is the right outcome,
  but the stub should have gone in the test file to begin with (it's only
  used from `_test.go`), and the result should name what it stubbed.
- Writing nested lines of `replace_statements` content with tabs (since it
  re-indents with tabs when given `function`) avoided mixed indentation this
  time.

### PR: parens inside strings

- No tool issues. `replace_statements` by line range replaced the tokenizer's
  whole `if` chain (one statement) and kept the file's indentation. Passing Go
  rune literals like `'"'` inside JSON inside a shell argument took awkward
  quoting; that's the shell, not the tool, and `batch @file` avoids it.

### PR: operator characters as symbols

- No tool issues. `replace_expression` changed just the condition of one
  `else if`, so nothing else in the chain had to be restated. Its argument
  for the old text isn't named in `refactor list`'s description ("its source
  text"); I guessed `expression` and it was right, but naming the arguments in
  the description would remove the guess.

### PR: unterminated strings

- The added `fmt` import was tab-indented among space-indented imports again
  (third time); fixed with `replace_text`. Worth fixing in the tool: match the
  import block's existing indentation.
- Otherwise none: `insert_statements` before a line put the check exactly
  where wanted.

### PR: non-ASCII text

- No tool issues: a one-line `replace_statements` by line range swapped the
  byte read for `bs.ReadRune()`.

### PR: unbalanced parens

- **Batch refused after a rename.** A 9-step batch began with `rename`
  (`Tokenize` → `tokenizeList`) and then addressed line 41 — the recursive
  call — with `replace_expression`. The rename had rewritten that line (only
  the identifier changed), so the batch refused: "line 41 … was rewritten by
  step 1, so it is not there to address". The refusal was clear and rolled
  everything back. Workaround: rename and `add_parameter` in one call, then a
  fresh `inspect` and the remaining steps in a second call. It would help if a
  line whose only change was a rename stayed addressable.
- **`add_parameter` didn't flag shadowing.** The new parameter `nested` was
  shadowed by an existing local `nested := …` in one branch of the body. Legal
  Go, and the body didn't use the parameter yet, but a note would have saved a
  read; I removed the local with `replace_statements`.
- **Helpful:** `rename` noted that renaming the exported `Tokenize` changes the
  module's API (so I re-added `Tokenize` as a wrapper); `batch` noticed that
  step 2 used a variable step 3 declares, and re-ran with `check: "end"` on its
  own.
- **Correct refusal:** `move_statement_up` refused to put the
  unterminated-string check above the missing-paren check, because both panic
  and swapping them changes which error wins — which was the point.
  `replace_statements` over both did it.
- The new checks exposed two existing tests whose input was missing a `)`
  (`TestCdrGrabsTail`, `TestAtomIsFalseForComplexExpres`) besides
  `TestApplyCallsFunctions`; fixed with `replace_text`.

## Working through the rest of the roadmap (stacked PRs)

### PR: gofmt

- gorefactor has no formatting tool, so this one is plain `gofmt -w .`.
  That's the right tool, but it means the "whitespace" category of friction
  above (tabs vs. the file's 4 spaces, mixed indentation, tab-indented
  imports) was really a mismatch with an un-gofmt'd codebase: the tools
  write gofmt-style tabs. From here on the files are gofmt'd, so new edits
  should match. A `format` step (or `check` reporting files that aren't
  gofmt'd) would have caught it on the first edit.

### PR: dead code

- No tool issues. One four-step `batch`: `replace_statements` with empty
  `content` deleted each unreachable `return nil` (nothing in its description
  says empty content deletes, but it does — worth saying, since there's no
  separate "delete statements" tool); `remove_else` un-nested `atom`;
  `remove_dead_code` removed `ParamsList` and noted the exported-API change.
  Line numbers stayed valid across steps, and since the files are gofmt'd now,
  `gofmt -l` stayed clean after the edits.

### PR: Go-style names

- No tool issues. Having been refused once for addressing a line an earlier
  `rename` rewrote, I planned around it: the two locals were named by
  `function` (which the description says stays valid), and the function
  itself was renamed from its call site rather than its declaration line,
  which the local renames had rewritten. All eight steps (six renames, two
  `remove_dead_code`) went through in one `batch`.
- Small wish: `remove_dead_code` takes extra names to remove "with it", but
  the argument for them isn't named in `refactor list`; two separate steps
  were simpler than guessing.

### PR: `ioutil` and the `macro` message

- No tool issues, and one nice touch: `replace_expression` swapping
  `ioutil.ReadFile` for `os.ReadFile` added the `os` import *and* removed
  `io/ioutil` because nothing else used it — and with the file gofmt'd, the
  import came out correctly indented this time.
- Writing `\n` inside a Go string, inside JSON, inside a shell heredoc took
  `\\\\n` in the heredoc; again the shell, not the tool.
