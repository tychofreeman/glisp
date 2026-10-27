package glisp

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// String builtins. Indexes and lengths count characters (runes), not bytes.

// showJoined is (str x ...): its arguments as p shows them, joined.
func showJoined(_ *Scope, params List) interface{} {
	var b strings.Builder
	for _, p := range params {
		b.WriteString(Show(p))
	}
	return b.String()
}

// stringLength is (string_length s): how many characters s has.
func stringLength(_ *Scope, args List) interface{} {
	return int64(utf8.RuneCountInString(args[0].(string)))
}

// substring is (substring s start end): the characters of s from start up
// to, not including, end.
func substring(_ *Scope, args List) interface{} {
	s := []rune(args[0].(string))
	start, end := args[1].(int64), args[2].(int64)
	if start < 0 || start > end || end > int64(len(s)) {
		panic(fmt.Sprintf("substring: indexes %d..%d out of range for a string of length %d", start, end, len(s)))
	}
	return string(s[start:end])
}

// chars is (chars s): the characters of s, each a one-character string.
func chars(_ *Scope, args List) interface{} {
	list := List{}
	for _, r := range args[0].(string) {
		list = append(list, string(r))
	}
	return list
}

// stringLess is (string< a b): whether a sorts before b, byte by byte.
func stringLess(_ *Scope, args List) interface{} {
	return args[0].(string) < args[1].(string)
}

// toSymbol is (symbol s): the symbol named s.
func toSymbol(_ *Scope, args List) interface{} {
	return symbol(args[0].(string))
}

// parseNumber is (parse_number s): the integer s spells, an optional - and
// digits, or nil if it spells something else.
func parseNumber(_ *Scope, args List) interface{} {
	digits, negative := strings.CutPrefix(args[0].(string), "-")
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
