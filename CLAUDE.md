# glisp

A small Lisp in Go with two backends that must agree: the interpreter
(`Eval`) and a compiler to WebAssembly for WASI (`wasm_*.go`, with the
binary encoder in `wasm/`). The language is specified in
[docs/spec.md](docs/spec.md).

## How we work

Work test first: write the test you need next, make it pass with the
least code, rerun the tests.

Make every change in two phases: make the change easy, then make the
easy change.
1. Refactor first, with the gorefactor tools, until the change is small
   and local: rename to what a thing will mean, extract what you will
   change, add the parameter or field the change needs. Tests pass after
   every step.
2. Then make the behaviour change, with the edit tools
   (replace_statements, insert_statements, add_declaration, create_file).

The backends print the same thing for every program, errors included.
New behaviour gets a case in `wasmPrograms` (wasm_compile_test.go), which
runs it in both and compares the output, also with a collection at nearly
every call. A change to the language updates docs/spec.md, with an
example and its output: `TestSpecExamples` runs each in both backends.

One branch and one PR per item; the user merges. Each PR updates the
roadmap in README.md (check the item off, saying what changed) and
appends an entry to docs/friction.md. There is no CI: before pushing, run

    gofmt -l .       # must print nothing
    go vet ./...
    go test ./...

## Work as a derivation

Treat the code as a structure you reshape with operations. Before
editing, write the change as a list of operations that keep behaviour,
ending in the one small edit that changes it, and send it as one
`batch`. Where no operation does a step, keep the hand edit as small as
it can be.

Moves that work:
- **Near-duplicates:** extract_function from one copy, then `fold` the
  others onto it. Copies that differ only in constants: `parameterize`.
- **Room in a loop:** extract_function the loop together with what it
  builds; the change is then one guard line.
- **Change a representation:** put a getter or setter in front of every
  use (add_declaration, then `fold`), change the representation inside
  it, then remove_dead_code the old field.
- **Move behaviour or change a signature (expand, migrate, contract):**
  add_declaration the new function as a wrapper over the old one,
  `fold` the callers onto it, inline_function the old one with
  all_calls, then rename. For one parameter or result, add_parameter,
  remove_parameter, change_parameter_type or add_result do it in one
  step.
- **A field or function a new test uses** is stubbed for you.

## Friction log

Append one entry per PR at the end of [docs/friction.md](docs/friction.md),
never between older entries, headed `### PR: <title>`: how the change went
through the tools, what you did by hand and why no tool did it (none
exists, one refused and why, or its argument was missing), and the
operation that would have done it. Log refusals that cost a turn too.
The log is the evidence for which gorefactor tool to build next, so
record the gap, not only the workaround.

## Using gorefactor

`.mcp.json` runs `refactor-mcp@latest`; a session keeps the build it
started with, and `@latest` can lag a fresh commit (`go install` the
pseudo-version). Without the server (a cloud session), `refactor`
(`cmd/refactor`) runs the same tools: `refactor batch @steps.json`,
`refactor inspect '{"names":["Eval"]}'`, `refactor undo`. Write the JSON
from a quoted heredoc, so the shell leaves Go source alone.

- Load every gorefactor schema you will need in one ToolSearch.
- Read Go with `inspect`: a file first (no bodies), then only what you
  will change, by `names` (`["Eval", "Map.put"]`) or `targets`. Don't
  search before a refactoring; it finds every reference itself.
- Plan from one read and send one batch, with `"report": "summary"`, and
  `"check": "end"` when a test comes before the code it tests.
- Address declarations by name, not line (`Type.Method`, `function`,
  `after`/`before`); names stay valid after earlier steps.
- `replace_text` for string literals, comments and files that are not
  Go; to append to a file, anchor on its last lines.
- Read refusals: they say what to send. `undo` reverts a step or batch.

Known gaps (do these by hand and log them): nothing restores a file to an
earlier commit's version, and nothing replaces a whole file (create_file
refuses one that exists: use replace_text, or create the new one under
another name, delete_file the old and move_file the new into place).
