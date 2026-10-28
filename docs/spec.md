# The glisp language

glisp is a small, lexically scoped Lisp with immutable data. A program runs
in two ways that print exactly the same thing, errors included:

- the **interpreter**, written in Go, runs a program directly;
- the **compiler** turns a program into a WebAssembly module for WASI, which
  runs in any WASI runtime (wasmtime, wazero, Node).

This document describes the language as both implement it. Every example
under a `glisp` heading below is run by `TestSpecExamples`
([`spec_test.go`](../spec_test.go)) in the interpreter and compiled, and
must print exactly the output shown after it, so the examples stay true as
the language changes.

## Contents

1. [Running programs](#1-running-programs)
2. [Lexical structure](#2-lexical-structure)
3. [Values and types](#3-values-and-types)
4. [Evaluation](#4-evaluation)
5. [Special forms](#5-special-forms)
6. [Macros](#6-macros)
7. [The standard library](#7-the-standard-library)
8. [Builtin functions](#8-builtin-functions)
9. [Errors](#9-errors)
10. [The two backends](#10-the-two-backends)
11. [A complete program](#11-a-complete-program)

## 1. Running programs

A program is a file of forms. Running it evaluates the forms in order,
prints what each `p` call prints, and then prints `=>` and the value of the
last form. `hello.glisp`:

```text
; hello.glisp
(defn greet (name) (str "Hello, " name "!"))
(p (greet "world"))
(* 6 7)
```

### In the interpreter

From a checkout of the repository:

```sh
go run ./cmd/glisp hello.glisp
```

```text
Hello, world!
=> 42
```

or install the command once and use it anywhere:

```sh
go install ./cmd/glisp      # puts glisp in $(go env GOPATH)/bin
glisp hello.glisp
```

### Compiled to WebAssembly

`-wasm OUT` compiles the program instead of running it. The module needs
nothing but WASI's `fd_write` and `proc_exit`:

```sh
go run ./cmd/glisp -wasm hello.wasm hello.glisp
```

Then run the module in a WASI runtime. Each prints the same as the
interpreter did:

```sh
wasmtime hello.wasm

# wazero, the runtime glisp's tests use; from the glisp checkout this needs
# no install, since go.mod already requires it:
go run github.com/tetratelabs/wazero/cmd/wazero run hello.wasm

# Node, with its built-in WASI (see run.mjs below):
node run.mjs hello.wasm
```

For Node, `run.mjs` is:

```js
import { readFile } from 'node:fs/promises';
import { WASI } from 'node:wasi';

const wasi = new WASI({ version: 'preview1' });
const module = await WebAssembly.compile(await readFile(process.argv[2]));
const instance = await WebAssembly.instantiate(module, wasi.getImportObject());
wasi.start(instance);
```

Node prints a warning that WASI is experimental, and Node 22 has been seen to
crash on long runs; `node --no-wasm-tier-up run.mjs hello.wasm` mostly
avoids it (see [the backends](#10-the-two-backends)).

### Exit status and errors

A program that runs to its end exits with status 0. An error stops the
program: what it printed so far stays, then `glisp: ` and the message go to
standard error, and the status is 1. Both backends do this. `fail.glisp`:

```text
(p "start")
(car 5)
(p "never printed")
```

```sh
$ glisp fail.glisp; echo "status $?"
start
glisp: line 2: car: expected a list, got number 5
status 1
```

The compiled module behaves the same:

```sh
$ glisp -wasm fail.wasm fail.glisp
$ wasmtime fail.wasm; echo "status $?"
start
glisp: line 2: car: expected a list, got number 5
status 1
```

A program that doesn't parse is reported when it is compiled, and no module
is written. `bad.glisp` is `(p 1` and a newline:

```sh
$ glisp -wasm bad.wasm bad.glisp; echo "status $?"
glisp: line 2: missing )
status 1
```

### From Go

The package `github.com/tychofreeman/glisp` does what the command does:

```go
value, err := glisp.Run(`(plus 1 2)`)   // value is int64(3); errors are returned
fmt.Println(glisp.Show(value))           // 3

module, err := glisp.CompileWasm(`(p "hi")`)  // the bytes of a WASI module
```

`RunFile` and `CompileWasmFile` take a file name instead of source.

## 2. Lexical structure

Source text is read as a sequence of forms. There are four kinds of token,
and lists of forms in parentheses.

**Whitespace** separates tokens. **Comments** run from `;` to the end of the
line.

**Integers** are an optional sign and decimal digits: `42`, `-7`, `+5`.

**Floats** are an optional sign and digits, then a fraction (`.` and
digits), an exponent (`e` or `E`, an optional sign and digits), or both:
`1.5`, `-0.25`, `2e3`, `6.02e23`. The point needs digits on both sides, so
`.5` and `1.` are symbols. A float literal is the float nearest to the
decimal it spells.

**Strings** are any text between double quotes, and may span lines. A
backslash starts an escape: `\"` is a double quote, `\\` a backslash,
`\n` a newline and `\t` a tab. A backslash before anything else is an
error. Inside a list, vector or map a string is printed with the same
escapes, so it reads back as the same string.

**Symbols** are any other run of characters that aren't whitespace,
parentheses or `"`: `x`, `string_length`, `nil?`, `<=`, `+`, `->`. A run of
digits too long for 64 bits is a symbol too.

**Lists** are forms in parentheses: `(plus 1 2)`, `(a (b c) "d")`, `()`.

```glisp
; A comment.
(p 42 -7 +5)     ; three numbers
(p 1.5 -0.25 2e3 1e-3)
(p "a string
over two lines")
(p "a \"quoted\" word" "back\\slash" "two\nlines")
(p (list "tab\there" "q\""))
```

```output
42 -7 5
1.5 -0.25 2000.0 1.0e-3
a string
over two lines
a "quoted" word back\slash two
lines
("tab\there" "q\"")
=> ()
```

There is no reader shorthand for quoting: `'x` is a symbol whose name starts
with `'`. Write `(quote x)`.

## 3. Values and types

| Type     | Written as                | Printed as         | Predicate   |
|----------|---------------------------|--------------------|-------------|
| number   | `42`, `1.5`               | `42`, `1.5`        | `number?`   |
| string   | `"text"`                  | `text`, or `"text"` inside a structure | `string?` |
| symbol   | `(quote name)`            | `name`             | `symbol?`   |
| list     | `(quote (1 2))`           | `(1 2)`            | `list?`     |
| nil      | `nil` or `()`             | `nil`              | `nil?`      |
| boolean  | `true`, `false`           | `true`, `false`    | `boolean?`  |
| function | `(lambda (x) x)`, `car`   | `<function>`       | `function?` |
| vector   | `(vector 1 2)`            | `[1 2]`            | `vector?`   |
| map      | `(hash_map "a" 1)`        | `{"a" 1}`          | `map?`      |
| data     | `(circle 1)`, after `deftype` | `#(circle 1)` | `data?`     |

All values are immutable: operations that "change" a vector or map return a
new one.

**Numbers** are integers or floats. Integers have 63 bits, from -2^62 to
2^62-1, and arithmetic on them wraps around at the ends, the same in both
backends. Floats are IEEE 754 64-bit floating-point numbers. `number?` is
true for both, and `integer?` and `float?` tell them apart; an integer and
a float are never `eq`, even when they are the same number. In errors an
integer's type is `number` and a float's `float`.

A float prints as the shortest decimal that reads back as the same float,
as Haskell shows one: with a point from 0.1 up to 10^7 (`0.1`, `2.0`,
`1234567.0`), in scientific notation otherwise (`1.0e-2`, `1.0e7`), and
`Infinity`, `-Infinity` and `NaN` for the values that aren't numbers.

```glisp
(p 4611686018427387903)
(p 1.5 0.1 100.0 1e7 0.001 (float? 1.5) (integer? 1.5) (eq 1 1.0))
(plus 4611686018427387903 1)
```

```output
4611686018427387903
1.5 0.1 100.0 1.0e7 1.0e-3 true false false
=> -4611686018427387904
```

**Strings** are text, counted in characters (Unicode code points), not
bytes. `p` and `=>` print a string as its text; inside a list, vector or map
it is printed in quotes.

```glisp
(p "héllo" (string_length "héllo"))
(p (quote ("a" b)))
"done"
```

```output
héllo 5
("a" b)
=> done
```

**Symbols** are names. Quoted, a symbol is a value; `symbol` makes one from
a string.

```glisp
(p (quote apple) (symbol? (quote apple)))
(eq (quote apple) (symbol "apple"))
```

```output
apple true
=> true
```

**Lists** are sequences of values. `nil` and the empty list `()` both end a
list and both mean "nothing", but they are different values: `nil` is not a
list, and `(quote ())` is. `()` written in code evaluates to `nil`.

```glisp
(p (list? (quote ())) (list? nil) (nil? nil))
(p (eq (quote ()) nil) (eq () nil))
(p (cdr (quote (1))) (car nil))
```

```output
true false true
false true
() nil
=> ()
```

There are no dotted pairs: `cons` onto something that isn't a list makes a
list of two.

**Booleans** are `true` and `false`. Only `true` counts as true: `if`
treats every other value, `0` and `nil` and `()` included, as false.

```glisp
(p (if true "yes" "no") (if 1 "yes" "no") (if nil "yes" "no"))
```

```output
yes no no
=> ()
```

**Functions** are the builtins and the values of `lambda` expressions.
Functions are never equal to anything, not even themselves.

```glisp
(p car (lambda (x) x))
(p (function? car) (eq car car))
```

```output
<function> <function>
true false
=> ()
```

**Vectors** hold values by index and read any of them in constant time.

```glisp
(def v (vector "a" "b" "c"))
(p v (vector_ref v 1) (vector_length v))
(p (vector_set v 0 "z") v)
```

```output
["a" "b" "c"] b 3
["z" "b" "c"] ["a" "b" "c"]
=> ()
```

**Maps** associate keys with values. Any value but a function can be a key,
including lists, vectors and maps; keys are compared with `eq`. A map's
entries print in an order that is fixed (the same in both backends, and the
same for equal maps however they were built) but not sorted.

```glisp
(def m (hash_map "apples" 3 "pears" 5))
(p (map_get m "pears") (map_get m "plums") (map_get m "plums" 0))
(def m2 (map_put m "plums" 7))
(p (map_count m) (map_count m2) (map_has m2 "plums"))
(eq m (map_remove m2 "plums"))
```

```output
5 nil 0
2 3 true
=> true
```

## 4. Evaluation

A form is evaluated as follows:

- a number or string evaluates to itself;
- a symbol evaluates to the value it is bound to, in the innermost scope that
  binds it (see [scope](#scope));
- `()` evaluates to `nil`;
- a list whose first element names a [special form](#5-special-forms) or a
  [macro](#6-macros) is handled by it;
- any other list is a **call**: its first element and then its arguments are
  evaluated, left to right, and the function is called with the arguments.

```glisp
(plus (p 1) 2)
```

```output
1
glisp: line 1: plus: expected a number, got list ()
```

(`p` prints its arguments and returns `()`, which `plus` refuses after the
`1` has been printed: the arguments were evaluated in order first.)

Calling something that isn't a function is an error:

```glisp
(5 1)
```

```output
glisp: line 1: Cannot call number 5 in (5 1)
```

### Scope

Names are lexically scoped. The top level of a program is one scope, over the
builtins; each call of a lambda makes a new scope, over the scope the lambda
was made in, holding its parameters and its own `def`s. A name looks up
through the scopes from the inside out.

A closure sees the scope it was made in as it is when the closure is called,
not as it was when it was made, so functions can refer to definitions that
come after them, and to each other:

```glisp
(defn is_even (n) (if (eq n 0) true (is_odd (- n 1))))
(defn is_odd (n) (if (eq n 0) false (is_even (- n 1))))
(p (is_even 10) (is_odd 7))
(def a 1)
(defn get_a () a)
(def a 2)
(get_a)
```

```output
true true
=> 2
```

Builtins are ordinary names and can be redefined, in the program's own scope.

### Tail calls

A call whose value is the value of the function it is in, a **tail call**,
doesn't use stack in either backend: a loop written as recursion runs in
constant space, mutual recursion included. The branches of an `if`, the last
form of a lambda, and the clauses of `and`, `or` and `cond` are in tail
position.

```glisp
(defn count_down (n) (if (eq n 0) (quote done) (count_down (- n 1))))
(count_down 1000000)
```

```output
=> done
```

Calls that aren't tail calls nest; compiled code allows them about 300,000
deep.

## 5. Special forms

There are six. They are recognised by name, before macros and calls.

### `quote`

`(quote datum)` is `datum` itself, not evaluated: a symbol stays a symbol and
a list stays a list.

```glisp
(p (quote x) (quote (plus 1 2)) (quote (a (b "c") 3)))
```

```output
x (plus 1 2) (a (b "c") 3)
=> ()
```

### `if`

`(if test then else)` evaluates `test`; if it is `true`, the value is that of
`then`, otherwise that of `else`. Only one branch is evaluated. All three
parts are required.

```glisp
(defn sign (n) (if (< n 0) -1 (if (eq n 0) 0 1)))
(p (sign -5) (sign 0) (sign 9))
(if true "only this" (car 5))
```

```output
-1 0 1
=> only this
```

```glisp
(if true 1)
```

```output
glisp: line 1: IF requires 3 parts - conditional, true expression and false expression. You have 2 parts - (true 1).
```

### `lambda`

`(lambda (params...) body...)` makes a function. Calling it binds each
parameter to an argument, evaluates the body forms in order, and gives the
value of the last; an empty body gives `nil`. The number of arguments must
match.

A **rest parameter** takes the remaining arguments as a list: `(a b . rest)`
needs at least two arguments and binds `rest` to the others, and a single
symbol in place of the list, `(lambda args ...)`, takes them all.

```glisp
(def add (lambda (x y) (plus x y)))
(p (add 2 3))
(p ((lambda (first . rest) rest) 1 2 3))
(p ((lambda args args) 1 2 3))
(p ((lambda (x))  1))
(def make_adder (lambda (n) (lambda (x) (plus x n))))
((make_adder 10) 5)
```

```output
5
(2 3)
(1 2 3)
nil
=> 15
```

```glisp
((lambda (a b) a) 1)
```

```output
glisp: line 1: lambda (a b) expects 2 arguments, got 1: (1)
```

### `def`

`(def name value)` evaluates `value` and binds `name` to it in the current
scope: at the top level, for the whole program; inside a lambda, for that
call only. Defining a name again replaces it. `def` gives `()`.

```glisp
(def x 5)
(defn scaled (n)
  (def factor 3)
  (* n factor))
(p (scaled x))
(def x 6)
(p x)
(def y 1)
```

```output
15
6
=> ()
```

### `defmacro`

`(defmacro name function)` defines a macro: see [the next section](#6-macros).
It gives `()`.

### `begin`

`(begin form...)` evaluates its forms in order and gives the value of the
last; `(begin)` gives `nil`. At the top level, or among the forms of a
lambda's body, its forms take its place, so a `def` in it defines in that
scope: that is how one macro can define several names (`deftype` does).
Anywhere else it is a sequence with a scope of its own.

```glisp
(begin (def a 1) (def b 2))
(p (plus a b))
(defn twice_plus_one (x) (begin (def y (* x 2))) (plus y 1))
(p (twice_plus_one 5))
(if true (begin (p "one") "two") "three")
```

```output
3
11
one
=> two
```

## 6. Macros

A macro is a function that runs on code before the code runs. When the
expander meets a list whose first element names a macro, it calls the
macro's function with **one argument, the list of the call's unevaluated
arguments**, and replaces the call with the code it returns, which is then
expanded in turn.

```glisp
; (unless test then else) is (if test else then).
(defmacro unless (lambda (xs)
  (cons (quote if)
    (cons (car xs)
      (cons (car (cdr (cdr xs)))
        (cons (car (cdr xs)) nil))))))
(unless false "then" "else")
```

```output
=> then
```

Macros are expanded as the program is read, form by form, so a macro applies
to the forms after its definition. A macro's function may call functions
defined before it with `def` or `defn`. A macro is not a value:

```glisp
defn
```

```output
glisp: defn is a macro, not a value
```

When a program is compiled, its macros run in the compiler. Their helper
functions are run there too, if they are top-level `def`s of lambdas (as
`defn` makes); other `def`s are not evaluated while compiling, so a macro
can't use a value the program computes when it runs.

## 7. The standard library

These are written in glisp, in [`stdlib.glisp`](../stdlib.glisp), and loaded
before every program. A compiled program includes only the ones it uses.

| Form | Meaning |
|------|---------|
| `(defn name (params...) body...)` | `(def name (lambda (params...) body...))`; rest parameters as for `lambda` |
| `(let (name value ...) body...)` | binds each name in turn, so a value can use the names before it, then evaluates `body` |
| `(not x)` | `true` unless `x` is `true` (a function) |
| `(and a b ...)` | the first argument that isn't `true`, as `false`, without evaluating the rest; otherwise the last. `(and)` is `true` |
| `(or a b ...)` | `true` as soon as an argument is `true`, without evaluating the rest; otherwise the last. `(or)` is `false` |
| `(cond (test body...) ...)` | the body of the first clause whose test is `true`, or `nil`. A test of `else` always is |
| `(mult x y)` | `(* x y)`, kept for older programs |

All but `not` and `mult` are macros. The stdlib also has a list library,
below.

```glisp
(defn describe (n)
  (cond ((< n 0) "negative")
        ((eq n 0) "zero")
        ((and (> n 0) (< n 10)) "small")
        (else "large")))
(p (describe -3) (describe 0) (describe 4) (describe 50))
(p (not true) (not 0) (or false 7) (and true 5) (and 1 (car 5)))
(let (a 2 b (* a 10))
  (plus a b))
```

```output
negative zero small large
false true 7 5 false
=> 22
```

### Lists

| Function | Result |
|----------|--------|
| `(list x ...)` | the list of its arguments |
| `(empty? l)` | whether `l` is `nil` or the empty list |
| `(cons? x)` | whether `x` is a list with at least one element |
| `(length l)` | the number of elements of `l` |
| `(map f l)` | the list of `(f x)` for each element `x` of `l` |
| `(filter f l)` | the elements `x` of `l` for which `(f x)` is `true` |
| `(fold f acc l)` | `(f (f (f acc a) b) c)` for `l` = `(a b c)`; `acc` for an empty `l` |
| `(foldr f init l)` | `(f a (f b (f c init)))` for `l` = `(a b c)`; `init` for an empty `l` |
| `(reverse l)` | the elements of `l` in reverse order |
| `(append a b)` | the elements of `a`, then those of `b` |
| `(identity x)` | `x` |
| `(compose f g ...)` | the function that applies the last of its arguments first: `((compose f g) x)` is `(f (g x))` |
| `(partial f x ...)` | `f` with its first arguments given: `((partial f x) y)` is `(f x y)` |

Each takes `nil` or `()` as the empty list, and an empty result is `()`.
They all run in constant stack, so they work on lists of any length.
They are ordinary definitions, so a program may redefine them; one that
does changes the others that use it (`map` uses `fold` and `reverse`).

```glisp
(def nums (list 1 2 3 4 5))
(defn square (x) (* x x))
(defn even? (x) (eq 0 (mod x 2)))
(p (map square nums) (filter even? nums) (length nums))
(p (fold plus 0 nums) (foldr cons (quote ()) nums) (reverse nums))
(p (append (list 1 2) (list 3)) (map square nil) (empty? nil) (empty? nums))
(def inc_then_double (compose (partial * 2) (partial plus 1)))
(p (inc_then_double 5) ((partial plus 1 2) 3) (identity 7))
(fold (lambda (acc x) (plus acc (square x))) 0 nums)
```

```output
(1 4 9 16 25) (2 4) 5
15 (1 2 3 4 5) (5 4 3 2 1)
(1 2 3) () true false
12 6 7
=> 55
```

### Data types

`(deftype type (constructor field ...) ...)` defines an algebraic data
type: a type whose values are each made by one of its constructors. For
`(deftype shape (circle r) (rect w h))` it defines

| Name | What it is |
|------|------------|
| `circle`, `rect` | the constructors: `(circle 2)` is a shape with `r` 2 |
| `circle?`, `rect?` | whether a value was made by that constructor |
| `shape?` | whether a value was made by any of the type's constructors |
| `circle_r`, `rect_w`, `rect_h` | the fields, by name; an error for a value made otherwise |

A constructor may have no fields: `(deftype maybe (just x) (nothing))`
makes `(nothing)` a value. The values are [data](#data) values: they print
as `#(circle 2)`, and are equal when their constructors and fields are.
`deftype` is a macro whose expansion is a [`begin`](#begin) of `def`s, so
it defines in the scope where it is.

```glisp
(deftype shape (circle r) (rect w h))
(def shapes (list (circle 2) (rect 3 4)))
(p shapes)
(defn area (s)
  (if (circle? s) (* 3 (* (circle_r s) (circle_r s))) (* (rect_w s) (rect_h s))))
(p (map area shapes) (map shape? (list (circle 1) 5)))
(p (eq (circle 2) (circle 2)) (eq (circle 2) (rect 2 2)) (data? (circle 1)))
(circle_r (rect 1 2))
```

```output
(#(circle 2) #(rect 3 4))
(12 12) (true false)
true false true
glisp: line 1: circle_r: expected a circle, got #(rect 1 2)
```

The error names line 1: code a macro made has the line of the macro call,
here `deftype`.

### Pattern matching

`(match x (pattern body...) ...)` evaluates `x` once, then the body of the
first clause whose pattern matches its value, with the names in the
pattern bound to the parts they matched. If no pattern matches, it is an
error. A body is in tail position.

| Pattern | Matches |
|---------|---------|
| `_` | anything, binding nothing |
| a symbol, `x` | anything, binding `x` to it |
| `42`, `"text"`, `true`, `false`, `nil` | that value (by `eq`) |
| `(quote datum)` | `datum`: `(quote yes)` matches the symbol `yes` |
| `()` | `nil` or the empty list |
| `(list p ...)` | a list of exactly as many elements, each matching its pattern |
| `(cons h t)` | a non-empty list whose first element matches `h` and the rest `t` |
| `(vector p ...)` | a vector of exactly as many values, each matching its pattern |
| `(c p ...)` | a data value made by the constructor `c`, with as many fields, each matching its pattern |

Patterns nest. A name may appear only once in a pattern. The heads
`quote`, `list`, `cons` and `vector` are patterns of their own, so a
constructor with one of those names can't be matched.

```glisp
(deftype shape (circle r) (rect w h))
(defn area (s)
  (match s
    ((circle r) (* 3 (* r r)))
    ((rect w h) (* w h))))
(p (map area (list (circle 2) (rect 3 4))))
(defn describe (x)
  (match x
    (0 "zero")
    ((quote yes) "the symbol yes")
    (() "empty")
    ((list a) (str "one: " a))
    ((list a (list b c)) (str "a, then a pair: " a b c))
    ((cons h t) (str h " and " (length t) " more"))
    ((vector _ y) (str "a vector, then " y))
    ((circle (rect w _)) (str "a circle of a rect " w " wide"))
    (_ "something else")))
(p (describe 0) "|" (describe (quote yes)) "|" (describe nil))
(p (describe (list 1)) "|" (describe (list 1 (list 2 3))) "|" (describe (list 1 2 3)))
(p (describe (vector 7 8)) "|" (describe (circle (rect 5 1))) "|" (describe 9))
(defn sum (l) (match l (() 0) ((cons h t) (plus h (sum t)))))
(p (sum (list 1 2 3)))
(match 5 (0 "zero"))
```

```output
(12 12)
zero | the symbol yes | empty
one: 1 | a, then a pair: 123 | 1 and 2 more
a vector, then 8 | a circle of a rect 5 wide | something else
6
glisp: line 23: match: no clause matches 5
```

`match` is a macro: a clause becomes an `if` that tests the value's shape
from the outside in, then a `let` of the names.

### Errors as values

`error` stops the program, and nothing can catch it. A function that can
fail can instead return its failure as a value, of one of two data types
in the stdlib:

```text
(deftype maybe (just value) (nothing))      ; a value, or none
(deftype either (left value) (right value)) ; a failure (left), or a value (right)
```

| Function | Result |
|----------|--------|
| `(bind m f)` | for `(just x)` or `(right x)`, `(f x)`; for `(nothing)` or a `left`, `m` itself; for a list, the lists `(f x)` gives for its elements, appended |
| `(do step ... result)` | a chain of steps through `bind`: `(x <- m)` binds `x` to what `m` gives and goes on, any other step goes on whatever it gives, and `result` is the chain's value. A step that fails ends the chain with that failure |

`do` is a macro: `(do (x <- m) rest...)` is `(bind m (lambda (x) (do rest...)))`.
The steps and the result must be of the same type: maybes, eithers, or
lists.

```glisp
(defn safe_div (a b) (if (eq b 0) (nothing) (just (/ a b))))
(p (safe_div 10 2) (safe_div 1 0))
(defn calc (a b c)
  (do (x <- (safe_div a b))
      (y <- (safe_div x c))
      (just (plus x y))))
(p (calc 100 5 2) (calc 1 0 2))
(defn parse (s)
  (let (n (parse_number s))
    (if (nil? n) (left (str "not a number: " s)) (right n))))
(defn add_strings (a b) (do (x <- (parse a)) (y <- (parse b)) (right (plus x y))))
(p (add_strings "2" "40") (add_strings "2" "forty"))
(p (match (add_strings "1" "x") ((right n) n) ((left e) (str "failed, " e))))
(p (do (x <- (list 1 2)) (y <- (list 10 20)) (list (plus x y))))
(bind 5 identity)
```

```output
#(just 5) #(nothing)
#(just 30) #(nothing)
#(right 42) #(left "not a number: forty")
failed, not a number: x
(11 21 12 22)
glisp: line 15: bind: expected a maybe, an either or a list, got 5
```

### Generic operations

A generic operation has an instance for each type of value it takes:
typeclasses without types. `(defgeneric name (param ...) on)` defines
`name` to fail for any value of `on`; `(definstance name (param ...)
body ...)`, with one parameter written `(p type?)`, adds the instance for
the values for which `(type? p)` holds. A later instance is tried before
earlier ones, so a program can add instances for its own types, or
replace those of the stdlib, whose generic operations are these:

| Function | Result |
|----------|--------|
| `(fmap f x)` | `f` applied to each value `x` holds: the elements of a list or a vector, the values of a map, the value of a `just` or a `right` (a `nothing` or a `left` is itself) |
| `(show x)` | `x` written as a string, as `p` prints it, but through `show`'s instances for the values inside lists, vectors, maps and data |
| `(compare a b)` | -1, 0 or 1 as `a` sorts before, with or after `b`, two values of one type: numbers by value, strings and symbols by their characters, `false` before `true`, lists, vectors and data element by element (data by constructor name first, but `nothing` before `just`) |
| `(data_fields d)` | the fields of `d`, as a list |

```glisp
(def inc (lambda (x) (plus x 1)))
(p (fmap inc (list 1 2)) (fmap inc (vector 1 2)) (fmap inc (hash_map "a" 1)))
(p (fmap inc (just 1)) (fmap inc (nothing)) (fmap inc (right 1)) (fmap inc (left "no")))
(p (show (list 1 "a" (vector 2.5 (quote s)) (hash_map 1 (just "x")))))
(p (compare 1 2) (compare "b" "a") (compare (list 1 2) (list 1 2)) (compare (nothing) (just 0)))
(deftype point (pt x y))
(definstance show ((x point?)) (str "<" (pt_x x) " " (pt_y x) ">"))
(definstance compare ((a point?) b) (compare (list (pt_x a) (pt_y a)) (list (pt_x b) (pt_y b))))
(p (show (list (pt 1 2) (vector (pt 3 4)))) (compare (pt 1 2) (pt 1 3)))
(defgeneric size (x) x)
(definstance size ((x list?)) (length x))
(definstance size ((x vector?)) (vector_length x))
(p (size (list 1 2 3)) (size (vector)))
(size "abc")
```

```output
(2 3) [2 3] {"a" 2}
#(just 2) #(nothing) #(right 2) #(left "no")
(1 "a" [2.5 s] {1 #(just "x")})
-1 1 0 -1
(<1 2> [<3 4>]) -1
3 0
glisp: line 10: size: no instance for "abc"
```

The error names line 10, where `size` was defined: the call being made
is the `error` that `defgeneric` put there.

### Math

| Name | What it is |
|------|------------|
| `pi` | the float nearest π |
| `(exp x)`, `(log x)` | e to the power `x`; the natural logarithm of `x` (`NaN` below 0, `-Infinity` at 0) |
| `(sin x)`, `(cos x)`, `(tan x)` | the trigonometric functions, of `x` in radians |
| `(asin x)`, `(acos x)`, `(atan x)` | their inverses (`NaN` outside -1 to 1 for `asin` and `acos`) |
| `(atan2 y x)` | the angle of the point (x, y), from -π to π |
| `(expt x y)` | `x` to the power `y`: an integer for integers with `y` at least 0 (wrapping as integer arithmetic does), otherwise a float |

They take integers or floats and, but for `expt` of integers, give floats.
They are written in glisp in the stdlib, with Go's math package's
algorithms, so both backends compute the same float. The results are
within a float or so of the true value; `sin`, `cos` and `tan` lose
accuracy for arguments beyond about 5×10⁸.

```glisp
(p pi (exp 1) (log 10) (log 1e-310))
(p (sin (/ pi 6)) (cos 0) (tan (/ pi 4)) (atan 1) (atan2 1 -1) (acos -1))
(p (expt 2 10) (expt 2 -1) (expt 2.0 0.5) (expt -8 (/ 1.0 3)) (expt 10 -3))
(exp 1000)
```

```output
3.141592653589793 2.718281828459045 2.302585092994046 -713.8013788281542
0.49999999999999994 1.0 1.0 0.7853981633974483 2.356194490192345 3.141592653589793
1024 0.5 1.4142135623730951 NaN 1.0e-3
=> Infinity
```

## 8. Builtin functions

Builtins check their arguments before they run: the number, and the type of
each, with the same messages in both backends. In the tables, `n` is a
number, `i` and `j` integers, `s` a string, `l` a list (or `nil`), `v` a vector, `m` a map, `f` a
function and `x` any value; `...` means any number more.

### Lists

| Builtin | Result |
|---------|--------|
| `(car l)` | the first element of `l`; `nil` for an empty list or `nil` |
| `(cdr l)` | `l` without its first element, a list; `nil` for an empty list or `nil` |
| `(cons x l)` | the list of `x` followed by the elements of `l`. If `l` is `nil` or left out, `(x)`; if it is not a list, `(x l)` |
| `(atom x)` | `true` unless `x` is a list |

```glisp
(def l (quote (1 2 3)))
(p (car l) (cdr l) (cons 0 l))
(p (cons 1 nil) (cons 1 2) (atom l) (atom nil))
```

```output
1 (2 3) (0 1 2 3)
(1) (1 2) false true
=> ()
```

### Arithmetic and comparison

| Builtin | Result |
|---------|--------|
| `(plus n ...)` | the sum; `(plus)` is 0 |
| `(- n ...)` | `n` minus the others; `(- n)` is `-n` |
| `(* n ...)` | the product; `(*)` is 1 |
| `(/ a b)` | `a` divided by `b`: for two integers, truncated toward zero |
| `(mod i j)` | the remainder of `(/ i j)`, with `i`'s sign: integers only |
| `(< a b)` `(> a b)` `(<= a b)` `(>= a b)` | comparisons of two numbers |

If any argument is a float, every argument is taken as a float and the
result is a float (a comparison compares them as floats). Dividing an
integer by the integer 0 is an error; with a float, division by zero
gives `Infinity`, `-Infinity` or `NaN`, as IEEE 754 does.

```glisp
(p (plus 1 2 3) (- 10 3 2) (- 5) (* 2 3 4))
(p (/ 7 2) (/ -7 2) (mod 7 2) (mod -7 2))
(p (< 1 2) (>= 1 2) (<= 2 2))
(/ 1 0)
```

```output
6 5 -5 24
3 -3 1 -1
true false true
glisp: line 4: /: division by zero
```

```glisp
(p (plus 1 2.5) (* 2 0.5) (- 1.5) (/ 7 2.0) (/ 1 0.0) (/ 0.0 0))
(p (plus 0.1 0.2) (< 1 1.5) (> 2.0 2))
```

```output
3.5 1.0 -1.5 3.5 Infinity NaN
0.30000000000000004 true false
=> ()
```

### Floats

| Builtin | Result |
|---------|--------|
| `(float n)` | `n` as a float |
| `(floor n)` | the largest integer not above `n` |
| `(ceiling n)` | the smallest integer not below `n` |
| `(truncate n)` | `n` without its fraction: the integer toward zero |
| `(round n)` | the nearest integer; halfway between two, the even one |
| `(sqrt n)` | the square root of `n`, a float (`NaN` for a negative `n`) |

`floor`, `ceiling`, `truncate` and `round` give an integer, so a float
whose integer part is outside the integers' range, or `NaN`, is an error.
An integer gives itself.

```glisp
(p (float 3) (floor 2.5) (ceiling 2.5) (truncate -2.5) (floor 7))
(p (round 2.5) (round 3.5) (round -0.5) (sqrt 2) (sqrt 16))
(floor 1e300)
```

```output
3.0 2 3 -2 7
2 4 0 1.4142135623730951 4.0
glisp: line 3: floor: 1.0e300 is out of range for an integer
```

### Equality

`(eq a b)` is `true` if `a` and `b` are equal: numbers, strings, symbols and
booleans by value, lists and vectors element by element, maps entry by
entry. Values of different types are never equal, nor are functions.

```glisp
(p (eq 1 1) (eq "a" "a") (eq 1 "1"))
(p (eq (quote (1 (2))) (quote (1 (2)))) (eq (vector 1 2) (vector 1 2)))
(eq (hash_map 1 2 3 4) (hash_map 3 4 1 2))
```

```output
true true false
true true
=> true
```

### Functions

`(apply f x ... l)` calls `f` with the arguments `x ...` followed by the
elements of the list `l`.

```glisp
(p (apply plus (quote (1 2 3))))
(apply plus 1 2 (quote (3 4)))
```

```output
6
=> 10
```

### Output and errors

| Builtin | Result |
|---------|--------|
| `(p x ...)` | prints its arguments on one line, separated by spaces, and gives `()` |
| `(str x ...)` | its arguments printed as `p` prints them, joined into one string |
| `(error x ...)` | stops the program with an error whose message is `(str x ...)` |

```glisp
(p "total:" 3 (quote (a "b")))
(p (str "x=" 1 ", list=" (quote (a "b"))))
(error "bad value: " 42)
```

```output
total: 3 (a "b")
x=1, list=(a "b")
glisp: line 3: bad value: 42
```

### Strings

| Builtin | Result |
|---------|--------|
| `(string_length s)` | the number of characters in `s` |
| `(substring s start end)` | the characters of `s` from `start` up to, not including, `end` |
| `(chars s)` | the characters of `s`, as a list of one-character strings |
| `(string< a b)` | whether `a` sorts before `b` (by bytes, so by code point) |
| `(symbol s)` | the symbol named `s` |
| `(parse_number s)` | the number `s` spells, or `nil`: an integer (an optional `-` and digits) or a float (written as a float literal is, without a `+`), read as the float nearest its first 40 significant digits |

```glisp
(p (string_length "héllo") (substring "hello" 1 3) (chars "abc"))
(p (string< "apple" "banana") (parse_number "-12") (parse_number "12x") (parse_number "2.5e-3"))
(substring "hello" 2 9)
```

```output
5 el ("a" "b" "c")
true -12 nil 2.5e-3
glisp: line 3: substring: indexes 2..9 out of range for a string of length 5
```

### Vectors

| Builtin | Result |
|---------|--------|
| `(vector x ...)` | a vector of its arguments |
| `(vector_ref v i)` | the value at index `i`, counting from 0 |
| `(vector_length v)` | the number of values in `v` |
| `(vector_set v i x)` | a new vector like `v` but with `x` at `i` |
| `(vector_to_list v)` | the values of `v`, as a list |

`(apply vector l)` makes a vector from a list. An index outside the vector
is an error.

```glisp
(def v (apply vector (quote (10 20 30))))
(p v (vector_to_list v) (vector))
(vector_ref v 3)
```

```output
[10 20 30] (10 20 30) []
glisp: line 3: vector_ref: index 3 out of range for a vector of length 3
```

### Maps

| Builtin | Result |
|---------|--------|
| `(hash_map k v ...)` | a map of the keys and values given in pairs; a later value for the same key wins |
| `(map_get m k)` | `k`'s value in `m`, or `nil` |
| `(map_get m k default)` | `k`'s value in `m`, or `default` |
| `(map_put m k v)` | a new map like `m` but with `k` bound to `v` |
| `(map_remove m k)` | a new map like `m` but without `k` |
| `(map_has m k)` | whether `m` has the key `k` |
| `(map_count m)` | the number of entries in `m` |
| `(map_keys m)` | the keys of `m`, as a list, in the map's order |
| `(map_to_list m)` | the entries of `m`, as a list of `(k v)` lists, in the map's order |

Maps are hash array mapped tries: getting, putting and removing take time
that grows with the logarithm of the size, and a new map shares most of its
structure with the old one.

```glisp
(def m (hash_map 1 "one" 1 "uno" (quote (a b)) "a list"))
(p (map_count m) (map_get m 1) (map_get m (quote (a b))))
(p (map_to_list (map_remove m (quote (a b)))))
(map_put m car 1)
```

```output
2 uno a list
((1 "uno"))
glisp: line 4: map_put: can't hash function <function>
```

### Data

Values of the types `deftype` defines (see [data types](#data-types)) are
made by these builtins, which a program can also call itself.

| Builtin | Result |
|---------|--------|
| `(data name x ...)` | a data value made by the constructor named by the symbol `name`, with the fields `x ...` |
| `(data_name d)` | the name of `d`'s constructor, a symbol |
| `(data_ref d i)` | field `i` of `d`, counting from 0 |
| `(data_length d)` | how many fields `d` has |

A data value prints as `#(` its constructor's name and fields `)`. Two are
equal if they have the same constructor name and equal fields; they hash
alike too, so they can be map keys.

```glisp
(def d (data (quote point) 1 "two"))
(p d (data_name d) (data_ref d 1) (data_length d))
(p (data (quote unit)) (eq d (data (quote point) 1 "two")) (eq d (vector (quote point) 1 "two")))
(p (map_get (hash_map d "found") (data (quote point) 1 "two")))
(data_ref d 2)
```

```output
#(point 1 "two") point two 2
#(unit) true false
found
glisp: line 5: data_ref: index 2 out of range for #(point 1 "two")
```

```glisp
(data "point" 1)
```

```output
glisp: line 1: data: expected a symbol, got string "point"
```

### Hashing

`(hash x)` is a number from 0 to 2^32-1 computed from `x`'s value, the same
in both backends: equal values have equal hashes. Functions can't be hashed.

```glisp
(eq (hash (quote (1 "two"))) (hash (cons 1 (quote ("two")))))
```

```output
=> true
```

### Type predicates

`number?`, `integer?`, `float?`, `string?`, `symbol?`, `list?`, `nil?`,
`boolean?`, `function?`, `vector?`, `map?` and `data?` each take one value and say whether it is of that type
(see [values and types](#3-values-and-types)).

```glisp
(p (number? 1) (string? "s") (symbol? (quote s)) (boolean? false))
(p (list? (quote ())) (list? nil) (vector? (vector)) (map? (hash_map)))
(p (data? (data (quote unit))) (data? (vector)))
(p (number? 1.5) (integer? 1.5) (float? 1.5) (integer? 1))
```

```output
true true true true
true false true true
true false
true false true true
=> ()
```

## 9. Errors

An error stops the program; glisp has no way to catch one. The message
says what went wrong and, for an error in a call, the line of the call that
was being made, inside functions too. Code a macro made gets the line of the
macro call.

```glisp
(defn first_of (l) (car l))
(p "before")
(first_of 5)
```

```output
before
glisp: line 1: car: expected a list, got number 5
```

The kinds of error:

- **parse errors**: `missing )`, `unexpected )`, `unterminated string`, with
  the line where they were found;
- **unknown names**: `Cannot resolve symbol x`;
- **arguments**: the wrong number (`car: expects 1 argument, got 2`) or type
  (`car: expected a list, got number 5`), for builtins and lambdas alike;
- **operations**: dividing by zero, an index out of range, hashing a
  function;
- **`error`**: whatever message the program gives.

```glisp
(car 1 2)
```

```output
glisp: line 1: car: expects 1 argument, got 2
```

```glisp
undefined_name
```

```output
glisp: Cannot resolve symbol undefined_name
```

## 10. The two backends

The interpreter and the compiler implement one language: the tests run
every program in both and compare what they print, error messages included.
What differs:

- **Speed.** Compiled code is faster; it inlines calls to the common builtins
  (`plus`, `-`, `*`, comparisons, `eq`, `car`, `cdr`, `cons`, `atom` and some
  type predicates) while still allowing a program to redefine them.
- **Memory.** Compiled code manages its own heap in linear memory, with a
  copying garbage collector, and grows memory as it needs.
- **Stack.** Calls that aren't tail calls nest about 300,000 deep in
  compiled code; the interpreter goes deeper.
- **Errors found while compiling.** Parse errors and malformed special
  forms are reported by `glisp -wasm`, and no module is written; the
  interpreter reports them when it reaches them, after running the forms
  before. The message is the same.
- **Macros at compile time.** A macro can use only functions defined with a
  top-level `def` of a lambda (see [macros](#6-macros)).
- **Trailing spaces.** The interpreter drops trailing spaces from an error
  message; compiled code keeps them.
- **Runtimes.** Node 22 (`node:wasi`) has crashed at random on long runs,
  which `--no-wasm-tier-up` mostly avoids; wazero and wasmtime run them.

## 11. A complete program

A word count, with a map, a fold over a list, and sorting:

```glisp
(defn count_words (words)
  (fold (lambda (counts w) (map_put counts w (plus 1 (map_get counts w 0))))
        (hash_map) words))

; Insertion sort of (word count) pairs, most frequent first, then by word.
(defn before (a b)
  (let (ca (car (cdr a)) cb (car (cdr b)))
    (or (> ca cb) (and (eq ca cb) (string< (car a) (car b))))))
(defn insert (x sorted)
  (cond ((eq sorted (quote ())) (cons x nil))
        ((before x (car sorted)) (cons x sorted))
        (else (cons (car sorted) (insert x (cdr sorted))))))
(defn sort (l) (fold (lambda (sorted x) (insert x sorted)) (quote ()) l))

(def words (quote ("the" "cat" "and" "the" "hat" "and" "the" "bat")))
(def counts (count_words words))
(p (map_count counts) "distinct words")
(defn show (pairs)
  (if (eq pairs (quote ())) (quote done)
    (let (pair (car pairs))
      (p (car pair) (car (cdr pair)))
      (show (cdr pairs)))))
(show (sort (map_to_list counts)))
```

```output
5 distinct words
the 3
and 2
bat 1
cat 1
hat 1
=> done
```

More programs: [`examples/numbers.glisp`](../examples/numbers.glisp) and
[`examples/tour.glisp`](../examples/tour.glisp), a longer tour with a list
library, sorting, primes, symbolic differentiation, N-queens, Church
numerals and lazy streams.
