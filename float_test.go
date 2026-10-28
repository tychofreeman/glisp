package glisp

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// Floats print as Haskell's show prints a Double.
func TestFloatsPrintAsHaskellShows(t *testing.T) {
	for _, c := range []struct {
		f    float64
		want string
	}{
		{0.1, "0.1"}, {0.5, "0.5"}, {1, "1.0"}, {100, "100.0"}, {123.456, "123.456"},
		{1234567, "1234567.0"}, {9999999, "9999999.0"}, {1e7, "1.0e7"}, {12345678, "1.2345678e7"},
		{0.099, "9.9e-2"}, {0.01, "1.0e-2"}, {1e21, "1.0e21"}, {0, "0.0"},
		{math.Copysign(0, -1), "-0.0"}, {-1.5, "-1.5"}, {-0.001, "-1.0e-3"},
		{0.30000000000000004, "0.30000000000000004"}, {5e-324, "5.0e-324"},
		{math.MaxFloat64, "1.7976931348623157e308"}, {math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"}, {math.NaN(), "NaN"},
	} {
		if got := formatFloat(c.f); got != c.want {
			t.Errorf("formatFloat(%v) = %q, want %q", c.f, got, c.want)
		}
	}
}

// Compiled code prints floats with an algorithm of its own (exact
// arithmetic on bignums, after Burger and Dybvig); the interpreter with
// strconv's shortest digits. They must agree on every float.
func TestFloatsPrintTheSameCompiled(t *testing.T) {
	values := []float64{5e-324, 1e-323, 2.2250738585072014e-308, 2.225073858507201e-308,
		math.MaxFloat64, math.SmallestNonzeroFloat64, 1 << 52, 1<<53 + 2, 9007199254740993,
		0.1, 0.2, 0.3, 1.0 / 3, 2.0 / 3, 123456789012345680, 1e23, 8.41e21, 5e-310}
	for e := -1074; e <= 1023; e += 7 {
		values = append(values, math.Ldexp(1, e))
	}
	for e := -323; e <= 308; e += 3 {
		values = append(values, math.Pow(10, float64(e)))
	}
	r := rand.New(rand.NewPCG(1, 2))
	for len(values) < 3000 {
		f := math.Float64frombits(r.Uint64())
		if !math.IsNaN(f) && !math.IsInf(f, 0) {
			values = append(values, f)
		}
		values = append(values, float64(r.IntN(2000000))/1000)
	}
	var program, want strings.Builder
	for i := 0; i < len(values); i += 10 {
		var shown []string
		program.WriteString("(p")
		for _, f := range values[i:min(i+10, len(values))] {
			program.WriteString(" " + strconv.FormatFloat(f, 'e', -1, 64))
			shown = append(shown, formatFloat(f))
		}
		program.WriteString(")\n")
		want.WriteString(strings.Join(shown, " ") + "\n")
	}
	want.WriteString("=> ()\n")
	if got := runInterpreted(t, program.String()); got != want.String() {
		t.Fatalf("interpreted, the floats print differently from formatFloat")
	}
	got, w := strings.Split(compiledPrints(t, program.String()), "\n"), strings.Split(want.String(), "\n")
	for i := range w {
		if i >= len(got) || got[i] != w[i] {
			t.Fatalf("line %d differs\ncompiled:    %q\ninterpreted: %q", i+1, lineOf(got, i), w[i])
		}
	}
}
