package glisp

import "testing"

func TestValuesPrintTheWayTheyAreWritten(t *testing.T) {
	cases := []struct {
		value       interface{}
		write, show string
	}{
		{int64(-5), "-5", "-5"},
		{"a \"b\"\\\n\t", `"a \"b\"\\\n\t"`, "a \"b\"\\\n\t"},
		{symbol("x"), "x", "x"},
		{nil, "nil", "nil"},
		{true, "true", "true"},
		{List{}, "()", "()"},
		{List{int64(1), List{symbol("a"), "s"}, nil}, `(1 (a "s") nil)`, `(1 (a "s") nil)`},
		{Function(car), "<function>", "<function>"},
	}
	for _, c := range cases {
		if got := Write(c.value); got != c.write {
			t.Errorf("Write(%#v) = %s, want %s", c.value, got, c.write)
		}
		if got := Show(c.value); got != c.show {
			t.Errorf("Show(%#v) = %s, want %s", c.value, got, c.show)
		}
	}
}

func TestPShowsItsArguments(t *testing.T) {
	program := `(p "x:" (quote (a "b c" ())) nil 5) (quote (1 "two"))`
	want := "x: (a \"b c\" ()) nil 5\n=> (1 \"two\")\n"
	if got := runInterpreted(t, program); got != want {
		t.Errorf("interpreted: got %q, want %q", got, want)
	}
	if got := runCompiled(t, program); got != want {
		t.Errorf("compiled: got %q, want %q", got, want)
	}
}

func TestErrorsNameGlispTypes(t *testing.T) {
	program := `(car (quote (a "b")))  (plus 1 (quote (a "b")))`
	want := "glisp: plus: expected a number, got list (a \"b\")\n"
	if got := runInterpreted(t, program); got != want {
		t.Errorf("interpreted: got %q, want %q", got, want)
	}
	if got := runCompiled(t, program); got != want {
		t.Errorf("compiled: got %q, want %q", got, want)
	}
}
