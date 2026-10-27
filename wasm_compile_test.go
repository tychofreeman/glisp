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
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	var out bytes.Buffer
	config := wazero.NewModuleConfig().WithStdout(&out).WithStderr(&out)
	_, err = r.InstantiateWithConfig(ctx, module, config)
	var exit *sys.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return out.String() + "wasm error: " + err.Error()
	}
	return out.String()
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
	`(apply plus 1 2 3)`,
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
	`(p (apply * 1 2 3 4) (apply / 9 2))`,
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
}

func TestCompiledProgramsPrintWhatTheInterpreterDoes(t *testing.T) {
	for _, program := range wasmPrograms {
		want := runInterpreted(t, program)
		if got := runCompiled(t, program); got != want {
			t.Errorf("%s\ncompiled:    %q\ninterpreted: %q", program, got, want)
		}
	}
}

func TestCompiledExampleMatchesTheInterpreter(t *testing.T) {
	program, err := os.ReadFile("examples/numbers.glisp")
	if err != nil {
		t.Fatal(err)
	}
	want := runInterpreted(t, string(program))
	if got := runCompiled(t, string(program)); got != want {
		t.Errorf("compiled:\n%s\ninterpreted:\n%s", got, want)
	}
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

// Memory grows by doubling, so a program that allocates a lot grows it a few
// times, not once per allocation past the end (which crashed Node's WASI).
func TestCompiledMemoryGrowsByDoubling(t *testing.T) {
	module, err := CompileWasm(`(defn build (n acc) (if (eq n 0) acc (build (- n 1) (cons n acc))))
		(defn churn (k) (if (eq k 0) 0 (plus (car (build 10000 nil)) (churn (- k 1)))))
		(churn 100)`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	var out bytes.Buffer
	mod, err := r.InstantiateWithConfig(ctx, module, wazero.NewModuleConfig().WithStdout(&out).WithStartFunctions())
	if err != nil {
		t.Fatal(err)
	}
	initial := mod.Memory().Size()
	if _, err := mod.ExportedFunction("_start").Call(ctx); err != nil {
		t.Fatal(err)
	}
	final := mod.Memory().Size()
	if out.String() != "=> 100\n" {
		t.Errorf("printed %q", out.String())
	}
	if ratio := final / initial; final <= initial || final%initial != 0 || ratio&(ratio-1) != 0 {
		t.Errorf("memory grew from %d to %d bytes, not by doubling", initial, final)
	}
}

func TestCompilingReportsMalformedForms(t *testing.T) {
	_, err := CompileWasm("(p 1) (def x)")
	if err == nil || !strings.Contains(err.Error(), "def: expects a name and a value") {
		t.Errorf("got %v", err)
	}
}
