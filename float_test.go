package glisp

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// ulps is how many floats apart a and b are, or a large number if one is
// NaN and the other isn't.
func ulps(a, b float64) uint64 {
	switch {
	case math.IsNaN(a) || math.IsNaN(b):
		if math.IsNaN(a) && math.IsNaN(b) {
			return 0
		}
		return math.MaxUint64
	case a == b:
		return 0
	case math.Signbit(a) != math.Signbit(b):
		return math.MaxUint64
	}
	x, y := math.Float64bits(a), math.Float64bits(b)
	if x > y {
		return x - y
	}
	return y - x
}

// readShown is a float as glisp prints it.
func readShown(t *testing.T, s string) float64 {
	switch s {
	case "NaN":
		return math.NaN()
	case "Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q isn't a float", s)
	}
	return f
}

// The stdlib's math functions are within a few floats of Go's math package
// (as accurate, for the ones whose algorithm is the same on this machine),
// and print alike compiled.
func TestMathFunctionsAreAccurate(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	uniform := func(lo, hi float64) float64 { return lo + (hi-lo)*r.Float64() }
	anyFloat := func() float64 {
		for {
			if f := math.Float64frombits(r.Uint64()); !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f
			}
		}
	}
	// Go's Log is wrong for subnormals on amd64 (Log(1e-310) is -709.09,
	// not -713.80), so scale them first.
	logOf := func(x float64) float64 {
		if x > 0 && x < 0x1p-1022 {
			return math.Log(x*0x1p54) - 54*math.Ln2
		}
		return math.Log(x)
	}
	type fn struct {
		name   string
		of     func(a []float64) float64
		args   func() []float64
		within uint64
	}
	one := func(f func(float64) float64) func([]float64) float64 {
		return func(a []float64) float64 { return f(a[0]) }
	}
	fns := []fn{
		{"exp", one(math.Exp), func() []float64 { return []float64{uniform(-745, 709)} }, 1},
		{"log", one(logOf), func() []float64 { return []float64{math.Abs(anyFloat())} }, 1},
		{"sin", one(math.Sin), func() []float64 { return []float64{uniform(-1e6, 1e6)} }, 0},
		{"cos", one(math.Cos), func() []float64 { return []float64{uniform(-10, 10)} }, 0},
		{"tan", one(math.Tan), func() []float64 { return []float64{uniform(-1e3, 1e3)} }, 0},
		{"atan", one(math.Atan), func() []float64 { return []float64{anyFloat()} }, 0},
		{"asin", one(math.Asin), func() []float64 { return []float64{uniform(-1, 1)} }, 0},
		{"acos", one(math.Acos), func() []float64 { return []float64{uniform(-1, 1)} }, 0},
		{"atan2", func(a []float64) float64 { return math.Atan2(a[0], a[1]) },
			func() []float64 { return []float64{uniform(-1e3, 1e3), uniform(-1e3, 1e3)} }, 0},
		{"expt", func(a []float64) float64 { return math.Pow(a[0], a[1]) },
			func() []float64 { return []float64{uniform(0, 100), uniform(-150, 150)} }, 3},
	}
	specials := []float64{0, math.Copysign(0, -1), 1, -1, 0.5, 2, math.Inf(1), math.Inf(-1), math.NaN(), 1e-310, 710, -746}
	// A special value as code: the names Write gives NaN and the
	// infinities aren't literals.
	code := func(f float64) string {
		return strings.NewReplacer("-Infinity", "(- math_inf)", "Infinity", "math_inf", "NaN", "(/ 0.0 0)").Replace(Write(f))
	}
	var program strings.Builder
	var cases [][][]float64 // each line's arguments
	for _, f := range fns {
		arity := len(f.args())
		var line [][]float64
		program.WriteString("(p (map (lambda (a) (apply " + f.name + " a)) (list")
		for _, a := range specials {
			for _, b := range specials[:1+(arity-1)*(len(specials)-1)] {
				args := []float64{a, b}[:arity]
				program.WriteString(" (list")
				for _, x := range args {
					program.WriteString(" " + code(x))
				}
				program.WriteString(")")
				line = append(line, args)
			}
		}
		program.WriteString(")))\n(p (map (lambda (a) (apply " + f.name + " a)) (quote (")
		cases = append(cases, line)
		line = nil
		for range 300 {
			args := f.args()
			program.WriteString(" (")
			for _, x := range args {
				program.WriteString(" " + Write(x))
			}
			program.WriteString(")")
			line = append(line, args)
		}
		program.WriteString("))))\n")
		cases = append(cases, line)
	}
	got := strings.Split(runInterpreted(t, program.String()), "\n")
	worst := map[string]uint64{}
	for i, line := range cases {
		f := fns[i/2]
		results := strings.Fields(strings.Trim(got[i], "()"))
		if len(results) != len(line) {
			t.Fatalf("%s: %d results for %d cases: %s", f.name, len(results), len(line), got[i])
		}
		for j, args := range line {
			u := ulps(readShown(t, results[j]), f.of(args))
			worst[f.name] = max(worst[f.name], u)
			if u > f.within {
				t.Errorf("(%s %v) = %s, Go's math gives %v: %d floats apart", f.name, args, results[j], f.of(args), u)
			}
		}
	}
	t.Logf("most floats apart from Go's math: %v", worst)
	if compiled := compiledPrints(t, program.String()); compiled != strings.Join(got, "\n") {
		t.Errorf("compiled, the math functions print differently")
	}
}

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

// parse_number reads a float to the float nearest its first 40 significant
// digits, in both backends: exactly what strconv gives for such strings.
func TestParseNumberReadsFloats(t *testing.T) {
	strs := []string{"1.5", "-0.0", "0.1", "1e5", "1E-5", "2.5e+3", "9007199254740993.0",
		"2.4703282292062327e-324", "2.4703282292062328e-324", "4.9406564584124654e-324",
		"2.2250738585072011e-308", "2.2250738585072014e-308", "1.7976931348623157e308",
		"1.7976931348623158e308", "1.7976931348623159e308", "1e309", "1e-400", "0.000e5",
		"00012.50000", "123456789012345678901234567890.123456789", "1e99999999999", "1e-99999999999"}
	r := rand.New(rand.NewPCG(3, 4))
	for len(strs) < 3000 {
		f := math.Float64frombits(r.Uint64())
		if math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		strs = append(strs, strconv.FormatFloat(f, 'e', -1, 64), strconv.FormatFloat(f, 'e', 20, 64))
		digits := strconv.FormatUint(r.Uint64()>>uint(r.IntN(64)), 10) + strconv.FormatUint(r.Uint64(), 10)
		strs = append(strs, digits[:1+r.IntN(len(digits)-1)]+"."+digits[1:]+"e"+strconv.Itoa(r.IntN(700)-350))
	}
	var program, want strings.Builder
	for i := 0; i < len(strs); i += 10 {
		var shown []string
		program.WriteString("(p")
		for _, s := range strs[i:min(i+10, len(strs))] {
			program.WriteString(" (parse_number " + strconv.Quote(s) + ")")
			f, _ := strconv.ParseFloat(s, 64)
			shown = append(shown, formatFloat(f))
		}
		program.WriteString(")\n")
		want.WriteString(strings.Join(shown, " ") + "\n")
	}
	want.WriteString("=> ()\n")
	got, w := strings.Split(runInterpreted(t, program.String()), "\n"), strings.Split(want.String(), "\n")
	for i := range w {
		if i >= len(got) || got[i] != w[i] {
			t.Fatalf("interpreted, line %d differs from strconv\ngot:  %q\nwant: %q", i+1, lineOf(got, i), w[i])
		}
	}
	got = strings.Split(compiledPrints(t, program.String()), "\n")
	for i := range w {
		if i >= len(got) || got[i] != w[i] {
			t.Fatalf("compiled, line %d differs\ngot:  %q\nwant: %q", i+1, lineOf(got, i), w[i])
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
