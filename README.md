# glisp

A small Lisp interpreter written in Go.

```
go run ./cmd/glisp examples/numbers.glisp
```

runs a program and prints each `p` call, then the value of the last
expression. [`examples/numbers.glisp`](examples/numbers.glisp) builds
map, filter, fold, sorting, primes and more out of the builtins.

Builtins: `quote car cdr atom cons plus if eq apply def defmacro p`, plus
`lambda`. [`stdlib.glisp`](stdlib.glisp) is embedded in the package and
loaded before every program; it defines `mult`.

## Roadmap

### Bugs

- [x] `def` stores the unevaluated expression, so `(def y (plus 1 2)) (plus y 1)`
  returns 1.
- [x] Functions can't be used as values: `GetValue` has no case for `Function`,
  so a lambda passed inline as an argument, or returned from a function, panics.
  Every function passed to `map`/`filter`/`fold` has to be a named `def`.
- [x] Scoping is dynamic, not lexical: a lambda body runs in its caller's scope,
  so closures don't capture variables, and a free variable can pick up a
  caller's parameter of the same name.
- [x] Calling a lambda with too few arguments panics (index out of range); extra
  arguments are silently dropped.
- [x] `eq` on two lists crashes the Go runtime ("comparing uncomparable type").
- [x] Silent wrong answers: `plus` skips non-integers (`(plus 1 "a")` is 1), and
  `car`/`cdr` of a non-list return nil.
- [x] Tokenizer:
  - [x] `(` or `)` inside a string splits the string.
  - [x] Characters other than letters, digits, `- _ ?` and `"` are dropped as
    separators, so `(+ 1 2)` reads as `(1 2)`.
  - [x] A lone `"` panics.
  - [x] Input is read byte by byte, so non-ASCII text is mangled
    (`héllo` becomes `hÃ©llo`).
  - [x] An unmatched `)` ends parsing: `(plus 1 2)) (plus 10 10)` returns 3.
- [x] Evaluating an empty list panicked.
- [x] `stdlib.glisp` was read from a hardcoded path.

### Language

- [x] Comments: `;` to the end of the line.
- [x] Subtraction and comparison builtins (`-`, `<`, `>`). Programs built them
  by counting up from 0, which was very slow (Collatz steps for 27 took about
  a minute; it now takes well under a second).
- [x] `true`, `false` and `nil` literals; programs defined them as
  `(eq 1 1)`, `(eq 1 2)` and `(car (quote ()))`.
- [x] `cons` onto `nil` gives `(x nil)`, not `(x)`; lists had to be built on
  `(quote ())`.
- [x] Quoted numbers stay tokens: `plus` accepts them, but
  `(eq (car (quote (1))) 1)` is false.
- [x] Mutual recursion between `def`s (a disabled test). It works since
  lexical scoping; the test's program had a malformed three-argument `def`,
  which now reports "expects a name and a value".
- [x] `defn` (a disabled test): `(defn name (params) body...)`, a stdlib macro.
  Macros can now expand to `lambda`, which is also a run-time special form.
- [x] Macros: `isMacroScope` and `createNonEvaluatingScope` were never used, and a
  macro's expansion wasn't evaluated. Expansions are now evaluated in the
  caller's scope, and the unused machinery is gone.

### Design

- [x] `def` writes into the shared `builtins` map, so definitions leak between
  `Process` calls and tests, the stdlib is reloaded on every call, and nothing
  is safe to run concurrently. Give each run its own scope over the builtins.
- [x] `Process` panics on bad input; return `(value, error)` or recover at the
  top level.
- [x] Values are Go `int64`/`string` in some places and `NumberToken`/
  `StringToken` in others; `NumberToken` re-parses its string on every `Eval`.

### Cleanup

- [x] gofmt the code (it's indented with 4 spaces; `define_` uses tabs).
- [x] Remove dead code: the `ParamsList` type, the unreachable `return nil` in
  `GetValue` and `macro`, the `else` after `return` in `atom`.
- [x] Go-style names for `make_param_binding_fn`, `if_`, `define_`, and `print`
  (which shadows Go's builtin).
- [x] Replace deprecated `ioutil`; fix the "Could nt" typo and `%t` verb in
  `macro`.
- [x] Tests for errors, arity and functions as values.

### Compiler foundations

Groundwork for compiling glisp: expand macros into a small **core AST**, and
make every backend (the interpreter first; later a bytecode VM, Go source,
WebAssembly, …) a walker over it.

The core forms, as Go types implementing a `Node` interface:

| Node | Form |
|---|---|
| `Const` | `int64`, string, `true`/`false`/`nil` |
| `Quote` | `(quote x)`: `x` as data |
| `Ref` | a variable; a later pass fills in its `Loc` (scope depth and slot) |
| `If` | `(if c then else)` |
| `Lambda` | `(lambda (params) body...)` |
| `Def` | `(def name value)` |
| `Apply` | `(f args...)`, a call (`Call` clashes with a go-matchers name) |

`defn`, `defmacro` and other macros never reach a backend: an `Expander`
runs them while expanding, using the interpreter to evaluate macro bodies.
Builtins other than the special forms (`car`, `plus`, `<`, `p`, …) stay
ordinary functions, called through `Ref` and `Apply`.

- [x] Add the `Node` types.
- [x] Write the `Expander`: special forms become nodes; `defmacro` registers a
  macro; macro calls are expanded, recursively, until only core forms remain.
- [x] Make the interpreter evaluate `Node`s, so every existing test runs
  through the expander. Macros then expand once, not on every call.
- [x] Remove the old `List`-walking evaluator and the `quote`, `if`,
  `lambda`, `def` and `defmacro` builtins it needed.
- [ ] A resolve pass that fills in each `Ref`'s `Loc` and each `Lambda`'s
  captured variables, for backends that compile variable access.

Decisions this makes:

- **Macros are defined before use.** A `defmacro` applies to the forms after it
  (and the stdlib's to every program). Today a macro expands each time its call
  runs, so a macro defined later still works; that stops.
- **Macros aren't values.** A macro name can't be passed to a function or
  returned, since it's gone after expansion.
