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

### PR: type predicates

- `add_case` with `line` (the `case "eq":` clause) and `where: "before"`
  put the new 24-line clause exactly there, indented to match, and named
  both the clause and the switch it found in its message. Clauses written
  without the switch's indentation came out right.
- `add_declaration` with `before` placed the test ahead of an existing one;
  `after` put the new builtin helper after `divide`. No refusals.
- gofmt realigned the `builtins` map after the predicates went in as their
  own block (a blank line before them keeps the old block's alignment).

### PR: self tail calls as loops (wasm)

- **One refusal, from guessing arguments:** `add_field` has no `comment`
  argument; the field's comment goes in `field` with its declaration
  (`"blocks int // ..."`), and the struct is `name`, not `type`. The
  refusal listed every argument with a description, so the fix took one
  try. (CLAUDE.md says to load schemas first; through the CLI there's no
  schema to load, and `refactor list` doesn't show arguments.)
- The 5-step `batch` (field, two `replace_statements` by line range, one by
  function, an `add_declaration` of three methods) went through after
  that, and moved both line ranges past the earlier steps: 483 to 488 and
  566 to 639. `inspect` afterwards confirmed both landed where intended.
- The batch noted that step 2 didn't type-check alone (it calls
  `tailExpr`, added in step 4) and re-ran itself with `check: "end"`,
  which saved a round trip.

## Tail calls between functions (stacked PRs)

### PR: frame replacement in the interpreter

- No refusals. An 8-step `batch` with `check: "end"` did it all:
  `replace_statements` with `function: Eval` rewrote the evaluator as a
  loop, `add_declaration` added the `Closure` type, its two methods and
  `asFunction`, `remove_dead_code` removed the old `closure` once nothing
  called it (and said so), and five `replace_text` steps fixed the
  `Function` checks in `apply`, `defmacro`, `equal` and printing.
- Finding those checks took a `grep` for `Function`; there's no "find
  every type assertion to this type" in the tools, which is what this
  change needed. `inspect` finds declarations and references to a name,
  but `fn.(Function)` is a reference to the type among many others
  (`Function(car)`, return types), so the list would be long.
- `replace_text` reports the line it changed, which here was a line of
  the edit's surroundings rather than the removed one; the edit itself
  was right.

### PR: a trampoline for tail calls in compiled code

- **One refusal:** `replace_declaration` takes exactly one declaration, and
  I'd put the new `tailCall` method in the same content as the replaced
  `tailApply`. The message said so ("it has 2") and the batch rolled back
  the four steps before it. Splitting it into `replace_declaration` plus
  `add_declaration` with `after: "wasmFunc.tailApply"` went through.
  Letting `replace_declaration` take extra declarations to add after the
  replaced one would have saved the retry.

## More of the roadmap (stacked PRs)

### PR: the tour uses the new builtins

- No refusals: one `batch` of 23 `replace_text` steps on a non-Go file
  (`examples/tour.glisp`), built by a short script. For a section that
  goes away entirely, the script cut the old text out of the file between
  two markers, so `old` matched exactly without retyping it.

### PR: string functions

- **One refusal, and it caught a real clash:** the batch named the new
  builtin's Go function `str`, which the package already has (`token.go`'s
  `str(s string) StringToken`). The batch type-checks at the end, so it
  reported "cannot convert str (value of type func(s string) StringToken)"
  on the line an earlier step wrote, marked with `*`, and rolled all 11
  steps back. Renaming it `showJoined` fixed it. The first attempt failed
  the same way through the tests' use of `str`, with more errors listed.
- `create_file` of the two new files, `add_declaration` of the encoder's
  `MemoryCopy`, `add_field` and seven `replace_text` steps went through as
  one `batch`, built by a script from files in a scratch directory (the
  `create_file` content-from-a-file gap again).

### PR: line numbers in errors

- The biggest batch yet, 38 steps across both backends, the AST and the
  tokenizer, built by a script. Three refusals, each rolling everything
  back and naming the problem:
  1. `replace_text` on `eval.go` didn't match: the `Apply` case is inside
     a `for`, one tab deeper than I'd written. Reading the line with
     `grep -n` showed the real indentation.
  2. `remove_dead_code` (the last step) refused because the batch didn't
     type-check: `ast_test.go` builds `Apply` with unkeyed literals, which
     the new `Line` field breaks. The message listed all three with their
     positions; keying them with `replace_text` fixed it. (`add_field`
     alone refuses up front on unkeyed literals; inside a batch with
     `check: "end"` the error surfaced at the end, on a different step.)
  3. None: after that the batch applied, and a test run found a panic in
     the new `readLines` (taking `&t[0]` of an empty list), fixed with one
     `replace_text`.
- `replace_declaration` swapped `tokenizeList` for a one-line wrapper and
  `add_declaration` added the tokenizer type with its methods after it;
  `remove_dead_code` took out the old `recoverError` once nothing used it.

### PR: a garbage collector for compiled code

- An 18-step `batch` restructured the compiler: `create_file` for the
  collector, seven `replace_declaration` steps (`wasmFunc` and six of its
  methods), two `add_declaration` steps, and `replace_text` for the rest.
  It applied at the first try once the branch was right.
- **The one refusal was a stale base:** I had branched from `master` before
  the error-lines PR merged, so `replace_text` couldn't find the `gLine`
  global the batch built on. The message named the step and file;
  re-branching from that PR's branch and rerunning the same batch file
  worked. Nothing in the tools could have known.
- The `replace_declaration`-takes-one-declaration limit came up again;
  this time I planned for it (new helpers via `add_declaration` with
  `after`).
- Measuring took more time than editing: three throwaway probe tests
  (thresholds, a GC counter exported through a temporary global, compile
  vs. run timing), each edited in with `sed`/Python and restored by
  copying the file back. A tool for "apply this edit, run this, revert"
  would fit that loop; `undo` covers the revert only for edits made
  through the tools.

### PR: direct calls to builtins

- No refusals: `create_file` for the inline paths, then
  `replace_declaration` for `invoke`, in one `batch`.
- A name clash caught before sending: the new file wanted a helper called
  `field`, which `wasm_runtime.go` already declares. I renamed it
  (`pairField`) by hand in the scratch copy; the batch's type check would
  have caught it too, as it did for `str` in the strings PR. A tool that
  answers "is this name free in the package?" would be handy before writing
  a new file.
- `replace_declaration` replaced a test function as easily as code.
- The 9-step batch otherwise applied as planned: constants and globals by
  `replace_text`, the body of `invoke` by line range, its locals and
  `define` call by `replace_text`.

### PR: roadmap of missing abstractions

- Docs only: appended with a heredoc, not through the tools (README
  isn't Go, and `replace_text` needs an anchor; appending at the end
  has none). `create_file`/`replace_text` with an "append" mode would
  cover it.

### PR: structured wasm generation (`wasmBody`)

- Nine batches, about 120 steps, every one checked by hashing the compiled
  module of each parity program and both examples before and after: the
  bytes never changed. That check, a throwaway test outside the commit,
  was the real guard for this PR; the type check alone let through one
  behavior change (below).
- Real refactorings did the part they fit: `rename` (`define` →
  `defineCode` → `setBody`) and `function_to_method` for 13 helpers
  (113 calls updated), once `replace_text` with `all_occurrences` had
  changed their parameter from `*wasm.Code` to `*wasmBody`.
- The rest was edits computed by Python scripts and sent as
  `replace_declaration`/`replace_text` steps: constants of local indices
  into `b.param`/`b.local` calls (typed from the separate `define` list),
  and `If … Else … End` / `Block Loop … BrIf … Br End End` sequences
  wrapped into `ifElse`/`when`/`until` closures. No tool wraps a run of
  statements into a function literal passed to a call; that "wrap in a
  closure" edit was most of the PR.
- One mistake the type check couldn't see: dropping each switch case's
  final `return` let the cases run on into the `panic` after the switch.
  It compiled; the hash test caught it ("no builtin car"). Fixed by moving
  the panic into `default:`.
- `replace_text` refused a step whose text occurred twice (two identical
  `when(fail)` blocks); `all_occurrences` fixed it, as the refusal said.
- Appending this entry and the roadmap section in the previous PR was by
  hand (heredoc): nothing appends to a file.

### PR: a builtin, defined once (`builtinSpecs`)

- Tests first in the same batch as the code (`check: "end"`): the new
  messages in `glisp_test.go` and ten parity programs; the interpreter
  side passed at once and the parity failures then listed exactly the
  wasm work left.
- `create_file` for builtins.go; `replace_declaration` for 18 interpreter
  operations and both wasm builtin functions; `remove_dead_code` took the
  old checkers (`listParam`, `number`, `expectArgs`, `stringArg`, the
  wasm `numBody`/`listArgBody`/`strArgBody` and `builtinNames`) with their
  comments, once `replace_text` had dropped their declare/define lines
  and struct fields.
- One bug the type check couldn't see: the wasm kind check walked
  `spec.kinds` rather than the arguments, so the second argument of `<`
  and `string<` (whose spec gives one kind for both) went unchecked. The
  parity tests caught it.
- A name I nearly used for a package-level variable, `strings`, would
  have clashed with the import in other files; caught while drafting.
  The "is this name free?" check from the last PR's notes again.
- By hand: this entry (heredoc append).

### PR: value kinds (`tagKinds`)

- One 12-step batch: `create_file` for wasm_kinds.go, `replace_text` to
  take out the old `objSize`, `putType`, `kind.tags`, the predicates' case
  and the collector's per-tag scan, and to make `builtinSpecs` an
  `append` with the type predicates; `add_declaration` for the predicate
  table. It type-checked and passed the first time.
- Removing the old `objSizeBody` and `putTypeBody` had to go by
  `replace_text` of their whole text, comments included: their new
  versions are in another file, so `remove_dead_code` (which needs them
  unused) and `move_declaration` (which moves them unchanged) didn't fit.
  "Replace this declaration with one in another file" would.
- The regexes that found those old texts for the batch were the fragile
  part; a step addressing a declaration by name and dropping it
  regardless of uses (`delete_declaration`) would have made them
  unnecessary.
- By hand: this entry.

### PR: the calling convention (`stackCall`)

- Two batches (17 and 4 steps), each followed by hashing every compiled
  module: identical to the commit before, so a pure refactoring.
- The batch was refused once more for a `replace_declaration` whose
  content held a new helper beside the replaced function (slotStore with
  slotSet, pushCall with stackCall and its methods): "content must be one
  declaration". A script now splits such steps into `replace_declaration`
  plus `add_declaration` with `after`; the tool could do that split
  itself.
- `compileLambda` changed by substring replacements inside its text
  (computed in Python, sent as one `replace_declaration`); an edit
  addressed to "these statements in this function" would have been
  smaller.
- By hand: this entry.

### PR: one memory-growth policy (`reserve`)

- One batch of 9 steps (`add_declaration` for `reserve`, `replace_text`
  for the three growth blocks, the declaration and the struct field), then
  a one-step fix: I had inverted the test after the doubling grow
  (`I32Eq` for `I32Ne`). It type-checked and every test passed; only the
  doubling test, had it reached that path, would have seen it. Writing
  the new test case first would have caught it at once.
- The first program I wrote for the test expected the wrong value: I
  thought `apply` spread a list into arguments (it doesn't, in either
  backend). The mismatch looked like memory corruption until I worked out
  the printed length. The second program didn't exercise the change
  (memory had already doubled past what the string needed); checking the
  test against the old code (`git stash` of the two source files) showed
  that, and the third program fails on the old code and passes on the new.
- By hand: this entry.

### PR: a parity check (`assertSameOutput`), and item 6 set aside

- One 14-step batch. The first try was refused at step 2: step 1 had
  replaced `runCompiled`, whose old body was the only user of the `sys`
  import, so the tool removed the import, and the new helpers in step 2
  needed it back. The refusal named `imports`; passing the two import
  paths and putting the helpers first fixed it. Tidying imports after
  every step is right on its own, but in a batch checked at its end it
  could wait for the end.
- `fix_imports` ran by itself at the end and removed the five imports
  wasm_gc_test.go no longer used.
- By hand: this entry, and a throwaway test (deleted) to see what a parity
  failure now reports.

### PR: `apply` spreads its last argument

- Read only through `refactor inspect` (`names`, a declaration at a time)
  and Read for the ends of README and this file; one Grep for the uses of
  `(apply` in tests and examples.
- One 16-step batch, tests first (`check: "end"`), passed the first time,
  including the parity and collect-at-every-call tests: `replace_text`
  for the test changes and the wasm `apply` case, `add_declaration` for
  the shared `checkKind` (both backends) and `push`, `replace_declaration`
  for the interpreter's `apply`. `pushCall` now pushes through `push`.
- This entry was appended with `replace_text` anchored on the last entry's
  final lines, not a heredoc: the tools can append after all, given an
  anchor.

### PR: rest parameters

- Two batches. First a refactoring: `arity` (min, max) out of `builtin`,
  embedded so `b.min` and `b.expects()` kept working; 29 steps, most of
  them one `replace_text` per spec literal (`1, 1,` to `arity{1, 1},`),
  generated from `inspect`'s listing of `builtinSpecs`. The first try
  generated 6 steps, not 29: my script misread `inspect`'s line-number
  prefix, and the batch was refused with the type errors of the literals
  it hadn't changed. Nothing applied, as a refusal promises.
- Then the feature, tests first, 21 steps. Refused once more for a
  `replace_declaration` holding three declarations (`Lambda.String` with
  the new `paramList` and `arity`); the splitting script now lives in the
  scratchpad and runs on every batch. The third time this has come up:
  `replace_declaration` taking extra declarations, or a `split` option,
  is the tool change it asks for.
- Reading was all `inspect` (names, and start/end lines in tests) and Read
  for the file tails these docs are anchored on.
- Replacing `bind` removed eval.go's `strings` import, and `fix_imports`
  removed wasm_compile.go's at the end, without being asked.

### PR: `not`, `and`, `or`, `cond`

- Glisp, not Go: the stdlib grew by `replace_text` anchored on its last
  line, and the tests (a table in glisp_test.go, the same programs as
  parity programs) by `add_declaration` and `replace_text`. One batch of
  three steps, which passed first time.
- Read stdlib.glisp with Read: `inspect` reads Go only.

### PR: `error`

- One 5-step batch, tests first: `add_declaration` for the test and for
  `raise`, `replace_text` for the spec row, the wasm case and the parity
  programs. The only failure was my test's expected line (3, where the
  call to `check` is; the error is reported at 2, the `error` call inside
  it, as documented), fixed with one `replace_text`.

### PR: the tour uses the new language

- 32 `replace_text` steps on examples/tour.glisp in one batch: 20 of them
  converted the tour's flat `cond`s to clauses, generated by a script
  that matched parentheses and kept each form's line breaks; the rest
  removed the tour's own `not`/`and`/`or`/`cond`, folded `list1`...`list4`
  into one variadic `list` (`all_occurrences`), and used `apply` and
  `error`. The tour printed exactly what it did before, plus one new line.
- Nothing structural for glisp source exists in the tools, which only
  parse Go: the `cond` rewrite was text replacement computed outside them.

### PR: compile only the stdlib a program uses

- Measured before deciding (a throwaway benchmark, outside the commit):
  the plan was to load the stdlib once per process, but the interpreter's
  load costs 41us; wazero compiling each module (17.6ms for 25KB) was the
  cost, so the fix became leaving unused stdlib out of the module.
- One 3-step batch: the test and `usedDefs`/`globalRefs` by
  `add_declaration`, the call site by `replace_text`.
- To show the test fails without the change, `replace_statements` with
  `function` stubbed `usedDefs` to `return stdlib`; the test failed, and
  `refactor undo` restored it. A quick, clean "does this test test
  anything" loop.
- My own mistake outside the tools: timing master against the branch with
  uncommitted changes, `git stash` and `checkout`, I popped the stash onto
  master and timed the change twice. Committing first fixed it.

### PR: 63-bit integers in the interpreter

- One 10-step batch, tests first: `add_declaration` for the test table and
  `int63`, `replace_text` for each place an integer is made (a literal,
  `parse_number`, and the results of `plus`, `-`, `*`, `/`), with enough
  of the surrounding lines to be unique.
- The only failure was an expected value I worked out wrong
  (3037000500^2 wraps to 145474192); the parity test, which compares with
  compiled code, had it right. One `replace_text`.

### PR: vectors

- One 20-step batch, tests first, passed the first time: `create_file`
  for vectors.go and wasm_vectors.go, `add_declaration` for the test,
  and `replace_text` for the rest (the kind tables, spec rows, `Write`,
  `typeName`, `equal`, the tag, the `tagKinds` row, and the wasm print,
  equality and dispatch cases).
- The new tag collided with the collector's forwarding marker (10); the
  marker moved to 255. Nothing tied the two together but the numbers: an
  enum for the tags with the marker outside it would make that visible.

### PR: the maps roadmap

- Docs only, one `replace_text` per file through `refactor batch`.

### PR: the parallelism roadmap

- Docs only, one `replace_text` per file through `refactor batch`.

### PR: one hash for both backends

- One 9-step batch, tests first: `create_file` for hash.go and
  wasm_hash.go, `add_declaration` for the test, `replace_text` for the
  spec row, the runtime's declaration, definition, field and dispatch.
- Refused once at the end of the batch: `int32(fnvOffset)` of a typed
  constant that overflows int32 is a compile error in Go; the refusal
  named the line (in a step's own text, marked `*`). Going through a
  variable wraps it, as intended.
- The expected values come from a Python script implementing the
  encoding separately (not committed); my test then used `list`, which
  only the tour defines. One `replace_text`.

### PR: CHAMP maps in the interpreter

- One 20-step batch: `create_file` for maps.go, maps_test.go and
  wasm_maps.go (the placeholder), `replace_text` for `hash` taking the
  builtin's name for its error, the kind, spec rows, `Write`,
  `typeName`, `equal`, and the wasm `tagIn` and dispatch.
- One test failed: `reflect.DeepEqual` of two tries that had the same
  shape but empty slices in one where the other had nil. The test now
  compares the tries written out (`shape`); a second batch, which also
  dropped the `reflect` import on its own.

### PR: CHAMP maps in compiled code

- One 22-step batch: the parity programs, the tags, the `tagKinds` rows,
  the encoder's `i32.popcnt`, the runtime's declarations, dispatch,
  printing and equality cases, and `hashInto` taking the builtin's name,
  all by `replace_text`. wasm_maps.go replaced the placeholder of the
  same name: `create_file` refuses an existing file, so the new one went
  in as wasm_maps2.go, then `delete_file` the old and `move_file` the new
  into its place. A `create_file` that replaces, or a `replace_file`,
  would have been one step.
- It passed the first time; I checked a 3,000-step program by hand
  outside the tests too (interpreter and wazero output byte for byte
  alike), since 700 lines of hand-written trie code passing at once was
  more than I trusted.

### PR: Language spec

- Read through `inspect` (the builtin table, `Eval`, the expander, the
  tokenizer, the printer) and probed edge cases with the command.
- Three batches: `create_file` for spec_test.go (test first, failing on
  the missing spec), then the spec, the test's `compiledPrints` by
  `add_declaration` and a `replace_text` to call it, then `replace_text`
  for fixes to the spec's shell transcripts, this entry and README.
- One batch was lost to the shell, not the tool: an unquoted heredoc
  ran the backticks in the Go content as commands. The JSON is now
  written from a quoted heredoc.
- The spec's examples found one difference between the harness and the
  command: an error found while compiling came back as
  `compile error: ...`, where `glisp -wasm` prints `glisp: ...`.

### PR: CLAUDE.md from the example

- CLAUDE.md rewritten with one `replace_text` of the whole file: no tool
  replaces a file's content (create_file refuses an existing one), so
  `old` was the file as it stood. A `create_file` that may overwrite, or
  a `replace_file`, would be the step.

### PR: Roadmap for functional programming

- README only, one `replace_text` anchored on the next section's
  heading. Nothing by hand.

### PR: `hash_map`, and a list library

- Test first: the spec's new list examples, by `replace_text`, failed
  on `list`. Then one 22-step batch of `replace_text`: the stdlib, the
  builtin's name in the spec table, its error messages and the wasm
  dispatch, the tests (`all_occurrences` on `(map `), the spec and
  README. A second batch for `(map)` with no arguments, which the
  first pattern missed; a rename of a builtin that is named in strings
  of glisp source has no tool, so each spelling is a text replacement.
- The examples' own list functions removed with `replace_text` of each
  block; their output compared with master's by hand, in both backends.

### PR: Algebraic data types

- Refactorings first, by the tools: `add_parameter` gave `newVector` a
  tag and `replace_expression` used it; `extract_function` pulled the
  vector element loops out of the printer and the hash, and
  `function_to_method` made them methods (the hash's has eight
  parameters, as the tool warned: the locals of the function it came
  from); `extract_function` and `add_parameter` made `objectOfArgs`, the
  body of `vector`, take a tag. `extract_function` also split
  `Expander.Expand` into `expand` and the wrapper that turns a `begin`
  into a call; `add_case` added `begin` to the special forms.
- `replace_expression` was refused once for an argument named `old`
  (it is `expression`); the refusal listed the arguments.
- The rest by `replace_text`, `create_file` and `add_declaration`, test
  first: the spec's examples and the parity programs. Nothing by hand.

### PR: Pattern matching

- All in glisp: the spec's examples and parity programs first, then the
  macro in stdlib.glisp, each one `replace_text`. Nothing by hand, and
  no Go changed: the macro needed nothing the language lacked.
