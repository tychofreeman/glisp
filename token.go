package glisp

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type TokenType int8

const (
	STRING TokenType = iota
	NUM
	SYMBOL
	FLOAT
)

type Token interface {
	Str() string
	Type() TokenType
}

func token(s string) Token {
	if strings.HasPrefix(s, "\"") {
		return StringToken{s[1 : len(s)-1]}
	} else if _, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
		return NumberToken{s}
	} else if floatLiteral.MatchString(s) {
		return FloatToken{s}
	}
	return Symbol{s}
}

// floatLiteral is a float as a program writes one: digits with a fraction,
// an exponent or both.
var floatLiteral = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+([eE][+-]?[0-9]+)?|[eE][+-]?[0-9]+)$`)

// A FloatToken is a float literal.
type FloatToken struct {
	value string
}

func (f FloatToken) Str() string     { return f.value }
func (f FloatToken) Type() TokenType { return FLOAT }

// Value is the float nearest the literal: out of range, an infinity or 0.
func (f FloatToken) Value() float64 {
	v, _ := strconv.ParseFloat(f.value, 64)
	return v
}

type StringToken struct {
	value string
}

func (s StringToken) Str() string {
	return s.value
}

func (s StringToken) Value() string {
	return s.value
}

func (s StringToken) Equals(other interface{}) (bool, string) {
	switch o := other.(type) {
	case string:
		return o == s.value, fmt.Sprintf("Expected %v, actual %v\n", s.value, o)
	case StringToken:
		return o.value == s.value, fmt.Sprintf("Expected %v, actual %v\n", s.value, o.value)
	case reflect.Value:
		if o.CanInterface() {
			return s.Equals(o.Interface())
		}
		return false, fmt.Sprintf("Cannot coerce Value %v to string (%v)\n", o, s.Value())
	default:
		return false, fmt.Sprintf("Expected string (%v), found %T %v\n", s.value, o, o)
	}
}

func (s StringToken) Type() TokenType {
	return STRING
}

type NumberToken struct {
	value string
}

func (n NumberToken) Str() string {
	return n.value
}

func (n NumberToken) Value() int64 {
	num, err := strconv.ParseInt(strings.TrimSpace(n.value), 10, 64)
	if err == nil {
		return int63(num)
	}
	panic(fmt.Sprintf("Could not parse %v as a number!!", n.value))
}

func (n NumberToken) Type() TokenType {
	return NUM
}

func (n NumberToken) Equals(other interface{}) (bool, string) {
	switch o := other.(type) {
	case NumberToken:
		return o.Value() == n.Value(), fmt.Sprintf("Expected %v, found %v\n", n.Value(), o.Value())
	case int64:
		return o == n.Value(), fmt.Sprintf("Expected %v, found %v\n", n.Value(), o)
	case reflect.Value:
		if o.CanInterface() {
			return n.Equals(o.Interface())
		}
		return false, fmt.Sprintf("Cannot coerce Value %v to int (%v)\n", o, n.Value())
	default:
		return false, fmt.Sprintf("Expected number (%v), found %T %v\n", n.Value(), o, o)
	}
}

func num(n int64) NumberToken {
	return NumberToken{fmt.Sprintf("%v", n)}
}

func str(s string) StringToken {
	return StringToken{s}
}
