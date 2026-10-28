# glisp

A small Lisp interpreter written in Go.

```
go run ./cmd/glisp examples/numbers.glisp
```

runs a program and prints each `p` call, then the value of the last
expression. [`examples/numbers.glisp`](examples/numbers.glisp) builds
map, filter, fold, sorting, primes and more out of the builtins.

```
go run ./cmd/glisp -wasm numbers.wasm examples/numbers.glisp
wasmtime numbers.wasm        # or: wazero run numbers.wasm
```

compiles the program to a WebAssembly module instead, which runs in any
WASI runtime (including Node's `node:wasi`) and prints the same output.

Special forms: `quote if lambda def defmacro`. Builtins: `car cdr atom cons
eq apply p error`, `true false nil`, arithmetic `plus - * / mod < > <= >=`, and type
predicates `number? string? symbol? list? function? boolean? nil?`, and
strings `str string_length substring chars string< symbol parse_number`.
[`stdlib.glisp`](stdlib.glisp) is embedded in the package and loaded before
every program; it defines `mult`, `defn`, `let`, `not`, `and`, `or` and
`cond`.

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
- [x] `*`, `/`, `mod`, `<=` and `>=`. `/` truncates toward zero and `mod`'s
  result has the dividend's sign, as in Go; dividing by zero is an error.
  The stdlib's `mult` used to add `y` to itself `x` times; it is now `*`.
- [x] Type predicates: `number?`, `string?`, `symbol?`, `list?` (true for the
  empty list, not for `nil`), `function?`, `boolean?` and `nil?`. Before them a
  program couldn't tell a number from a symbol.
- [x] Strings: `(str x ...)` joins its arguments as `p` shows them (so it
  also turns numbers and lists into text); `string_length`, `substring` and
  `chars` count characters, not bytes (`(string_length "héllo")` is 5);
  `string<` orders strings; `symbol` makes a symbol from a string; and
  `parse_number` reads an integer, or gives `nil`.
- [x] Errors give their line: `glisp: line 2: car: expected a list, got
  number 5`. The tokenizer records the line each list starts on, the
  expander gives it to each call (code a macro made gets the macro call's
  line), and both backends report the line of the call being made when an
  error happens, inside functions too. Parse errors name the line where
  they were found.
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
- [x] `let` (the last disabled test): `(let (name value ...) body...)`, a stdlib
  macro. Bindings apply in order, so a value can use the names before it.
- [x] Macros: `isMacroScope` and `createNonEvaluatingScope` were never used, and a
  macro's expansion wasn't evaluated. Expansions are now evaluated in the
  caller's scope, and the unused machinery is gone.
- [x] Readable printing: values printed with Go's `%v` (`[1 2]`, `{x}`,
  `<nil>`, a function's address). `p` and the `=>` line now print glisp:
  `(1 2)`, `x`, `nil`, `<function>`, strings quoted inside lists; errors
  name glisp types (`got number 5`, `got list (a b)`).
- [x] `apply` spreads its last argument, as in other Lisps:
  `(apply f a b (quote (c d)))` calls `(f a b c d)`. It passed its
  arguments through unchanged, so `(apply plus (quote (1 2)))` was an
  error and `apply` added nothing over a plain call. The last argument
  must be a list (or nil); there must be at least two.
- [x] Rest parameters: `(lambda (a b . rest) ...)` binds `rest` to the list
  of the arguments after `a` and `b`, and `(lambda args ...)` to the list of
  all of them; `defn` takes either form. A lambda reports a wrong count as
  builtins do (`lambda (a b . rest) expects at least 2 arguments, got 1`),
  since both now share one `arity`.
- [x] `not`, `and`, `or` and `cond`, in the stdlib. Only `true` is true to
  `if`, so `(not x)` is true for anything else; `and` and `or` stop at the
  first argument that decides them and give the last otherwise (`(and)` is
  true, `(or)` false); `(cond (test body...) ... (else body...))` gives the
  first clause whose test is true, or nil. All but `not` are macros that
  expand to `if`, so a call in a clause's body stays a tail call.
- [x] `(error x ...)` fails with its arguments joined as `str` joins them,
  reported like any error: `glisp: line 2: negative: -2`, the line of the
  `error` call. (The interpreter drops trailing spaces from an error
  message; compiled code keeps them.)

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
- [x] A resolve pass that fills in each `Ref`'s `Loc` and each `Lambda`'s
  captured variables, for backends that compile variable access.

Decisions this makes:

- **Macros are defined before use.** A `defmacro` applies to the forms after it
  (and the stdlib's to every program). Today a macro expands each time its call
  runs, so a macro defined later still works; that stops.
- **Macros aren't values.** A macro name can't be passed to a function or
  returned, since it's gone after expansion.

### WebAssembly backend

`glisp -wasm OUT.wasm FILE.glisp` compiles the core AST to a WASI module,
with no dependencies: the [`wasm`](wasm) package encodes the binary format.
The tests run each module in [wazero](https://wazero.io) and check that it
prints exactly what the interpreter does, error messages included.

- [x] An encoder for the binary format (`wasm`).
- [x] Values in linear memory: tagged `i64`s, with integers inline and
  everything else (strings, symbols, list cells, closures) a tagged object.
- [x] Closures: each lambda is a function in the table; variables that inner
  lambdas capture live in cells, so a closure sees later `def`s.
- [x] Calls pass arguments on a stack in memory, so builtins like `plus` and
  `p` take any number and arity errors are reported, not trapped.
- [x] The builtins, printing and errors written in wasm, through WASI's
  `fd_write` and `proc_exit`.
- [x] Macros expand while compiling. Their helpers (top-level `def`s of
  lambdas) are evaluated in the interpreter too; other `def`s aren't, so a
  macro can't use a value computed at run time.
- [x] Memory doubles when the heap runs out, so a program that allocates a
  lot grows it a few times rather than once per allocation.
- [x] Garbage collection: a copying collector that slides what survives
  back to the start of the heap. It runs at safe points, just before a
  call, once the heap has grown 8MB (or as much as survived last time)
  since the last collection. To make that safe, a lambda keeps its closure
  and slots in a frame on the argument stack instead of wasm locals, and a
  call pushes the function it calls there too: at a safe point every value
  the program holds is on that stack or in a global, the collector's roots.
  Tests run every parity program with a collection at nearly every call.
- [ ] Node 22 (`node:wasi`) segfaults at random on long runs such as
  `examples/tour.glisp`, even with memory preallocated; turning off V8's
  wasm tier-up (`--no-wasm-tier-up`) mostly avoids it, and wazero runs it
  every time. Not yet tried on a newer Node.
- [x] Self tail calls are loops: a call in tail position to the function
  itself (checked at run time, so it holds even if the name is redefined)
  jumps back to the start of the function, so loops written as recursion
  run in constant stack. Each pass gives captured variables fresh cells.
- [x] Tail calls to other functions, mutual recursion included, use no
  stack: a trampoline. The caller leaves the call pending (the callee and
  its arguments, at the top of the argument stack) and returns a reserved
  value; `invoke` moves the arguments down to its own base and makes the
  call, repeating while calls come back pending. Calls to builtins stay
  direct. Only calls that aren't tail calls use the stack, about 300,000
  deep. (wasm's `return_call` would do this natively, but wazero v1.10
  doesn't run it.)
- [x] The interpreter replaces frames on tail calls: `Eval` is a loop, and
  a call to a closure in tail position (or an `if` branch) carries on in
  the same loop with the callee's body and scope instead of a new Go call.
  Mutual recursion runs in constant stack (a test goes a million deep);
  only calls that aren't tail calls use Go's stack, which crashes Go past
  about 300,000 deep. Lambdas now evaluate to a `*Closure` holding their
  code and scope; builtins stay `Function`s.
- [x] Integers are 63 bits in both backends: compiled code tags one bit of
  each value, so the interpreter wraps its integers there too (`int63` on
  every literal and arithmetic result). From 2^62-1 the next integer is
  -2^62; `(* 4611686018427387903 2)` is -2 in both.
- [x] Direct calls to builtins: a call naming `plus - * < > <= >= eq car cdr
  cons atom number? list? nil?` with the usual arguments runs the operation
  inline when the name still holds the builtin (checked at run time, so
  redefining it works), falling back to the ordinary call for anything the
  inline path doesn't handle, so results and errors are the same. A loop of
  tail calls through `plus` got 1.7x faster; arithmetic-heavy code 1.25x.
- [x] Only the stdlib a program uses is compiled: a stdlib definition goes
  into the module only if the program refers to it, directly or through
  another one. Macro helpers (`let_expand`, `cond_expand`, ...) run only
  while expanding, so they never do. The module for `1` went from 24.9KB
  to 9.0KB, and wazero compiles it in 5ms instead of 17.6ms; the tests,
  which compile hundreds of modules, take half as long (`go test -race`
  97s to 50s). The interpreter's stdlib load was never the cost: 41us a
  run.

### Missing abstractions

What a review of the code found: concepts each restated in several places,
by hand. In the order to do them; each makes the next cheaper.

- [x] **Structured wasm generation.** Runtime functions were assembled by
  hand: `If … Else … End` spelled out everywhere, and locals numbered by
  hand (`const v, p, n = 0, 1, 2`) with their types listed apart in
  `define`, so the two could disagree. Now every function, runtime,
  builtin or lambda, is built on a `wasmBody`: parameters by
  `b.param(i)` (the glisp convention's as `paramSelf`, `paramArgs`,
  `paramArgc`), locals declared where they're named
  (`n := b.local(i32)`), and control as `b.when`, `b.ifElse`, `b.until`
  and `c.failIf`; the helpers are its methods. The compiled modules are
  byte for byte what they were. Left for the builtin spec: each `fail`
  still prints its parts its own way.
- [x] **A builtin, defined once.** One builtin was written in up to five
  places, and its error messages twice, in Go and in wasm (`eq`'s had
  drifted). Now `builtinSpecs` (builtins.go) gives each builtin's name,
  how many arguments it takes and of what kind (the last kind stands for
  the rest), and its interpreter operation. Both backends check the
  arguments from the spec, with one wording, before the operation runs;
  the operations no longer check, and the wasm runtime's checking
  converters (`num`, `strArg`, `listArg`) are gone. The spec made the odd
  ones regular: `car`, `cdr` and `atom` take exactly one argument (they
  used to give nil or false for none and ignore extras), and `eq` and
  `apply` report a wrong count as the others do
  (`eq: expects 2 arguments, got 1`). The inline fast paths stay a wasm
  table by name, since they are an optimization of one backend.
- [x] **A value kind.** What each kind of value is lived in `typeName`,
  the predicates' tag map, `objSize`, the collector's field scan and
  `putType`. Now `tagKinds` (wasm_kinds.go) has a row per tag: the type
  `typeName` gives its values, and for heap objects their layout (a fixed
  part with its value fields, and a counted tail of bytes or values).
  `putType`, the type predicates, argument kinds, `objSize` and the
  collector's scan come from it, and a test checks every tag has a row.
  Argument kinds are sets of type names, so the interpreter checks them
  with `typeName` too. `putValue` still switches on tags itself: printing
  differs per kind in more than layout.
- [x] **The calling convention.** How a call lays out the argument stack
  was spelled out as raw offsets by `pushCall`, `invoke`, `tailApply`,
  `tailCall`, the inline paths and `compileLambda`'s frame. Now a call on
  the stack is a `stackCall` (`fn`, `arg(i)`, `args`, `pop`), a frame is
  written through `selfSet`/`slotSet`, and both share `valueOffset`, since
  both are a function followed by values. The runtime's indirect call and
  stack check are `callValue` and `checkStack`, and a lambda's arity
  message uses the builtins' `arguments`. Compiled modules are unchanged,
  byte for byte. The pending-tail-call protocol still has its one
  producer (`tailCall`) and one consumer (the runtime's `invoke`).
- [x] **One memory-growth policy.** `memory.grow` was called in five
  places in three functions (`alloc` and `gc` doubled, `str`'s capture
  grew a page at a time), each with its own "out of memory". Now the
  runtime's `reserve(end)` grows memory to reach `end`, doubling it, or by
  what's needed if that much isn't there, and all three call it. Building
  a long string with `str` now doubles memory too; a test that does so
  failed on the page-at-a-time growth.
- [ ] **Tail position and lines as facts of the AST.** Not done, on a
  closer look: the interpreter never decides tail position (whatever its
  `Eval` loop evaluates last is in tail position by construction), so
  only the compiler would read an `Apply.Tail`; and recording a call's
  line before and after its arguments is each backend's own way of
  tracking the call being made. Worth doing with a third backend.
- [x] **A parity check.** "Compile it, interpret it, compare" was written
  out in eight tests, and wazero's setup in three. Now `assertSameOutput`
  makes the comparison and reports the first line that differs, and
  `instantiate` sets up a module for tests that look inside it (memory
  size). A case type with options turned out not to be needed:
  `withGCThreshold` is the only option, and it is per test.
