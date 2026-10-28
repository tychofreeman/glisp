package glisp

import (
	"fmt"
	"strconv"
	"strings"
)

// Show returns v as p prints it: a string as its text, anything else as
// Write does.
func Show(v interface{}) string {
	if s, isString := v.(string); isString {
		return s
	}
	return Write(v)
}

// Write returns v as it would be written in glisp: lists in parentheses,
// symbols by name, strings in quotes.
func Write(v interface{}) string {
	switch v := v.(type) {
	case List:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = Write(item)
		}
		return "(" + strings.Join(parts, " ") + ")"
	case Vector:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = Write(item)
		}
		return "[" + strings.Join(parts, " ") + "]"
	case Map:
		var parts []string
		v.each(func(key, value interface{}) { parts = append(parts, Write(key)+" "+Write(value)) })
		return "{" + strings.Join(parts, ", ") + "}"
	case Symbol:
		return v.Str()
	case nil:
		return "nil"
	case string:
		return quote(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		return strconv.FormatBool(v)
	case Function, *Closure:
		return "<function>"
	case Macro:
		return "<macro>"
	}
	return fmt.Sprint(v)
}

// quote returns s in double quotes, with ", \, newline and tab escaped.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// typeName is the glisp name of v's type, for error messages.
func typeName(v interface{}) string {
	switch v.(type) {
	case int64:
		return "number"
	case string:
		return "string"
	case Symbol:
		return "symbol"
	case List:
		return "list"
	case Vector:
		return "vector"
	case Map:
		return "map"
	case Function, *Closure:
		return "function"
	case Macro:
		return "macro"
	case bool:
		return "bool"
	case nil:
		return "nil"
	}
	return fmt.Sprintf("%T", v)
}

// described is v's type and value, as errors show them: number 5,
// list (1 2), or just nil.
func described(v interface{}) string {
	if v == nil {
		return "nil"
	}
	return typeName(v) + " " + Write(v)
}
