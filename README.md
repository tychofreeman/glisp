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
- [ ] Calling a lambda with too few arguments panics (index out of range); extra
  arguments are silently dropped.
- [ ] `eq` on two lists crashes the Go runtime ("comparing uncomparable type").
- [ ] Silent wrong answers: `plus` skips non-integers (`(plus 1 "a")` is 1), and
  `car`/`cdr` of a non-list return nil.
- [ ] Tokenizer:
  - [ ] `(` or `)` inside a string splits the string.
  - [ ] Characters other than letters, digits, `- _ ?` and `"` are dropped as
    separators, so `(+ 1 2)` reads as `(1 2)`.
  - [ ] A lone `"` panics.
  - [ ] Input is read byte by byte, so non-ASCII text is mangled
    (`héllo` becomes `hÃ©llo`).
  - [ ] An unmatched `)` ends parsing: `(plus 1 2)) (plus 10 10)` returns 3.
- [x] Evaluating an empty list panicked.
- [x] `stdlib.glisp` was read from a hardcoded path.

### Language

- [ ] Comments.
- [ ] Subtraction and comparison builtins (`-`, `<`, `>`). Programs build them
  by counting up from 0, which is very slow for large numbers (Collatz steps
  for 27 takes about a minute).
- [ ] `true`, `false` and `nil` literals; programs define them as
  `(eq 1 1)`, `(eq 1 2)` and `(car (quote ()))`.
- [ ] `cons` onto `nil` gives `(x nil)`, not `(x)`; lists have to be built on
  `(quote ())`.
- [ ] Quoted numbers stay tokens, so `(plus (car (quote (1 2))) 1)` is 1.
- [ ] Macros: `isMacroScope` and `createNonEvaluatingScope` are never used.

### Design

- [ ] `def` writes into the shared `builtins` map, so definitions leak between
  `Process` calls and tests, the stdlib is reloaded on every call, and nothing
  is safe to run concurrently. Give each run its own scope over the builtins.
- [ ] `Process` panics on bad input; return `(value, error)` or recover at the
  top level.
- [ ] Values are Go `int64`/`string` in some places and `NumberToken`/
  `StringToken` in others; `NumberToken` re-parses its string on every `Eval`.

### Cleanup

- [ ] gofmt the code (it's indented with 4 spaces; `define_` uses tabs).
- [ ] Remove dead code: the `ParamsList` type, the unreachable `return nil` in
  `GetValue` and `macro`, the `else` after `return` in `atom`.
- [ ] Go-style names for `make_param_binding_fn`, `if_`, `define_`, and `print`
  (which shadows Go's builtin).
- [ ] Replace deprecated `ioutil`; fix the "Could nt" typo and `%t` verb in
  `macro`.
- [ ] Tests for errors, arity and functions as values.
