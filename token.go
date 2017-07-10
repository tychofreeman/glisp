package glisp

import (
    "strings"
    "strconv"
    "fmt"
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

func str_token(s string) StringToken {
    return StringToken{s}
}

func num_token(s string) NumberToken {
    return NumberToken{s}
}
