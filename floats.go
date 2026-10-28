package glisp

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// asFloats returns args as float64s, if any of them is a float: arithmetic
// with a float is done in floats.
func asFloats(args List) ([]float64, bool) {
	floats := false
	for _, arg := range args {
		if _, isFloat := arg.(float64); isFloat {
			floats = true
		}
	}
	if !floats {
		return nil, false
	}
	fs := make([]float64, len(args))
	for i, arg := range args {
		fs[i] = toFloat64(arg)
	}
	return fs, true
}

// toFloat64 is the number n as a float64.
func toFloat64(n interface{}) float64 {
	if i, isInt := n.(int64); isInt {
		return float64(i)
	}
	return n.(float64)
}

// toFloat is (float n).
func toFloat(_ *Scope, args List) interface{} {
	return toFloat64(args[0])
}

// squareRoot is (sqrt n).
func squareRoot(_ *Scope, args List) interface{} {
	return math.Sqrt(toFloat64(args[0]))
}

// rounding returns the builtin (name n): an integer is itself, and a float
// is round(n), which must be in the integers' range.
func rounding(name string, round func(float64) float64) Function {
	return func(_ *Scope, args List) interface{} {
		f, isFloat := args[0].(float64)
		if !isFloat {
			return args[0]
		}
		r := round(f)
		if !(r >= -(1<<62) && r < 1<<62) {
			panic(fmt.Sprintf("%s: %s is out of range for an integer", name, Write(f)))
		}
		return int64(r)
	}
}

// decimalFloat is a float as parse_number reads one: digits, then a
// fraction, an exponent or both, after an optional minus.
var decimalFloat = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+([eE][+-]?[0-9]+)?|[eE][+-]?[0-9]+)$`)

// maxDigits is how many significant digits of a float are read; the rest
// are ignored. It bounds the bignums compiled code needs to read one.
const maxDigits = 40

// readFloat is the float s spells, if it is one as parse_number reads it:
// the float nearest its first maxDigits significant digits.
func readFloat(s string) (float64, bool) {
	if !decimalFloat.MatchString(s) {
		return 0, false
	}
	negative := strings.HasPrefix(s, "-")
	mantissa, exponent, _ := strings.Cut(strings.ToLower(strings.TrimPrefix(s, "-")), "e")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	e := 0
	if exponent != "" {
		digits := strings.TrimLeft(strings.TrimLeft(exponent, "+-"), "0")
		if len(digits) > 6 {
			digits = "999999"
		}
		e, _ = strconv.Atoi(digits)
		if strings.HasPrefix(exponent, "-") {
			e = -e
		}
	}
	digits := strings.TrimLeft(whole+fraction, "0")
	e -= len(fraction)
	if len(digits) > maxDigits {
		e += len(digits) - maxDigits
		digits = digits[:maxDigits]
	}
	f := 0.0
	if digits != "" {
		f, _ = strconv.ParseFloat(digits+"e"+strconv.Itoa(e), 64)
	}
	if negative {
		f = -f
	}
	return f, true
}

// floatBits is f's bits as hash encodes them: one zero and one NaN.
func floatBits(f float64) uint64 {
	switch {
	case f == 0:
		return 0
	case math.IsNaN(f):
		return 0x7ff8000000000000
	}
	return math.Float64bits(f)
}

// formatFloat is f as glisp prints it, as Haskell's show prints a Double:
// the shortest digits that read back as f, with a point from 0.1 up to
// 10^7, and in scientific notation otherwise.
func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	sign := ""
	if math.Signbit(f) {
		sign, f = "-", -f
	}
	if f == 0 {
		return sign + "0.0"
	}
	// f is 0.digits times 10^k.
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	k, _ := strconv.Atoi(exponent)
	k++
	switch {
	case k < 0 || k > 7:
		rest := digits[1:]
		if rest == "" {
			rest = "0"
		}
		return sign + digits[:1] + "." + rest + "e" + strconv.Itoa(k-1)
	case k == 0:
		return sign + "0." + digits
	}
	for len(digits) < k {
		digits += "0"
	}
	fraction := digits[k:]
	if fraction == "" {
		fraction = "0"
	}
	return sign + digits[:k] + "." + fraction
}
