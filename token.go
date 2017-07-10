package glisp

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

type StringToken struct {
    value string
}

func (s StringToken) Str() string {
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

func (n NumberToken) Type() TokenType {
    return NUM
}

func str_token(s string) StringToken {
    return StringToken{s}
}

func num_token(s string) NumberToken {
    return NumberToken{s}
}
