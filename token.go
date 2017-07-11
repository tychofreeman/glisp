package glisp

import (
    "strings"
    "strconv"
    "fmt"
    "reflect"
)

type TokenType int8
const (
    STRING TokenType = iota
    NUM
    SYMBOL
)

type Token interface {
    Str() string
    Type() TokenType
}

func token(s string) Token {
    if strings.HasPrefix(s, "\"") {
        return StringToken{s[1:len(s)-1]}
    } else if _, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
        return NumberToken{s}
    }
    return Symbol{s}
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

func (s StringToken) Eval(scope *Scope) interface{} {
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
        return num
    }
    panic(fmt.Sprintf("Could not parse %v as a number!!", n.value))
}

func (n NumberToken) Eval(scope *Scope) interface{} {
    return n.Value()
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

func str_token(s string) StringToken {
    return StringToken{s}
}

func num_token(s string) NumberToken {
    return NumberToken{s}
}
