package glisp

import (
	"strings"
	"testing"
)

// resolveProgram expands and resolves each top-level form of src, and
// returns them as core glisp, with Locs as name@depth.index and each
// lambda's captured variables in [brackets].
func resolveProgram(src string) string {
	e := NewExpander(newScope())
	var out []string
	for _, form := range TokenizeString(src) {
		out = append(out, Resolve(e.Expand(Read(form))).String())
	}
	return strings.Join(out, " ")
}

func TestResolvingVariables(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"top-level names are found by name",
			`(def x 1) x`, `(def x 1) x`},
		{"parameters",
			`(lambda (a b) (plus a b))`, `(lambda (a b) (plus a@0.0 b@0.1))`},
		{"a variable from the enclosing lambda is captured",
			`(lambda (x) (lambda (y) (plus x y)))`, `(lambda (x) (lambda (y) [x] (plus x@1.0 y@0.0)))`},
		{"captures pass through every lambda in between",
			`(lambda (a) (lambda (b) (lambda (c) a)))`, `(lambda (a) (lambda (b) [a] (lambda (c) [a] a@2.0)))`},
		{"local defs get slots after the parameters",
			`(lambda (x) (def y 2) (plus x y))`, `(lambda (x) (def y 2) (plus x@0.0 y@0.1))`},
		{"a def inside an if is local too",
			`(lambda () (if true (def z 1) nil) z)`, `(lambda () (if true (def z 1) nil) z@0.0)`},
		{"an inner parameter shadows an outer one",
			`(lambda (x) (lambda (x) x))`, `(lambda (x) (lambda (x) x@0.0))`},
		{"a recursive defn refers to itself by name",
			`(defn f (n) (if (< n 1) 0 (f (- n 1))))`, `(def f (lambda (n) (if (< n@0.0 1) 0 (f (- n@0.0 1)))))`},
	} {
		if got := resolveProgram(c.src); got != c.want {
			t.Errorf("%s: %s\n got %s\nwant %s", c.name, c.src, got, c.want)
		}
	}
}
