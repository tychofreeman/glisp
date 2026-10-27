package glisp

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// String builtins. Indexes and lengths count characters (runes), not bytes.

// expectArgs panics unless the builtin fn got n arguments.
func expectArgs(fn string, params List, n int) {
	if len(params) != n {
		arguments := "arguments"
		if n == 1 {
			arguments = "argument"
		}
		panic(fmt.Sprintf("%s: expects %d %s, got %d", fn, n, arguments, len(params)))
	}
}

// stringArg returns argument i of the builtin fn, which must be a string.
func stringArg(fn string, params List, i int) string {
	if s, isString := params[i].(string); isString {
		return s
	}
	panic(fmt.Sprintf("%s: expected a string, got %s", fn, described(params[i])))
}

// showJoined is (str x ...): its arguments as p shows them, joined.
func showJoined(_ *Scope, params List) interface{} {
	var b strings.Builder
	for _, p := range params {
		b.WriteString(Show(p))
	}
	return b.String()
}

// stringLength is (string_length s): how many characters s has.
func stringLength(_ *Scope, params List) interface{} {
	expectArgs("string_length", params, 1)
	return int64(utf8.RuneCountInString(stringArg("string_length", params, 0)))
}

// substring is (substring s start end): the characters of s from start up
// to, not including, end.
func substring(_ *Scope, params List) interface{} {
	expectArgs("substring", params, 3)
	s := []rune(stringArg("substring", params, 0))
	start, end := number("substring", params[1]), number("substring", params[2])
	if start < 0 || start > end || end > int64(len(s)) {
		panic(fmt.Sprintf("substring: indexes %d..%d out of range for a string of length %d", start, end, len(s)))
	}
	return string(s[start:end])
}

// chars is (chars s): the characters of s, each a one-character string.
func chars(_ *Scope, params List) interface{} {
	expectArgs("chars", params, 1)
	list := List{}
	for _, r := range stringArg("chars", params, 0) {
		list = append(list, string(r))
	}
	return list
}

// stringLess is (string< a b): whether a sorts before b, byte by byte.
func stringLess(_ *Scope, params List) interface{} {
	expectArgs("string<", params, 2)
	return stringArg("string<", params, 0) < stringArg("string<", params, 1)
}

// toSymbol is (symbol s): the symbol named s.
func toSymbol(_ *Scope, params List) interface{} {
	expectArgs("symbol", params, 1)
	return symbol(stringArg("symbol", params, 0))
}

// parseNumber is (parse_number s): the integer s spells, an optional - and
// digits, or nil if it spells something else.
func parseNumber(_ *Scope, params List) interface{} {
	expectArgs("parse_number", params, 1)
	s := stringArg("parse_number", params, 0)
	digits, negative := strings.CutPrefix(s, "-")
	if digits == "" {
		return nil
	}
	var n int64
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return nil
		}
		n = n*10 + int64(digits[i]-'0')
	}
	if negative {
		n = -n
	}
	return n
}
