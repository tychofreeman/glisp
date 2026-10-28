package glisp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// runCompiled compiles program to wasm and runs it under WASI, returning
// what it prints on stdout and stderr.
func runCompiled(t *testing.T, program string) string {
	t.Helper()
	module, err := CompileWasm(program)
	if err != nil {
		return "compile error: " + err.Error()
	}
	m := instantiate(t, module)
	defer m.close()
	if err := m.start(); err != nil {
		return m.out.String() + "wasm error: " + err.Error()
	}
	return m.out.String()
}

// A compiledModule is a compiled program instantiated under WASI, not yet
// started. out collects what it prints on stdout and stderr.
type compiledModule struct {
	ctx   context.Context
	mod   api.Module
	out   *bytes.Buffer
	close func()
}

func instantiate(t *testing.T, module []byte) compiledModule {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	out := &bytes.Buffer{}
	config := wazero.NewModuleConfig().WithStdout(out).WithStderr(out).WithStartFunctions()
	mod, err := r.InstantiateWithConfig(ctx, module, config)
	if err != nil {
		r.Close(ctx)
		t.Fatal(err)
	}
	return compiledModule{ctx, mod, out, func() { r.Close(ctx) }}
}

// start runs the program. Ending by reporting an error (exit code 1) counts
// as running normally.
func (m compiledModule) start() error {
	_, err := m.mod.ExportedFunction("_start").Call(m.ctx)
	var exit *sys.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	return err
}

// assertSameOutput checks that program prints the same compiled as
// interpreted, reporting the first line where they differ.
func assertSameOutput(t *testing.T, program string) {
	t.Helper()
	got, want := runCompiled(t, program), runInterpreted(t, program)
	if got == want {
		return
	}
	name, _, cut := strings.Cut(program, "\n")
	if cut {
		name += " …"
	}
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; ; i++ {
		if i >= len(g) || i >= len(w) || g[i] != w[i] {
			t.Errorf("%s\nline %d of the output differs\ncompiled:    %q\ninterpreted: %q", name, i+1, lineOf(g, i), lineOf(w, i))
			return
		}
	}
}

// lineOf is lines[i], or "" past the end.
func lineOf(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// runInterpreted runs program in the interpreter and returns what
// cmd/glisp would print.
func runInterpreted(t *testing.T, program string) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = write
	printed := make(chan string)
	go func() {
		b, _ := io.ReadAll(read)
		printed <- string(b)
	}()
	value, err := Run(program)
	os.Stdout = stdout
	write.Close()
	out := <-printed
	if err != nil {
		return out + "glisp: " + err.Error() + "\n"
	}
	return out + "=> " + Show(value) + "\n"
}

var wasmPrograms = []string{
	`(plus 1 2)`,
	`-42`,
	`"héllo"`,
	`()`,
	``,
	`(quote (a b (c 1) "s"))`,
	`(car (quote (1 2 3)))`,
	`(cdr (quote (1 2 3)))`,
	`(cdr (quote (1)))`,
	`(car nil)`,
	`(cdr nil)`,
	`(car (quote ()))`,
	`(atom 1)`,
	`(atom (quote (1)))`,
	`(atom)`,
	`(atom nil)`,
	`(cons 1 2)`,
	`(cons 1 nil)`,
	`(cons 1 (quote (2 3)))`,
	`(cons 1)`,
	`(eq 1 1)`,
	`(eq "a" "a")`,
	`(eq "a" "b")`,
	`(eq (quote (1 (2))) (quote (1 (2))))`,
	`(eq (quote (1 (2))) (quote (1 (3))))`,
	`(eq (quote a) (quote a))`,
	`(eq (quote ()) nil)`,
	`(eq nil nil)`,
	`(eq car car)`,
	`(eq 1 "1")`,
	`(eq true true)`,
	`(- 10 3 2)`,
	`(- 5)`,
	`(plus)`,
	`(< 1 2)`,
	`(> 1 2)`,
	`(if true 1 2)`,
	`(if 1 1 2)`,
	`(def x 5) (plus x 1)`,
	`(def x 5)`,
	`((lambda (x y) (plus x y)) 1 2)`,
	`((lambda (x)) 1)`,
	`(def adder (lambda (n) (lambda (x) (plus x n)))) ((adder 3) 4)`,
	`(apply plus 1 (quote (2 3)))`,
	`(not true)`,
	`(not false)`,
	`(not 5)`,
	`(and)`,
	`(and true 5)`,
	`(and true true false)`,
	`(and false (car 5))`,
	`(or)`,
	`(or false 7)`,
	`(or false false)`,
	`(or true (car 5))`,
	`(cond)`,
	`(cond (false 1) (true 2))`,
	`(cond ((eq 1 2) 1) (else 3))`,
	`(cond (false 1))`,
	`(cond (true (p 1) 2))`,
	`(defn sign (n) (cond ((< n 0) -1) ((eq n 0) 0) (else 1))) (p (sign -5) (sign 0) (sign 9))`,
	`(defn count (n acc) (cond ((eq n 0) acc) (else (count (- n 1) (plus acc 1))))) (count 100000 0)`,
	`(error "bad value: " 5 " in " (quote (a "b")))`,
	`(defn check (n) (if (< n 0) (error "negative: " n) n)) (p (check 1)) (check -2)`,
	`(defn check (n) (if (< n 0) (error "negative: " n) n)) (check 1) (p (check -2) 3)`,
	`(error)`,
	`(plus 4611686018427387903 1)`,
	`(* 4611686018427387903 2)`,
	`(- -4611686018427387904 1)`,
	`(- -4611686018427387904)`,
	`(/ -4611686018427387904 -1)`,
	`4611686018427387904`,
	`(parse_number "4611686018427387905")`,
	`(* 3037000500 3037000500)`,
	`((lambda args args) 1 2)`,
	`((lambda args args))`,
	`((lambda (a . rest) (cons a rest)) 1 2 3)`,
	`((lambda (a b . rest) rest) 1 2)`,
	`((lambda (a b . rest) a) 1)`,
	`(defn total ns (if (eq ns (quote ())) 0 (plus (car ns) (apply total (cdr ns))))) (total 1 2 3 4)`,
	`(defn countdown (n . acc) (if (eq n 0) acc (countdown (- n 1) n))) (countdown 3)`,
	`(defn keep (. xs) (lambda () xs)) ((keep 1 2))`,
	`(apply plus (quote (1 2 3)))`,
	`(apply plus 1 2 (quote (3 4)))`,
	`(apply plus nil)`,
	`(apply plus 1 2)`,
	`(apply cons 1 (quote ((2 3))))`,
	`(apply (lambda (a b) (- a b)) (quote (10 3)))`,
	`(defn upto (n acc) (if (eq n 0) acc (upto (- n 1) (cons n acc)))) (apply plus (upto 10000 nil))`,
	`(p 1 "two" (quote three) (quote (4 5)) nil true false (quote ()))`,
	`(p)`,
	`(defn f (n) (if (eq n 0) 0 (plus n (f (- n 1))))) (f 100)`,
	`(defn ev (n) (if (eq n 0) true (od (- n 1)))) (defn od (n) (if (eq n 0) false (ev (- n 1)))) (p (ev 10) (od 7)) (ev 7)`,
	`(let (a 1 b (plus a 1)) (p a b) (plus a b))`,
	`(defn f (x) (let (y (plus x 1)) (lambda () (plus x y)))) ((f 10))`,
	`(defmacro twice (lambda (xs) (cons (quote plus) (cons (car xs) (cons (car xs) nil))))) (twice 21)`,
	`(def double (lambda (x) (plus x x))) (defmacro five (lambda (xs) (double 5))) (five one two three)`,
	`(defn f () (def g (lambda () h)) (def h 7) (g)) (f)`,
	`(defn f (x) (def x (plus x 1)) x) (f 1)`,
	`(defn counter (n) (lambda () n)) ((counter 9))`,
	`(defn compose (f g) (lambda (x) (f (g x)))) (defn inc (x) (plus x 1)) ((compose inc inc) 1)`,
	`(defn outer (a) (lambda (b) (lambda (c) (plus a b c)))) (((outer 1) 2) 3)`,
	`(mult 6 7)`,
	`(def p (lambda (x) x)) (p 5)`,
	`(cons (lambda (x) x) nil) 1`,
	`(defn count (n) (if (eq n 0) 0 (plus 1 (count (- n 1))))) (count 3000)`,
	`(p (* 2 3 7) (*) (/ 17 5) (/ -17 5) (mod 17 5) (mod -17 5) (mult 6 7))`,
	`(p (<= 1 2) (<= 2 2) (<= 3 2) (>= 2 2) (>= 1 2))`,
	`(p (apply * (quote (1 2 3 4))) (apply / 9 (quote (2))))`,
	`(p (number? 5) (number? (quote a)) (string? "s") (string? (quote s)) (symbol? (quote s)) (symbol? "s"))`,
	`(p (list? (quote (1 2))) (list? (quote ())) (list? nil) (function? car) (function? (lambda (x) x)) (function? (quote car)))`,
	`(p (boolean? false) (boolean? true) (boolean? nil) (nil? nil) (nil? (quote ())) (nil? 0))`,
	`(defn collect (n fs) (if (eq n 0) fs (collect (- n 1) (cons (lambda () n) fs)))) (def fs (collect 3 nil)) (p ((car fs)) ((car (cdr fs))) ((car (cdr (cdr fs)))))`,
	`(defn h (n) (def m (* n 2)) (if (eq n 0) m (h (- n 1)))) (h 5)`,
	`(defn g (n acc) (if (< n 1) acc (if (even n) (g (- n 1) (cons n acc)) (g (- n 1) acc)))) (defn even (n) (eq (mod n 2) 0)) (g 10 nil)`,
	`(defn adder (k) (lambda (x) (if (eq k 0) x ((adder (- k 1)) (plus x 1))))) ((adder 5) 10)`,
	`(def f (lambda (n) (if (eq n 0) (quote done) (f (- n 1))))) (def g f) (def f (lambda (n) (quote other))) (g 3)`,
	`(defn ev (n) (if (eq n 0) true (od (- n 1)))) (defn od (n) (if (eq n 0) false (ev (- n 1)))) (p (ev 1000) (od 1000) (ev 7))`,
	`(defn a (n acc) (if (eq n 0) acc (b (- n 1) (cons (quote a) acc)))) (defn b (n acc) (c n (cons (quote b) acc))) (defn c (n acc) (a n acc)) (a 3 nil)`,
	`(defn f (x) (g x 1)) (defn g (x y) (plus x y)) (f 41)`,
	`(defn f (n) (apply g n (quote (1)))) (defn g (n m) (if (eq n 0) m (f (- n 1)))) (f 100)`,
	`(defn make (k) (lambda (n) (if (eq n 0) k ((make (plus k 1)) (- n 1))))) ((make 0) 1000)`,
	`(p (plus 2 3) (- 2 3) (* -4 5) (< 1 2) (> 1 2) (<= 2 2) (>= 1 2) (eq (quote (1)) (quote (1))))`,
	`(p (car (quote (1 2))) (cdr (quote (1 2))) (cons 1 (quote (2))) (cons 1 nil) (cons 1 2) (atom 1) (atom (quote ())))`,
	`(p (number? 1) (number? "1") (list? nil) (list? (quote ())) (nil? nil) (nil? 0))`,
	`(def plus (lambda (a b) (* a b))) (plus 3 4)`,
	`(defn f (x) (car x)) (p (f (quote (1 2)))) (def car cdr) (f (quote (1 2)))`,
	`(defn g (a b) (< a b)) (p (g 1 2)) (def < >) (g 1 2)`,
	// Errors.
	`(car 1)`,
	`(cdr "s")`,
	`(plus 1 "a")`,
	`(- (quote a))`,
	`(-)`,
	`(< 1)`,
	`(eq 1)`,
	`(cons)`,
	`(apply)`,
	`(car)`,
	`(car 1 2)`,
	`(cdr 1)`,
	`(cons 1 2 3)`,
	`(substring "abc" 1)`,
	`(substring "abc" "x" 2)`,
	`(string< "a" 1)`,
	`(mod 1)`,
	`(< 1 "a")`,
	`(number? 1 2)`,
	`(apply 1 2)`,
	`((lambda (x) x))`,
	`((lambda (x y) x) 1 2 3)`,
	`(1 2)`,
	`(def l (quote (1 2))) (l 1)`,
	`(undefined)`,
	`(p 1) (car 1)`,
	`(defn f () (g)) (f)`,
	`(p defn)`,
	`(defn f () (p h) (def h 1)) (f)`,
	`(* 2 "a")`,
	`(/ 1 0)`,
	`(mod 1 0)`,
	`(/ 1)`,
	`(>= 1 2 3)`,
	`(number? 1 2)`,
	`(list?)`,
	`(defn k (n) (if (eq n 0) 0 (k))) (k 1)`,
	`(defn k (n) (if (eq n 0) (car 5) (k (- n 1)))) (k 3)`,
	`(defn f () (5 1)) (f)`,
	`(defn g (a b) a) (defn f (x) (g x)) (f 1)`,
	`(defn ev (n) (if (eq n 0) (car 1) (od (- n 1)))) (defn od (n) (ev (- n 1))) (p 0) (ev 10)`,
}

func TestCompiledProgramsPrintWhatTheInterpreterDoes(t *testing.T) {
	for _, program := range wasmPrograms {
		assertSameOutput(t, program)
	}
}

func TestCompiledExampleMatchesTheInterpreter(t *testing.T) {
	program, err := os.ReadFile("examples/numbers.glisp")
	if err != nil {
		t.Fatal(err)
	}
	assertSameOutput(t, string(program))
}

func TestCompiledFunctionsPrintAsFunctions(t *testing.T) {
	if got := runCompiled(t, `(p car (lambda (x) x))`); got != "<function> <function>\n=> ()\n" {
		t.Errorf("got %q", got)
	}
}

func TestCompiledRecursionGoesDeep(t *testing.T) {
	program := `(defn count (n) (if (eq n 0) 0 (plus 1 (count (- n 1))))) (count 200000)`
	if got := runCompiled(t, program); got != "=> 200000\n" {
		t.Errorf("got %q", got)
	}
}

// A call in tail position to the function itself is a jump, not a call, so
// a loop written as recursion runs in constant stack.
func TestCompiledSelfTailCallsAreLoops(t *testing.T) {
	program := `(defn count_down (n acc) (if (eq n 0) acc (count_down (- n 1) (plus acc 1))))
		(count_down 5000000 0)`
	if got := runCompiled(t, program); got != "=> 5000000\n" {
		t.Errorf("got %q", got)
	}
}

// A tail call to another function returns it to invoke as a pending call,
// which invoke makes after the caller's frame is gone: mutual recursion runs
// in constant stack.
func TestCompiledMutualTailCallsUseConstantStack(t *testing.T) {
	program := `(defn ev (n) (if (eq n 0) true (od (- n 1))))
		(defn od (n) (if (eq n 0) false (ev (- n 1))))
		(p (ev 1000000))
		(ev 1000001)`
	if got := runCompiled(t, program); got != "true\n=> false\n" {
		t.Errorf("got %q", got)
	}
}

// Calls to builtins run inline when the name still holds the builtin: the
// inline paths are there, and the results are the same as without them.
func TestCompiledBuiltinCallsAreDirect(t *testing.T) {
	program := `(defn f (a b) (cons (plus a (car b)) (cdr b))) (p (f 1 (quote (2 3)))) (f 1 2)`
	direct, err := CompileWasm(program)
	if err != nil {
		t.Fatal(err)
	}
	inlineCalls = false
	indirect, err := CompileWasm(program)
	inlineCalls = true
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) <= len(indirect) {
		t.Errorf("with inline paths: %d bytes; without: %d", len(direct), len(indirect))
	}
	assertSameOutput(t, program)
}

// Memory grows by doubling, so a program that allocates a lot grows it a few
// times, not once per allocation past the end (which crashed Node's WASI).
func TestCompiledMemoryGrowsByDoubling(t *testing.T) {
	for _, tc := range []struct{ name, program, want string }{
		{"allocating", `(defn build (n acc) (if (eq n 0) acc (build (- n 1) (cons n acc))))
			(defn churn (k) (if (eq k 0) 0 (plus (car (build 10000 nil)) (churn (- k 1)))))
			(churn 100)`, "=> 100\n"},
		{"building a string", `(defn twice (s k) (if (eq k 0) s (twice (str s s) (- k 1))))
			(string_length (twice "ab" 24))`, "=> 33554432\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			module, err := CompileWasm(tc.program)
			if err != nil {
				t.Fatal(err)
			}
			m := instantiate(t, module)
			defer m.close()
			initial := m.mod.Memory().Size()
			if err := m.start(); err != nil {
				t.Fatal(err)
			}
			final := m.mod.Memory().Size()
			if m.out.String() != tc.want {
				t.Errorf("printed %q", m.out.String())
			}
			if ratio := final / initial; final <= initial || final%initial != 0 || ratio&(ratio-1) != 0 {
				t.Errorf("memory grew from %d to %d bytes, not by doubling", initial, final)
			}
		})
	}
}

func TestCompilingReportsMalformedForms(t *testing.T) {
	_, err := CompileWasm("(p 1) (def x)")
	if err == nil || !strings.Contains(err.Error(), "def: expects a name and a value") {
		t.Errorf("got %v", err)
	}
}

func TestOnlyTheStdlibAProgramUsesIsCompiled(t *testing.T) {
	lambda := func(refs ...string) Lambda {
		var body []Node
		for _, name := range refs {
			body = append(body, Ref{Name: name})
		}
		return Lambda{Body: body}
	}
	stdlib := []Node{
		Def{"a", lambda("b", "car")},
		Def{"b", lambda()},
		Def{"helper", lambda("a")},
	}
	program := []Node{Apply{Fn: Ref{Name: "a"}}}
	var kept []string
	for _, node := range usedDefs(stdlib, program) {
		kept = append(kept, node.(Def).Name)
	}
	if strings.Join(kept, " ") != "a b" {
		t.Errorf("kept %v, want a and b", kept)
	}
}
