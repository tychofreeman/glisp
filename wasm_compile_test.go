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
	module, err := CompileWasm(program)
	return compiledOutput(t, module, err)
}

// compiledOutput runs a compiled module, or reports why it didn't compile.
func compiledOutput(t *testing.T, module []byte, err error) string {
	t.Helper()
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
// interpreted, as cmd/glisp shows it, reporting the first line where they
// differ.
func assertSameOutput(t *testing.T, program string) {
	t.Helper()
	got, want := compiledPrints(t, program), runInterpreted(t, program)
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
	return printedBy(t, func() (interface{}, error) { return Run(program) })
}

// printedBy runs a program with run, and returns what it printed and its
// value, or its error, as the glisp command shows them.
func printedBy(t *testing.T, run func() (interface{}, error)) string {
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
	value, err := run()
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
	`(p (parse_number "1.5") (parse_number "-0.0") (parse_number "1e3") (parse_number "-2.5E-2"))`,
	`(p (parse_number ".5") (parse_number "1.") (parse_number "+1.5") (parse_number "1e") (parse_number "1e+") (parse_number "1.5x") (parse_number "-") (parse_number "1.2.3"))`,
	`(p (parse_number "0.1000000000000000000000000000000000000000000000001") (parse_number "1e400") (parse_number "-1e-400"))`,
	`(* 3037000500 3037000500)`,
	`(vector 1 (quote (a b)) "s")`,
	`(vector)`,
	`(vector_ref (vector 1 2 3) 1)`,
	`(vector_length (vector 1 2 3))`,
	`(let (v (vector 1 2)) (cons (vector_set v 0 9) (cons v nil)))`,
	`(vector_to_list (vector 1 2))`,
	`(apply vector (quote (1 2 3)))`,
	`(eq (vector 1 (quote (2))) (vector 1 (quote (2))))`,
	`(eq (vector 1) (vector 2))`,
	`(eq (vector 1) (vector 1 2))`,
	`(eq (vector) (quote ()))`,
	`(vector? (vector))`,
	`(vector? (quote ()))`,
	`(atom (vector))`,
	`(list? (vector))`,
	`(vector_ref (vector 1) 1)`,
	`(vector_ref (vector 1) -1)`,
	`(vector_set (vector) 0 1)`,
	`(vector_ref (quote (1)) 0)`,
	`(vector_length 5)`,
	`(p (vector "a" (vector 1 (quote b))) (vector))`,
	`(defn fill (n acc) (if (eq n 0) acc (fill (- n 1) (cons (vector n (quote (x)) "y") acc)))) (defn total (l s) (if (eq l (quote ())) s (total (cdr l) (plus s (vector_ref (car l) 0))))) (total (fill 2000 nil) 0)`,
	`(defn grow (v n) (if (eq n 0) v (grow (vector_set v (mod n 3) (vector n)) (- n 1)))) (grow (vector 0 0 0) 500)`,
	`(p (hash 0) (hash -1) (hash 4611686018427387903) (hash nil) (hash true) (hash false))`,
	`(p (hash "") (hash "abc") (hash "héllo") (hash (quote abc)) (hash (quote ())))`,
	`(p (hash (quote (1 2))) (hash (quote (1 (2 "x") b))) (hash (vector)) (hash (vector 1 (quote (a)))))`,
	`(eq (hash (quote (1 2))) (hash (cons 1 (cons 2 nil))))`,
	`(hash car)`,
	`(hash (cons 1 (cons car nil)))`,
	`(defn upto (n acc) (if (eq n 0) acc (upto (- n 1) (cons (vector n "s") acc)))) (hash (upto 3000 nil))`,
	`(hash_map)`,
	`(hash_map 1 2)`,
	`(map_get (hash_map 1 2) 1)`,
	`(map_get (hash_map 1 2) 3)`,
	`(map_get (hash_map) 1 9)`,
	`(map_get (hash_map 1 2 1 5) 1)`,
	`(map_count (hash_map 1 2 3 4 1 5))`,
	`(map_has (hash_map "a" 1) "a")`,
	`(map_has (hash_map "a" 1) "b")`,
	`(map_count (map_remove (hash_map 1 2 3 4) 1))`,
	`(map_count (map_remove (hash_map 1 2) 7))`,
	`(map_get (hash_map (quote (a b)) 1 (vector 1) 2) (quote (a b)))`,
	`(map_get (hash_map (quote (a b)) 1 (vector 1) 2) (vector 1))`,
	`(map_keys (hash_map (quote (a b)) 1))`,
	`(map_to_list (hash_map "k" (quote v)))`,
	`(let (m (hash_map 1 2)) (cons (map_put m 1 3) (cons m nil)))`,
	`(eq (hash_map 1 2 3 4) (hash_map 3 4 1 2))`,
	`(eq (hash_map 1 2) (hash_map 1 3))`,
	`(eq (hash_map 1 2) (hash_map 1 2 3 4))`,
	`(eq (hash_map) (vector))`,
	`(map? (hash_map))`,
	`(map? (vector))`,
	`(hash_map 1)`,
	`(hash_map 1 2 3)`,
	`(map_put (hash_map) car 1)`,
	`(map_get 5 1)`,
	`(map_get (hash_map) 1 2 3)`,
	`(hash (hash_map))`,
	`(map car nil)`,
	`(map list 5)`,
	`(filter (lambda (x) x) (list true 1 true))`,
	`((compose) 5)`,
	`((compose car cdr) (list 1 2 3))`,
	`((partial list 1) 2 3)`,
	`(foldr list 0 (list 1 2))`,
	`(append nil (list 1))`,
	`(length (quote ()))`,
	`(begin)`,
	`(begin (def a 1) (def b (plus a 1))) (p a b) (begin 7)`,
	`((lambda () (begin (def x 1) (def y 2)) (plus x y)))`,
	`(p (begin 1 2)) ((lambda () 1 (begin)))`,
	`(data (quote a) 1 (quote (2 3)) "s" (vector 4) (hash_map 5 6))`,
	`(p (data_length (data (quote a))) (data_name (data (quote b) 1)))`,
	`(data_ref (data (quote a) 1) -1)`,
	`(data_name 5)`,
	`(data)`,
	`(eq (data (quote a) 1) (data (quote b) 1))`,
	`(eq (data (quote a) 1) (data (quote a) 1 2))`,
	`(hash (data (quote a) 1 (data (quote b))))`,
	`(eq (hash (data (quote a))) (hash (quote a)))`,
	`(deftype t (k a b) (z)) (p (k 1 2) (z) (k? (z)) (z? (z)) (t? 3) (k_b (k 1 2)))`,
	`(deftype t (apple a)) (apple_a 5)`,
	`(deftype 5 (k))`,
	`(deftype t k)`,
	`(p (cons? (list 1)) (cons? nil) (cons? (quote ())) (cons? 5))`,
	`(match (list 1 (vector 2 (list 3)) (data (quote k) 4)) ((list a (vector b (cons c _)) (k d)) (list a b c d)))`,
	`(match (list 1 2) ((list a) a) ((list a b c) a) (x (length x)))`,
	`(match (vector 1 2 3) ((vector a b) a) ((vector a b c) c))`,
	`(match (data (quote k) 1 2) ((k a) a) ((k a b) b))`,
	`(match (quote ()) (nil 1) (() 2))`,
	`(match nil (nil 1) (() 2))`,
	`(match false (true 1) (false 2))`,
	`(match "s" ("t" 1) ("s" 2))`,
	`(defn count (n acc) (match n (0 acc) (_ (count (- n 1) (plus acc 1))))) (count 100000 0)`,
	`(match 1 ((list a a) a))`,
	`(match 1 (5.5 1))`,
	`(match 1 ((cons a) a))`,
	`(match 1 7)`,
	`(match)`,
	`(match 1 (x))`,
	`(p "before") (match (list 1) ((cons h (cons t _)) t))`,
	`(p (just 1) (nothing) (left 2) (right 3) (maybe? (nothing)) (either? (just 1)) (just_value (just 4)))`,
	`(p (bind (nothing) car) (bind (left 1) car) (bind (just (list 5)) car) (bind (right 6) list))`,
	`(p (bind nil list) (bind (quote ()) list) (bind (list 1 2) (lambda (x) (list x x))))`,
	`(do (just 1) (nothing) (just 2))`,
	`(do (just 1) (just 2))`,
	`(do (x <- (right 1)) (left x) (right 9))`,
	`(do (x <- (just 1)))`,
	`(do)`,
	`(do (5 <- (just 1)) (just 2))`,
	`(do (x <- (just 1) 2) (just x))`,
	`(do (x <- 7) (just x))`,
	`(defn let_expand (a b) 5) (defn fold (a b c) 6) (let (x 1) (match (list x) ((list y) (plus x y))))`,
	`1.5`,
	`-0.0`,
	`(p 0.1 1.0 100.0 1234567.0 12345678.0 0.01 1e21 5e-324 1.7976931348623157e308 -2.5e-7)`,
	`(p (plus 1 2.5) (plus 0.1 0.2) (- 1.5) (- 0.0) (- 10 0.5 1) (* 2 1.5) (/ 7 2.0) (/ 7 2))`,
	`(p (/ 1.0 0) (/ -1 0.0) (/ 0.0 0) (* 1e308 10) (- (* 1e308 10)))`,
	`(p (< 1 1.5) (>= 2.0 2) (< (/ 0.0 0) 1) (> 1e300 4611686018427387903))`,
	`(p (eq 1 1.0) (eq 0.0 -0.0) (eq 1.5 1.5) (eq (/ 0.0 0) (/ 0.0 0)) (eq (list 1.5) (list 1.5)))`,
	`(p (float 3) (float 2.5) (floor 2.5) (ceiling 2.5) (truncate -2.5) (round 2.5) (round 3.5) (round -2.5) (floor 7))`,
	`(p (floor -0.5) (ceiling -0.5) (round 0.5000000000000001) (floor 4611686018427387000.0))`,
	`(floor 1e300)`,
	`(round (/ 0.0 0))`,
	`(ceiling 4.611686018427388e18)`,
	`(p (sqrt 2) (sqrt -1) (sqrt 16.0) (sqrt 0))`,
	`(p (number? 1.5) (float? 1.5) (float? 1) (integer? 1) (integer? 1.5) (number? (quote a)))`,
	`(p (hash 1.5) (eq (hash 0.0) (hash -0.0)) (map_get (hash_map 1.5 "a" 1 "b") 1.5) (map_get (hash_map 1 "b") 1.0))`,
	`(p (str 1.5 " " 2e-3) (quote (1.5 a)) (vector 2.5) (data (quote a) 2.5))`,
	`(vector_ref (vector 1) 0.0)`,
	`(mod 5.0 2)`,
	`(car 1.5)`,
	`(plus 1 "a" 2.5)`,
	`(defn sum (n acc) (if (eq n 0) acc (sum (- n 1) (plus acc 0.5)))) (sum 1000 0.0)`,
	`"a\"b"`,
	`(p "x\ny" (list "t\tz" "q\"" "b\\") (string_length "a\\b") (chars "\"\n"))`,
	`(p (eq "\t" (substring "a\tb" 1 2)) (str "\\" 1))`,
	`(p 1) "bad \q"`,
	`(p 1) "end\`,
	`"a ; not a comment\" still in the string"`,
	`(map_keys (hash_map "a" 1 (quote b) 2 (quote (c)) 3 (vector 4) 5 nil 6 true 7 -9 8))`,
	`(p (hash_map "a" (hash_map 1 (vector 2)) (quote k) (quote (x y))))`,
	`(defn step (m n x) (if (eq n 0) m (let (k (mod x 300) y (mod (plus (* x 25173) 13849) 65536)) (step (if (eq (mod y 3) 0) (map_remove m k) (map_put m k y)) (- n 1) y)))) (def m (step (hash_map) 3000 7)) (p (map_count m) (map_to_list m)) m`,
	`(defn fill (m n) (if (eq n 0) m (fill (map_put m n (* n n)) (- n 1)))) (defn drain (m n) (if (eq n 0) m (drain (map_remove m n) (- n 2)))) (def m (drain (fill (hash_map) 2000) 2000)) (p (map_count m) (map_get m 1999) (map_get m 1998) (eq m (fill (hash_map) 0)))`,
	`(defn evens (m n) (if (eq n 0) m (evens (map_put m (* 2 n) n) (- n 1)))) (eq (evens (hash_map) 500) (apply map (map_to_list (evens (hash_map) 500))))`,
	`(defn pairs (n x acc) (if (eq n 0) acc (let (y (mod (plus (* x 25173) 13849) 65536)) (pairs (- n 1) y (cons (mod y 300) (cons n acc)))))) (def m (apply hash_map (pairs 3000 7 nil))) (def m2 (map_put m 5 (quote x))) (p (map_count m) (map_to_list m)) (p (map_get m 5) (map_get m2 5)) (eq m (apply hash_map (pairs 3000 7 nil)))`,
	`(deftype point (pt x y)) (definstance show ((x point?)) (str "<" (pt_x x) " " (pt_y x) ">")) (defgeneric size (x) x) (definstance size ((x list?)) (length x)) (p (fmap (lambda (x) (* x 2)) (list 1 2)) (fmap car (vector (list 1))) (fmap (lambda (x) x) (hash_map 1 2)) (fmap (lambda (x) 0) (just 1))) (p (show (list (pt 1 2) "s" (vector (pt 3 4)) (hash_map (quote k) (pt 5 6))))) (p (compare (list 1 (vector "a")) (list 1 (vector "b"))) (compare (nothing) (just 1)) (compare (pt 1 2) (pt 1 1)) (size (list 1 2))) (size 7)`,
	`(defn rand (n x acc) (if (eq n 0) acc (let (y (mod (plus (* x 25173) 13849) 65536)) (rand (- n 1) y (cons (list (mod y 50) n) acc))))) (def xs (rand 600 7 nil)) (def by_first (lambda (a b) (compare (car a) (car b)))) (p (sort (map car xs))) (sort_by by_first xs)`,
	`(p (letrec (ev? (lambda (n) (if (eq n 0) true (od? (- n 1)))) od? (lambda (n) (if (eq n 0) false (ev? (- n 1))))) (list (ev? 100001) (od? 100001)))) (p (letrec (x 1 y (plus x 1)) (list x y)))`,
	`(letrec (f) 1)`,
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

// A module leaves out the runtime its program can't reach: the builtins it
// never names, and what handles kinds of value it never makes: using one
// grows the module by its runtime.
func TestModulesLeaveOutWhatTheyCannotReach(t *testing.T) {
	size := func(program string) int {
		module, err := CompileWasm(program)
		if err != nil {
			t.Fatal(err)
		}
		return len(module)
	}
	for _, c := range []struct {
		use   string
		least int
	}{
		{`(hash_map 1 2)`, 3000},
		{`(vector 1)`, 300},
		{`(data (quote a) 1)`, 300},
		{`(str 1 "a")`, 300},
	} {
		if grown := size(`(p 1) `+c.use) - size(`(p 1)`); grown < c.least {
			t.Errorf("%s adds %d bytes to (p 1); was its runtime there already?", c.use, grown)
		}
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
