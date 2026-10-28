# glisp

A small Lisp interpreter written in Go.

The language, with examples of every form and builtin and how to run and
compile programs, is described in [`docs/spec.md`](docs/spec.md). Its
examples are run by the tests in both backends, so they stay true.

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

Special forms: `quote if lambda def defmacro begin`. Builtins: `car cdr atom cons
eq apply p error`, `true false nil`, arithmetic `plus - * / mod < > <= >=`, floats `float floor ceiling
truncate round sqrt`, and type predicates `number? integer? float? string? symbol? list? function? boolean? nil? vector?
map? data?`,
strings `str string_length substring chars string< symbol parse_number`, and
vectors `vector vector_ref vector_length vector_set vector_to_list`, maps
`hash_map map_get map_put map_remove map_has map_count map_keys map_to_list`,
data `data data_name data_ref data_length`, and `hash`.
[`stdlib.glisp`](stdlib.glisp) is embedded in the package and loaded before
every program; it defines `mult`, `defn`, `let`, `not`, `and`, `or` and
`cond`, and a list library: `list`, `empty?`, `length`, `map`, `filter`,
`fold`, `foldr`, `reverse`, `append`, `identity`, `compose`, `partial` and
`cons?`; `deftype`, which defines data types; `match`; math (`pi exp
log sin cos tan asin acos atan atan2 expt`); and errors as values, `maybe` (`just`, `nothing`), `either` (`left`, `right`), `bind`
and `do`.

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
- [x] Vectors: `(vector 1 2 3)` prints as `[1 2 3]`; `vector_ref` reads a
  value by index in constant time, `vector_length` counts them,
  `vector_to_list` lists them, and `(apply vector list)` makes one from a
  list. Like everything else in glisp they don't change: `vector_set`
  returns a new vector. `eq` compares them value by value; `vector?` tests
  for one. In compiled code a vector is one more row of `tagKinds`, so the
  collector needed nothing else.

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

### Maps

Planned: immutable maps, like the rest of glisp, built so that a map can
be fast to build without giving that up.

- [x] **One hash for both backends.** `(hash x)`: 32-bit FNV-1a over a
  canonical encoding (hash.go): an integer is `i` and its 8 bytes;
  `nil`, `true`, `false` are `n`, `t`, `f`; a string is `s`, its byte
  length and bytes, a symbol the same after `y`; a list is `(`, its
  elements, `)`; a vector the same in `[` `]`. Functions have no hash
  (`eq` never finds two equal). A test pins 15 values computed by a
  separate Python implementation of the encoding, and parity programs
  check that compiled code hashes alike, nested lists and vectors
  included.
- [x] **CHAMP in the interpreter.** A hash array mapped trie (Bagwell)
  in Steindorff and Vinju's CHAMP form: 32-way nodes with a bitmap for
  inline key/value pairs and one for child nodes, `popcount` to index
  them, collision nodes at the bottom, and compaction on removal so
  equal maps have one shape. Updates copy the path from the root
  (Okasaki's path copying), 3-4 nodes for a million keys. Builtins
  through `builtinSpecs` with a `mapKind`: `map`, `map_get` (with an
  optional default), `map_put`, `map_remove`, `map_has`, `map_count`,
  `map_keys`, `map_to_list`, `map?`; printed `{k v, k v}`. Tested
  against a Go `map` with random operations (maps.go, maps_test.go),
  for one shape however a map was built (removing the even keys of
  0..999 leaves the trie of the odd ones), and for keys whose whole
  hashes collide.
- [x] **CHAMP in wasm.** The same tries in memory (wasm_maps.go): a
  `tagMap` object (count, root) and `tagMapNode` nodes (two bitmaps, a
  slot count, then keys and values and subtries), both rows of
  `tagKinds`, so the collector needed nothing else; `i32.popcnt` in the
  encoder. One `nodeSplice` copies a node with slots taken out and put
  in, which every update is. Puts, removes (folding a one-entry subtrie
  back up), printing and equality follow the interpreter step for step,
  so a map has the same shape and prints the same in both backends: a
  parity program puts and removes 3,000 pseudo-random keys and prints
  the result, and another builds 2,000 entries and removes half.
- [x] **Transients inside.** `hash_map` changes in place the nodes it made
  itself, then freezes the result; no program sees the steps, and the
  trie is the one `map_put` after `map_put` makes (a test compares their
  shapes). In the interpreter a node carries its `owner` (the builder),
  and an owned node's slices are edited in place (`slices.Insert`); a
  node a builder takes over gets its own copies first, so nothing it
  shares with a frozen map changes. In wasm, where no collection runs
  inside a builtin, owned is an address: a node at or above `gOwned`,
  the heap's end when the build began, is the build's own; a value
  replaced or a subtrie changed is stored in place, and the map object
  is made once, at the end. Measured (`BenchmarkMaps`, 10,000 integer
  keys; 50,000 words from 2,000):

  | | Go `map` | `hash_map` | `map_put` each |
  |---|---|---|---|
  | build, interpreter | 1.0 ms | 2.3 ms (was 10; 1.6 allocations a key, was 6.9) | 10.0 ms |
  | word count, interpreter | 1.2 ms | | 41 ms (was 58) |
  | build ×50, compiled (wazero) | | 0.48 s (was 0.84) | 0.94 s |

  Where the gap is: a word count updates one key at a time, so it copies
  the path to it on every update (3 nodes, each with its slice), and
  hashed the key twice, for `map_get` and `map_put`; hashing a string no
  longer allocates (a third of the word count's allocations). What is
  left is the path copying itself, which only an explicit transient
  removes. Interpreted programs that build long lists pay more than any
  map: see the next item.
- [x] **Lists that share, in the interpreter.** A `List` is a Go slice, so
  `cons` copied the whole list: building a 50,000-word list with `cons`
  took 26 s of a word count whose map updates take 41 ms. Now `cons`
  makes a `Pair` (its first element and the list after it, shared), as
  wasm does, and a list is either: the reader, builtins' arguments and
  the builtins that build a whole list still make `List`s. `car`, `cdr`
  and equality step through either without copying (`uncons`); printing,
  hashing and `apply` copy a pair into a `List` (`asList`), as they read
  every element anyway. A macro's expansion and a program's value have
  their pairs made `List`s (`withoutPairs`), so the expander and Go
  callers see only `List`s, and the lines of lists read from source stay
  found. The word count went from 30 s to 0.39 s, a 10,000-pair
  `(apply hash_map ...)` program from 3.2 s to 0.31 s, and the tour from
  0.95 s to 0.81 s; a test checks that consing onto 10,000 elements
  allocates one pair.
- [ ] **Explicit transients, later (the door left open).** If
  write-heavy programs need it, Clojure's model: a transient taken
  from a map, updated in place by its one owner, and frozen back into
  an immutable map. Keep the node layout and builder able to carry an
  owner, so this needs no redesign.

### Parallelism and concurrency

Where wasm runtimes stand (October 2026): the threads proposal (shared
memory, atomics, wait/notify) is in V8, Wasmtime, Wasmer, WAMR and, as
experimental, wazero, but it doesn't create threads. Spawning them is
`wasi-threads` (Wasmtime behind a flag, WAMR; Wasmer's WASIX) or the
host's own workers, until the shared-everything-threads proposal
replaces it. WASI 0.3 (June 2026, on by default in Wasmtime 46) adds
async functions, streams and futures: concurrency on one event loop,
not parallelism.

**Preferred for now: share-nothing parallelism** (the user's choice).

- [ ] **Several instances, messages between them.** Each glisp process
  is its own instance of the module, with its own heap and collector;
  they exchange values through host functions, actor-style: `spawn` a
  function in a new instance, `send` and `receive` values. A value
  crosses by being copied (serialized out of one heap, built in the
  other), which immutability makes always safe. Works in every runtime
  today, wazero included, and needs nothing from the collector. The
  interpreter would do the same with goroutines and a copy at the
  boundary. To work out: how a spawned function's code and captured
  values reach the new instance (recompile the program there and send
  the closure's name and captures, or ship the module once and send a
  table index), message order and blocking, and what an error in one
  process does to the others.

Not now, recorded for later:

- [ ] **Green threads in one instance.** Pausing a computation needs the
  wasm call stack, which code can't save: only frames of tail calls are
  wholly in linear memory. Needs the stack-switching proposal, Binaryen's
  Asyncify, or compiling to continuation-passing style.
- [ ] **Shared-memory threads.** The collector is stop-the-world and the
  stack, heap and pending-call state are globals: it would need a stack
  and allocation buffer per thread, stopping every thread at a safe point
  to collect, and atomics around shared state, on runtimes where thread
  spawning is still experimental.

### Functional programming

Toward a Haskell-flavoured glisp. Decisions (2026-10-28, the user):

- **Dynamic first; static types punted.** glisp stays dynamically typed
  for now and grows the features that make functional code pleasant
  without types: algebraic data, pattern matching, errors as values.
  Hindley-Milner inference (over the core AST, as an optional checker)
  and typeclasses are not ruled out, just not planned.
- **`map` is the list function.** `(map f l)`, as in every functional
  language. The builtin that builds a hash map becomes `hash_map`, as in
  Clojure; `map_get`, `map_put` and the rest, and `map?`, keep their
  names, since they name the type.
- **No implicit laziness.** Haskell evaluates nothing until it is needed;
  glisp evaluates arguments before a call and keeps doing so. Laziness
  everywhere would change the order of output and errors, the spec, and
  the collector (an unevaluated value is a heap object), and isn't
  needed yet. Explicit laziness (`delay`/`force`, lazy sequences) can
  come later.
- **Floating-point numbers and string escapes** are wanted, after the
  four items below.

In order, one PR each:

- [x] **`map` renamed, and a list library.** `hash_map` builds a hash
  map; the stdlib gains `list`, `empty?`, `length`, `map`, `filter`,
  `fold`, `foldr`, `reverse`, `append`, `identity`, `compose` (of any
  number of functions) and `partial`, which programs (the tour among
  them) defined for themselves; the examples now use them, and print
  what they did, in both backends. Each takes `nil` or `()` as the empty
  list and runs in constant stack: `map`, `filter` and `foldr` fold and
  then reverse, rather than recursing. Specified, with examples, in
  docs/spec.md. A program can still redefine any of them, which changes
  the others that use it (`map` uses `fold` and `reverse`).
- [x] **Algebraic data types.** `(deftype shape (circle r) (rect w h))`
  defines constructors, a predicate per constructor (`circle?`) and one
  for the type (`shape?`), and accessors (`rect_w`). A value made by a
  constructor is a new kind of value, data: `(data name x ...)` makes
  one, `data_name`, `data_ref` and `data_length` read it, `data?` tests
  for it; it prints `#(circle 1)` and compares and hashes by its
  constructor's name and fields. In compiled code it is a vector-like
  object (`tagData`) whose first value is the name, so the collector,
  printer, `eq` and hash share the vector code: three refactorings
  first (`newVector` and `objectOfArgs` take the tag; the element loops
  of the printer and hash became methods). `deftype` is a stdlib macro,
  which needed a way for one macro to define several names: the special
  form `begin`, whose forms take its place at the top level or in a
  lambda's body (so its `def`s define there) and which is a sequence
  with its own scope elsewhere. The expander splices it, so no backend
  sees a new node. The parity check now compares what the command
  prints, so an error found while compiling counts as the same.
- [x] **Pattern matching.** `(match x ((circle r) ...) ((cons h t) ...)
  (_ ...))` over constructors, lists (`()`, `(list p ...)`, `(cons h
  t)`), vectors, literals and quoted data, nested, binding the names in a
  pattern; no match is an error, and a name may appear once in a pattern.
  A stdlib macro, so both backends get it from the expander: each clause
  becomes an `if` of the pattern's tests, outside in, then a `let` of
  its names, the value bound once to `__match`, and bodies stay in tail
  position (a loop of 100,000 through `match` runs in constant stack).
  The stdlib also gains `cons?`, a non-empty list, which the tests use.
- [x] **Errors as values.** `maybe` (`just`, `nothing`) and `either`
  (`left`, `right`) as data types in the stdlib, defined with
  `deftype`; `bind`, which chains a step after one that may fail (and
  over lists, as Haskell's list monad); and a `do` macro that writes a
  chain as a sequence of steps, `(x <- m)` binding a name. `bind`
  dispatches on the value it gets, so `do` needs no type to know which
  chain it is in. `error` still stops the program. All glisp, no Go.
- [x] **Floating-point numbers.** 64-bit floats beside the integers:
  literals such as `1.5` and `-2e-3` (digits on both sides of a point);
  `plus`, `-`, `*`, `/` and the comparisons take every argument as a
  float when one is (`/` of two integers still truncates; `mod` takes
  integers); IEEE 754 division by zero (`Infinity`, `NaN`); `float`,
  `floor`, `ceiling`, `truncate`, `round` (half to even) and `sqrt`;
  `integer?` and `float?`, with `number?` true for both; `eq` never
  equates an integer and a float; floats hash as `d` and their bits
  (one zero, one NaN), so they can be map keys. Builtins that need an
  integer (`substring`, `vector_ref`, `vector_set`, `data_ref`, `mod`)
  now say "expected an integer". A float prints as Haskell's `show`
  prints a Double: the shortest digits that read back as it, with a
  point from 0.1 to 10^7 and in scientific notation otherwise. In
  compiled code a float is a boxed object (`tagFloat`) and printing is
  Burger and Dybvig's free-format algorithm on 1280-bit bignums in a
  scratch area, checked against the interpreter's strconv on 3,000
  floats (random bits, powers of 2 and 10, subnormals, the extremes).
  The wasm encoder gained the f64 instructions; floats stay i64 bits in
  locals, reinterpreted for each operation.
- [x] **`parse_number` of a float.** `(parse_number "2.5e-3")` reads a
  float written as a literal is (without a `+`): the float nearest its
  first 40 significant digits, the rest ignored, in both backends and
  for literals too (`readFloat`). The interpreter reads them with
  strconv; compiled code with the printing bignums: the digits make N,
  the value N*10^e is A/B scaled by 2^k into [1, 2), and the float's
  bits are that many bits of the quotient, rounded half to even, with
  subnormals and overflow. 40 digits keep A and B within the bignums'
  1280 bits. Checked against strconv on 6,000 strings (random floats
  printed shortest and to 21 digits, random decimals of up to 40 digits
  with exponents from -350 to 350, the ties and boundaries).
- [x] **Math functions.** `pi`, `exp`, `log`, `sin`, `cos`, `tan`,
  `asin`, `acos`, `atan`, `atan2` and `expt`, written in glisp in the
  stdlib with Go's math package's algorithms (Cephes and FreeBSD msun),
  so both backends compute the same float by construction, no wasm was
  needed, and a program that doesn't use them doesn't include them.
  Against Go's math on 300 random arguments each and the special values:
  the trigonometric functions and their inverses agree exactly (the
  same algorithm), `log` exactly, `exp` within a float and `expt` within
  three (Go's `Exp` and `Log` are assembly on amd64). Go's `Log` is wrong
  for subnormals on amd64 (`Log(1e-310)` is -709.09); ours matches
  libm's -713.80. Internally `frexp` and `ldexp` are glisp loops over
  powers of two.
- [x] **Exact reduction for large arguments.** `sin`, `cos` and `tan` of
  arguments from 2^29 up reduce them as Go's `trigReduce` does
  (Payne-Hanek): the 53-bit mantissa times the bits of 4/pi from the
  right place (a table of 1,272 bits), keeping the bits Go keeps, its
  quirk included (the bits below the fraction's leading ones come from
  the top of the low word). glisp has no bit operations, so this is
  integer arithmetic on lists of 24-bit limbs (`math_bn_*`: add, scale,
  multiply, shift, mod), which no product overflows in either backend's
  integers. The accuracy test now draws half the arguments of each from
  2^29 to the largest float: all agree with Go's exactly. Before,
  `(sin 1e22)` gave -1.03e72 (Go's -0.8522...) and `(cos 1e300)`
  `-Infinity`.
- [x] **The stdlib, expanded once.** Every run expanded the whole stdlib
  again, its macros included: 41µs once, 1.2ms after `deftype`,
  `match` and `do`, 2.1ms with the math functions. Now it is expanded
  once (`stdlibExpanded`), in a scope of its own; a run copies in its
  macros and evaluates its nodes, and the compiler resolves the same
  nodes: 26µs a run, and the tests 31s to 24s. The macros are closures
  over the stdlib's scope, so a program that defines `let_expand` or
  `fold` no longer changes what `let` or `match` do.
- [x] **No float runtime without floats.** The float runtime (boxing,
  printing and reading with bignums, about 4.3KB) was in every module.
  A float can only come from a float literal or quoted float, or from
  `float`, `sqrt` or `parse_number` (arithmetic gives a float only from
  a float), and a builtin is only reached by its name; so the compiler
  checks the code the module will hold (`makesFloats`, after `usedDefs`)
  and, if it can't make a float, leaves out the float runtime and every
  float path in arithmetic, comparison, equality, hashing and printing.
  The module for `(p 1)` went from 23.5KB to 19.2KB. A test adds a float
  to programs with and without floats and checks what it costs. Left
  for later: the same for other parts of the runtime a program can't
  reach (maps, vectors, data), in general by reachability.
- [x] **Only the runtime a program reaches.** Two parts. In general,
  `wasm.Module.Prune` removes every function no export calls, directly
  or through others, nor a live table entry: it reads the calls out of
  each body (`renumberCalls`, which knows every instruction `Code`
  writes, and a test holds it to that) and renumbers them. A builtin's
  table entry is live once the compiler has made its closure, which only
  naming it does; a dead entry points at one function that traps, so
  the table keeps its indices. Then for what a call graph can't see,
  since printing, equality and hashing dispatch on the tag: `makes`
  (was `makesFloats`) finds which optional types the code can make
  (`makers`: floats, maps from `hash_map`, vectors, data), and the
  dispatch leaves out the branches for the others. The module for
  `(p 1)` went from 19.3KB to 8.6KB; using a map adds 3.5KB, a float
  1.6KB, `(exp 1)` 29KB (was 48KB in all). A test checks that each kind
  costs its runtime only when used.
- [x] **String escapes.** `\"`, `\\`, `\n` and `\t` in string literals,
  so a string can hold a quote; a backslash before anything else is an
  error (`line 1: unknown escape \q in a string`). The tokenizer reads
  them; both printers already wrote them, so a string inside a list
  prints as it reads back.
- [x] **Generic operations.** Typeclasses without types, in glisp
  itself: `(defgeneric name (param ...) on)` defines a function that
  fails for any value, and `(definstance name (param ... (p type?) ...)
  body ...)` redefines it to run its body when `(type? p)` holds, and
  the definition before it otherwise; a later instance is tried first,
  so a program extends or overrides the stdlib's. The stdlib has `fmap`
  (lists, vectors, map values, maybe, either), `show` (as `p` prints,
  but through `show` for the values inside containers, so a program's
  instance for its type shows inside a list) and `compare` (a total
  order within a type, `nothing` before `just`), with `data_fields`.
  Dispatch is a chain of predicate tests, one per instance, newest
  first. The compiler's `usedDefs` kept only the last definition of a
  name, so it missed what earlier instances refer to; it now follows
  every definition.
- [x] **An error's line, restored after a call.** Both backends set the
  line at each call with one and never put it back when the call
  returned, so code without lines (the stdlib's) that failed after a
  call reported the line of whatever ran last: `(fmap inc "abc")` after
  a program's own `show` instance reported that instance's line, not the
  `fmap` call's. Now a call that returns puts the line back: `Eval`
  saves it around `evaluate` (its old loop, extracted), and compiled
  code around each call not in tail position (`apply`), where the
  caller's call does it for a tail call. An error keeps the line it
  stopped at. Modules grow about 2% (the tour: 265KB to 272KB).

Not now, recorded for later: explicit laziness; purity (side effects stay
allowed anywhere, kept apart by convention); automatic currying, which conflicts with functions of any number
of arguments (`partial` instead); integers of any size; static types.

### Interop with Go

Decisions (2026-10-28, the user): glisp calls Go first (calling other wasm
libraries and being called from outside come later), through **typed
declarations**. Not built yet: the user asked for the plan first.

**Runtime: stay on wazero, defer the Component Model.** Calling Go needs
no Component Model: a Go function is a plain wasm import that the Go host,
running the module under wazero, supplies. The Component Model (WIT, the
canonical ABI for strings, lists and records between modules) matters
only for calling other wasm libraries. It isn't at 1.0 (June 2026), and
using it from Go means Wasmtime. Wasmtime is its reference
implementation, fast, and where new WASI features land, but wasmtime-go
needs cgo (no pure-Go cross-compiling or single static binary) and its C
API, which wasmtime-go wraps, didn't support components; it would also
replace the wazero the parity tests run on, and compiled glisp would no
longer run in any WASI runtime. On wazero there is only an experimental
component runtime (wacogo, August 2026). So: Go interop as typed core
wasm imports now, and the Component Model when it reaches 1.0 or wazero
supports it; nothing below would change for it. Wasm libraries could
meanwhile come in the same way, as typed core imports, with glisp's own
convention for passing strings and lists.

- [ ] **`extern`.** `(extern word_count "text" "count_words" (string)
  int)` declares a function a host supplies, by module and name, with
  its argument and result types: `int`, `float`, `bool`, `string`, and
  `nil` for no result, to start; `list`, `bytes`, `vector` and `any`
  later. A call checks its arguments in both backends with the builtins'
  messages (`word_count: expected a string, got number 5`).
- [ ] **A Go host.** `host := glisp.NewHost()`, `host.Register("text",
  "count_words", fn)`: the host checks each Go function's signature
  against its declaration by reflection, when the program loads, not
  when it calls; a Go function that returns an `error` raises a glisp
  error. `host.Run(program)` runs it in the interpreter, which calls the
  Go functions directly.
- [ ] **Compiled externs.** Each `extern` is a wasm import of the
  declared types: integers and floats as i64 and f64, booleans as i32,
  strings as a pointer and length in glisp's memory; a string result is
  copied into glisp's heap through an exported `alloc`, so the
  collector keeps working. `host.RunWasm(module, out)` runs a compiled
  module under wazero with the same Go functions, so a program behaves
  alike in both backends. `glisp run` runs a compiled module with a
  host; under other runtimes, the host supplies the imports itself.
- [ ] **Later:** calling glisp from Go (`host.Call(name, args...)`, and
  `(export name)` for compiled modules); wasm libraries as typed imports;
  the Component Model; share-nothing parallelism's `spawn`, `send` and
  `receive`, which are host functions of the same kind.

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
