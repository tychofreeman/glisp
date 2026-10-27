package glisp

import (
	"fmt"
	"strings"
	"testing"
)

// expandProgram reads and expands each top-level form of src, after the
// stdlib's, and returns the expanded forms as core glisp.
func expandProgram(src string) []string {
	e := NewExpander(&Scope{prev: builtinScope, table: map[string]interface{}{}})
	for _, form := range TokenizeString(stdlib) {
		e.Expand(Read(form))
	}
	var out []string
	for _, form := range TokenizeString(src) {
		out = append(out, e.Expand(Read(form)).String())
	}
	return out
}

func TestExpandingCoreForms(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`42`, `42`},
		{`"s"`, `"s"`},
		{`x`, `x`},
		{`()`, `nil`},
		{`(quote (a 1 "s"))`, `(quote (a 1 "s"))`},
		{`(if c 1 2)`, `(if c 1 2)`},
		{`(lambda (a b) (plus a b) a)`, `(lambda (a b) (plus a b) a)`},
		{`(def x (plus 1 2))`, `(def x (plus 1 2))`},
		{`((f 1) 2)`, `((f 1) 2)`},
	} {
		if got := strings.Join(expandProgram(c.src), " "); got != c.want {
			t.Errorf("%s: got %s, want %s", c.src, got, c.want)
		}
	}
}

func TestExpandingMacros(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`(defn inc (n) (plus n 1))`, `(def inc (lambda (n) (plus n 1)))`},
		{`(lambda (x) (defn g (y) y))`, `(lambda (x) (def g (lambda (y) y)))`},
		{unlessMacro + `(unless (eq a b) 1 2)`, `(quote ()) (if (eq a b) 2 1)`},
		{`(defmacro twice (lambda (xs) (cons (quote do2) (cons (car xs) (cons (car xs) nil))))) (twice (defn f () 1))`,
			`(quote ()) (do2 (def f (lambda () 1)) (def f (lambda () 1)))`},
	} {
		if got := strings.Join(expandProgram(c.src), " "); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestExpandingReportsMalformedForms(t *testing.T) {
	for src, want := range map[string]string{
		`(if 1 2)`:       "IF requires 3 parts",
		`(quote)`:        "QUOTE takes exactly 1 argument",
		`(lambda)`:       "lambda: expects a parameter list",
		`(lambda (1) 1)`: "lambda: parameters must be symbols, got int64 1",
		`(def 1 2)`:      "def: expected a symbol to define, got int64 1",
		`(def x 1 2)`:    "def: expects a name and a value, got 3 arguments",
		`(defmacro m 5)`: "defmacro: expected a function, got int64 5",
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), want) {
					t.Errorf("%s: got panic %v, want one containing %q", src, r, want)
				}
			}()
			expandProgram(src)
		}()
	}
}
