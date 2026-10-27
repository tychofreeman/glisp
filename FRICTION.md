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

### PR: a fresh scope per run

- No tool issues. A six-step `batch`: `rename` (`baseScope` →
  `builtinScope`, including its use in a test), `add_declaration` with `after`
  a package variable, three `replace_statements` by `function`, and
  `remove_parameter` dropping `ProcessTokens`'s now-unused `includeStdLib`
  and the `true` from both calls. Addressing by name throughout meant the
  rename's rewritten lines never got in the way.
- `add_declaration` added the `sync` import for the concurrency test,
  correctly indented now that the files are gofmt'd.
- `check` has no way to pass `-race`; I ran `go test -race` directly. A
  `race` (or general `test_flags`) option on `check` would keep it in the
  tool.

### PR: `Run` returns errors

- **A refusal pointed at an argument the tool doesn't take.** Adding the
  tests with `add_declaration` was refused because `RunFile`'s first result
  can't be inferred from `_, err := RunFile(…)`. The refusal said to "give
  them in results, e.g. {"RunFile": "(int, error)"}", but `add_declaration`
  rejected `results` as an unknown field. `results` belongs to
  `create_from_usage`. What worked: one `batch` with `check: "end"` —
  `add_declaration` of the tests, then `create_from_usage` with `results` and
  `to_file: "glisp.go"`. The hint should name `create_from_usage`, or
  `add_declaration` should accept `results` and pass it through.
- `create_from_usage` named the stubs' parameter `s` (the calls pass string
  literals); the real declarations then replaced the stubs, as documented,
  with a note for each.
- Otherwise smooth: `replace_text` to add a doc comment above `Process`,
  `replace_statements` by `function` for `cmd/glisp`'s `main`.

### PR: values at parse time

- No tool issues. `replace_expression` on the bare expression `node` in two
  `return node` lines did exactly that, and noted each type change
  (`NumberToken` → `int64`, `StringToken` → `string`) — a useful
  confirmation that the change meant what I thought.

### PR: comments

- No tool issues. `insert_statements` placed a whole `if` (with its own
  comment lines) before the tokenizer's `if` chain, which was simpler than
  restating the chain to add a branch. `replace_text` worked on the
  `.glisp` example as well as Go files.
- Writing Go rune literals (`';'`, `'\n'`) in JSON passed as a single-quoted
  shell argument needed `'"'"'` around each quote; `batch @file` from a
  heredoc or script is the better habit for anything with quotes.

### PR: `-`, `<` and `>`

- No tool issues. A five-step `batch`: `add_declaration` (a `number` helper,
  `minus`, `compare`) after `plus`, `replace_statements` to make `plus` use the
  helper, and three `add_element` steps putting `"-"`, `"<"` and `">"` into the
  `builtins` map, each `after` the previous key. `add_element`'s keys are
  written as in the source (`"\"plus\""`), which the description says.
- A seven-step `replace_text` batch updated the `.glisp` example.

### PR: `true`, `false` and `nil`

- No tool issues: three `add_element` steps (no `after`, so each went at the
  end of the `builtins` map), and gofmt-style alignment of the map came out
  right. One `replace_text` removed the example's own definitions.

### PR: `cons` onto `nil`

- No tool issues. `replace_statements` by `function` rewrote `cons`'s body;
  with the file gofmt'd, its tab indentation now matches.
- `replace_text` has an `all_occurrences` option (it's in the description; I
  hadn't noticed it before), which replaced the example's six uses of `empty`
  in one step.

### PR: macros

- **Two refusals removing the `isMacroScope` field, both correct and both
  told me what to do.** First: "an unkeyed literal of Scope … lists every
  field; give it keys first". Keying the three `&Scope{…}` literals with
  `replace_expression` fixed that. Second: removing the field would change
  what `%#v` prints in `Symbol.Eval`'s error, which passed the whole scope to
  `fmt`. That message dumped every definition in the run, so shortening it to
  `Cannot resolve symbol x` (another `replace_expression`, which also dropped
  the now-unused `fmt` import) was an improvement anyway; with it as step 1,
  the five-step batch went through.
- A wish: a tool to key an unkeyed composite literal (or `remove_dead_code`
  offering to do it) — three hand-written `replace_expression` steps had to
  restate each literal exactly.
- `replace_statements` with empty `content` deleted the `isMacroScope` check
  in `List.Eval`, as it did for the dead `return`s earlier.

### PR: more tests

- No tool issues. One `add_declaration` added seven tests; `batch` noticed
  that the first `replace_expression` called a helper the third step declares
  and re-ran with `check: "end"` by itself, as it did once before.

## Later changes

### PR: CLAUDE.md for glisp

- No tool issues: two `replace_text` steps in a `batch` edited the Markdown.
  `git checkout` was the quickest way to throw away a first attempt that
  changed more than intended (`undo` would have taken one call per step).

### PR: mutual recursion

- **`rename` refused to turn a disabled test back on**, correctly: renaming
  `THIS_FAILS_…_TestMutualRecursionFails` to `TestMutualRecursion` "would make
  go test run it as a test function". That was the point, but a rename is
  meant to keep behavior. `add_declaration` of the new tests plus
  `remove_dead_code` of the old function did it instead.
- `remove_dead_code` noted that removing the old function "changes the
  module's API" because its name is exported. It's in a `_test.go` file, so it
  was never part of the API; the note is a false positive there.
- Otherwise smooth: `replace_statements` by `function` and `replace_text` for
  the doc comment, in one `batch`.

### PR: `defn`

- No refusals. `add_declaration` + `remove_dead_code` replaced the disabled
  test (the same route as for mutual recursion; `remove_dead_code` again
  called removing a `_test.go` function an API change). `add_declaration`
  with `after`, `add_element` with `after "\"if\""`, and `replace_text` on
  `stdlib.glisp` added the feature in one `batch`.
- A 32-step `batch` of `replace_text` converted the example's definitions to
  `defn`. Each step needed the definition's exact text, so a script found the
  forms' extents; a structural "rewrite this s-expression" step would only
  make sense for Go, so that's no gap in a Go tool — just a note that
  non-Go bulk edits are text edits.

### PR: compiler roadmap

- No tool issues: one `replace_text` appended the section to the README.

## Compiler foundations

### PR: `Node` types

- **A name clash with a dot-imported package.** The tests dot-import
  go-matchers, which exports `Call`, so a `Call` node type doesn't compile in
  the test package. The batch's error said exactly that, and rolled back
  cleanly; the node is `Apply` instead.
- `rename` (old `Lambda` → `parsedLambda`) listed a comment that still said
  "Lambda", with its line — the cue to fix it with `replace_text`.
- **`create_file` gofmt'd the file it created**, realigning a map literal, so a
  later `replace_text` with `old` copied from my own source didn't match. The
  refusal's advice (grep for the line as it is) was right; replacing the two
  statements by line range was simpler. Worth knowing: text you hand to
  `create_file` or `add_declaration` may not be the text that lands.

### PR: the `Expander`

- No refusals. Two `create_file` steps added `expand.go` and its tests, and the
  batch added a third step on its own, `fix_imports`, for the `fmt` the test
  file used but I'd left out — reported as a step with a note, which is the
  right amount of visibility.

### PR: the interpreter on `Node`s

- No tool issues. A five-step `batch`: `create_file` for the evaluator,
  `replace_statements` by `function` for three bodies (including a method,
  `Expander.evalMacroBody`), and `replace_text` for a package variable's
  initializer. Every existing test passed on the first run, which is the
  point of this step: they now all go through the expander.

### PR: removing the old evaluator

- **An interface can't be removed together with its methods.** Removing the
  `Valuable` interface and the `Eval` methods that implemented it was refused
  three ways — `Valuable` under `also`, in its own step after the methods,
  and as the main name with the methods under `also` — each time with "the
  interface Valuable has a method Eval, which a type could implement through
  this one". Emptying the interface first with `replace_declaration` didn't
  work either: the next step needs code that type-checks, and `check: "end"`
  doesn't extend to refactoring steps (the refusal said so, clearly).
  What worked: delete the interface's only use (a `case Valuable:` in
  `GetValue`) with `replace_text`, then `remove_dead_code Valuable` alone,
  then everything else in one `remove_dead_code` with `also`. The check is
  right for removing *a method* while the interface stays; when the interface
  goes in the same step it shouldn't apply.
- `remove_dead_code`'s `also` isn't named in `refactor list`; an invalid
  argument got a refusal that listed the real ones, which is a decent way to
  ask, but the description could just say.
- Once it went through: 19 declarations and an import gone in one step, with
  the `builtins` map realigned after five `replace_text` deletions.
- `remove_dead_code`'s note that removing an exported name changes the API
  appeared 11 times for one step — one line listing the names would do.

### PR: the resolve pass

- **`add_field` refused, with exactly what to fix:** the three unkeyed
  `Lambda{…}` literals, with file and line. Keying them with
  `replace_expression` and `add_field` in one `batch` went through.
- **`create_file` of a test using an undefined `Resolve`** got the same
  refusal as `add_declaration` did earlier, again suggesting a `results`
  argument that `create_file` doesn't take. `create_from_usage` with
  `results` worked, but its `to_file` must already exist ("is not a file of
  package"), so the batch had to `create_file resolve.go` with just a package
  clause first. `to_file` creating the file would save a step.
- Otherwise smooth: `add_declaration` replaced the stub (and said so), and
  `replace_declaration` swapped two `String` methods.

### PR: `let`

- No tool issues: `add_declaration` + `remove_dead_code` replaced the last
  disabled test (the exported-name API note again, for a `_test.go`
  function), `replace_text` added the macro to `stdlib.glisp`, and
  `replace_statements` updated a test helper.

## WebAssembly backend (stacked PRs)

### PR: the wasm encoder package

- No refusals: one `batch` of three `create_file` steps made the new
  `wasm` package (encoder, instruction builder, test), gofmt-ed, and its
  test passed on the first run.
- `create_file` takes the file's content inline, so a 250-line file goes
  into JSON by hand. Wrote a small script that builds the `batch` file from
  files written in a scratch directory. A `content_file` (or `from`)
  argument naming a local file would remove the script.
- `go get` and `go mod tidy` wrote `go.mod` (outside gorefactor); merging
  its two `require` blocks was a `replace_text` on a non-Go file, which
  worked fine.

### PR: compiling to wasm

- Three `create_file` steps in one `batch` (compiler, runtime, test;
  ~1,700 lines) applied cleanly, and every test passed on the first run, so
  no edits were needed while debugging.
- `create_file` reported 627 and 890 lines for files I had guessed at 470
  and 620; the counts were right. Reporting the line count is useful.
- `replace_statements` with `function: main` rewrote `cmd/glisp`'s `main`
  for the `-wasm` flag and added the `flag` import itself (with a note), at
  the right indentation this time.
- `add_declaration` with `after` put the new test where I wanted it.
- Experiments (a larger stack, timing probes) were done with a throwaway
  `_test.go` file and `sed`, outside the tools, then restored; `undo` would
  have covered the constant, but not a file created and deleted by hand.

### PR: the tour example

- No gorefactor use: the PR adds one glisp file, written directly. (This
  entry went in with `replace_text`, which worked fine.)

### PR: doubling wasm memory

- No tool issues. The test went in first with `add_declaration` (`after`
  the last wasm test) and failed as expected; `replace_statements` with
  `function: wasmCompiler.allocBody` (a method, addressed as
  `Type.Method`) replaced the allocator's body, and two `replace_text`
  steps changed its locals and comment, in one `batch`.
- `inspect` with `names` showed just `allocBody`, with line numbers, which
  was all the reading this change needed.
- One refusal, my mistake: a `batch` anchored a `replace_text` on a
  FRICTION.md entry that was still on an unmerged branch. The refusal said
  exactly that ("old was not found") and rolled back the README step
  before it, so rerunning the same batch after merging was all it took.

## Printing, arithmetic, predicates and tail calls (stacked PRs)

### PR: readable printing

- This session was restarted partway through the PR, and the edits made
  before the restart were found uncommitted in the working tree; whatever
  tool friction came up then wasn't recorded. Nothing in gorefactor keeps
  a log of the edits it made, so there was no way to reconstruct it.
  `undo` history doesn't survive a restart either (each `refactor` call is
  its own process, though the CLI's on-disk undo would cover the last one).
- After the restart: `replace_text` for README and this entry; no issues.

### PR: `*`, `/`, `mod`, `<=`, `>=`

- No refusals. Tests first (one `batch`: `add_declaration` of two tests,
  `replace_text` adding programs to the wasm parity list; it failed with
  "Cannot resolve symbol *"), then one 8-step `batch` for both backends
  and the stdlib.
- Most of the wasm changes were `replace_text` on lines inside one big
  `switch` in `builtinBody` (a new case, widened case lists, one op
  choice). That's the right tool, but `add_case` would read better for
  the new `case "/", "mod":`; I didn't reach for it because placing it
  before an existing case needs that case's line, and I had read the
  file by `grep`, not `inspect`.
- A local branch `language/arithmetic` already existed (from the earlier
  `-`/`<`/`>` PR, merged and kept); `git checkout -b` refused, so this one
  is `language/more-arithmetic`. Not a gorefactor issue.
