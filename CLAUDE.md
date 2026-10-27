# glisp

## Using gorefactor well

`.mcp.json` runs the published server (`go run …/cmd/refactor-mcp@latest`),
and a session keeps the build it started with: to pick up a new gorefactor
release, start a new session. `@latest` comes from the Go module proxy and
can lag a fresh commit; `go install` the exact pseudo-version
(`…@v0.0.0-<date>-<commit>`) when you need one sooner. Outside Claude Code,
`refactor` (`cmd/refactor`) runs the same tools from the shell, including
`check` and `undo`.

The tool names below assume Claude Code's `gorefactor` server; through the
desktop app's device bridge they are `mcp__remote-devices__gorefactor__…`.

1. **Load every schema at once.** The tools are deferred: load all the
   gorefactor tools you expect to use in one ToolSearch
   (`select:mcp__gorefactor__batch,mcp__gorefactor__replace_statements,...`).
   Guessed argument names cost a refusal and a turn.
2. **Read through inspect.** `inspect` of a file shows it without function
   bodies, with line numbers, for a third to a half of the whole file;
   then read the functions or line ranges you will change, several in one
   call with `targets`, or by name alone with `names`
   (`["Settle", "Ledger.Post"]`, no file). Avoid reading whole files
   with Read or `cat`: each result is sent again on every later turn,
   and whole files were over half of what reading cost in the A/B runs.
3. **Don't search before a refactoring.** rename, add_parameter,
   move_declaration and the others find every reference with the type
   checker.
4. **Plan from one read, send one batch.** Steps that build on each other
   go in one `batch` call, with line numbers as they were when you read
   them: the batch moves them past earlier steps. Use `"report": "summary"`
   when you don't need the diff, and `"check": "end"` when edits pass
   through code that does not compile yet (a test before the code it
   tests). Independent steps can instead be parallel calls in one turn,
   which costs no extra turn.
5. **Address by name, not line,** where a tool allows it: `file` + `name`
   for declarations (`Type.Method`, `Type.Field`), `replace_statements`
   with `function` to replace a whole body, `add_declaration` with
   `after`/`before`. Names stay valid after earlier steps; a reference such
   as `$prev.end` addresses what an earlier step of the batch made.
6. **Test first through the tools.** Add the test with `add_declaration`
   (or `create_file`). Functions and methods it calls (or passes as
   values), types it makes with keyed literals, and fields of the
   module's structs it uses, that nothing
   declares get stubs (`panic("not implemented")`) automatically, and
   `add_declaration` of the real one replaces the stub. An edit may start
   from code that does not type-check if it fixes some errors and adds
   none (`add_field` for a field the new test uses).
7. **replace_text for everything else:** string literals, comments, and
   files that are not Go (README, docs, JSON fixtures); `files` makes
   the same replacement in several files. `old` must match
   the file exactly, after gofmt's alignment: if it is not found, grep for
   the line as it is.
8. **Read refusals; they say what to send.** A refusal changes nothing. A
   range that cuts a statement is refused with the range to use. Every
   step can be undone with `undo`.

Known gaps (do these by hand and log them in [FRICTION.md](FRICTION.md)):
nothing restores a file.
remove_dead_code removes an unused struct field (`Type.Field`).
Refactorings change exported code and note when the module's API
changed. Files and directories move and go with
move_file and delete_file.
