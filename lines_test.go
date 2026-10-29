package glisp

import "testing"

var lineErrors = []struct{ src, want string }{
	{"(p 1)\n(car 5)", "line 2: car: expected a list, got number 5"},
	{"(defn f (x)\n  (car x))\n\n(f 5)", "line 2: car: expected a list, got number 5"},
	{"(defn f (x) x)\n(p 1)\n(f\n 1\n 2)", "line 3: lambda (x) expects 1 argument, got 2: (1 2)"},
	{"(def x 1)\n(plus x\n  (car\n    \"a\"))", "line 3: car: expected a list, got string \"a\""},
	{"(defn ev (n) (if (eq n 0) (car n) (od (- n 1))))\n(defn od (n)\n  (ev (- n 1)))\n(ev 10)", "line 1: car: expected a list, got number 0"},
	{"(p 1)\n\n(let (a 1)\n  (cdr a))", "line 4: cdr: expected a list, got number 1"},
	{"; a comment\n(p \"two\nlines\")\n(car 1)", "line 4: car: expected a list, got number 1"},
	// The stdlib's fmap fails after calling a show instance from line 1,
	// whose call has returned: the call being made is still line 3's.
	{"(def x 1)\n(module geometry)", "line 2: module: must be the first form of a file"},
	{"(module 5)", "line 1: module: expected (module name (export name ...)), got (module 5)"},
	{"(module geometry (export area volume))\n(defn area (w h) (* w h))", "line 1: module geometry exports volume, which it doesn't define"},
	{"(def x 1)\n(p (plus x\n  (other/thing 1)))", "line 3: other/thing: no module other is imported"},
	{"(module geometry)\n(def geometry/x 1)", "line 2: def: can't define a qualified name, geometry/x"},
	{"(def x 1)\n(import geometry)", "line 2: import: no module geometry: a program not read from a file can import only the stdlib's modules, such as math"},
	{"(def x 1)\n(p (sin 0))", "line 2: Cannot resolve symbol sin"},
	{"(import geometry as)", "line 1: import: expected (import name), (import name (name ...)) or (import name as alias), got (import geometry as)"},
	{"(definstance show ((x vector?))\n  (str \"v\"))\n(fmap car \"abc\")", "line 3: fmap: no instance for \"abc\""},
}

func TestRunErrorsGiveTheirLine(t *testing.T) {
	for _, c := range lineErrors {
		if _, err := Run(c.src); err == nil || err.Error() != c.want {
			t.Errorf("%q: got %v, want %q", c.src, err, c.want)
		}
	}
}

func TestParseErrorsGiveTheirLine(t *testing.T) {
	for src, want := range map[string]string{
		"(p 1)\n)":       "line 2: unexpected )",
		"(p 1)\n(p\n  2": "line 3: missing )",
		"(p 1)\n(p \"ab": "line 2: unterminated string: \"ab",
	} {
		if _, err := Run(src); err == nil || err.Error() != want {
			t.Errorf("%q: got %v, want %q", src, err, want)
		}
	}
}

func TestCompiledErrorsGiveTheSameLine(t *testing.T) {
	for _, c := range lineErrors {
		assertSameOutput(t, c.src)
	}
}
