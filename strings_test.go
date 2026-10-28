package glisp

import "testing"

var stringPrograms = []struct{ src, want string }{
	{`(str "a" 1 (quote (b "c")) nil)`, `"a1(b \"c\")nil"`},
	{`(str)`, `""`},
	{`(string_length "héllo")`, `5`},
	{`(string_length "")`, `0`},
	{`(substring "héllo" 1 4)`, `"éll"`},
	{`(substring "abc" 0 3)`, `"abc"`},
	{`(substring "abc" 2 2)`, `""`},
	{`(chars "hé!")`, `("h" "é" "!")`},
	{`(chars "")`, `()`},
	{`(string< "apple" "banana")`, `true`},
	{`(string< "b" "apple")`, `false`},
	{`(string< "app" "apple")`, `true`},
	{`(string< "a" "a")`, `false`},
	{`(symbol "xyz")`, `xyz`},
	{`(eq (symbol "a") (quote a))`, `true`},
	{`(parse_number "-120")`, `-120`},
	{`(parse_number "12a")`, `nil`},
	{`(parse_number "-")`, `nil`},
	{`(parse_number "")`, `nil`},
	{`(eq (str "ab" "c") "abc")`, `true`},
}

func TestStringFunctions(t *testing.T) {
	for _, c := range stringPrograms {
		if got := Write(Process(c.src)); got != c.want {
			t.Errorf("%s = %s, want %s", c.src, got, c.want)
		}
	}
}

func TestStringFunctionErrors(t *testing.T) {
	for src, want := range map[string]string{
		`(string_length 5)`:       `string_length: expected a string, got number 5`,
		`(string_length "a" "b")`: `string_length: expects 1 argument, got 2`,
		`(substring "abc" 2 1)`:   `substring: indexes 2..1 out of range for a string of length 3`,
		`(substring "abc" 0 4)`:   `substring: indexes 0..4 out of range for a string of length 3`,
		`(substring "abc" -1 2)`:  `substring: indexes -1..2 out of range for a string of length 3`,
		`(substring "abc" "a" 2)`: `substring: expected an integer, got string "a"`,
		`(chars (quote a))`:       `chars: expected a string, got symbol a`,
		`(string< "a" 1)`:         `string<: expected a string, got number 1`,
		`(symbol 1)`:              `symbol: expected a string, got number 1`,
		`(parse_number 12)`:       `parse_number: expected a string, got number 12`,
	} {
		assertPanicsWith(t, src, want)
	}
}

// Compiled programs print and fail the same way.
func TestCompiledStringFunctions(t *testing.T) {
	for _, c := range stringPrograms {
		program := "(p " + c.src + ") (str " + c.src + ")"
		assertSameOutput(t, program)
	}
	for _, src := range []string{`(string_length 5)`, `(substring "abc" 0 4)`, `(substring "abc" "a" 2)`, `(chars (quote a))`, `(string< "a" 1)`, `(symbol 1)`, `(parse_number 12)`, `(p (chars "héllo")) (substring "héllo" 2 9)`} {
		assertSameOutput(t, src)
	}
}

func TestCompiledStrBuildsLongStrings(t *testing.T) {
	// Longer than the output buffer, so str can't be building it there.
	program := `(defn rep (s n) (if (eq n 0) "" (str s (rep s (- n 1))))) (string_length (rep "héllo" 2000))`
	if got := runCompiled(t, program); got != "=> 10000\n" {
		t.Errorf("got %q", got)
	}
}
