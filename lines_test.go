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
