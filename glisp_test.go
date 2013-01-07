package glisp

import (
    "testing"
    //. "github.com/tychofreeman/go-matchers"
    . "matchers"
)

func caar(in interface{}) interface{} {
    return car(car(in))
}

func car(in interface{}) interface{} {
    if in == nil {
        return nil
    }
    switch t := in.(type) {
        case []interface{}:
            return t[0];
    }
    return nil
}

func cdr(in interface{}) interface{} {
    if in == nil {
        return nil
    }
    switch t := in.(type) {
        case []interface{}:
            if len(t) > 0 {
                return t[1:]
            }
    }
    return nil
}

func cadr(in []interface{}) interface{} {
    return cdr(car(in))
}

func atom(in interface{}) bool {
    switch in.(type) {
        case []interface{}:
            return false
    }
    return true
}

func Process(input string)  []interface{} {
    tok := TokenizeString(input)
    name := caar(tok)
    if name == "quote" {
        tok[0] = cadr(tok)
    } else if name == "car" {
        tok[0] = car(car(cdr(tok[0])))
    } else if name == "cdr" {
        tok[0] = cdr(car(cdr(tok[0])))
    } else if name == "atom" {
        tok[0] = atom(car(cdr(tok[0])))
    }
    return tok
}

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
    AssertThat(t, Process("(quote a b c)"), HasExactly(HasExactly("a", "b", "c")))
}

func TestCarGrabsFirstItem(t *testing.T) {
    AssertThat(t, Process("(car (a b))"), HasExactly("a"))
}

func TestCdrGrabsTail(t *testing.T) {
    AssertThat(t, Process("(cdr (a b c (d))"), HasExactly(HasExactly("b", "c", HasExactly("d"))))
}

func TestAtomIsTrueForSymbols(t *testing.T) {
    AssertThat(t, Process("(atom a)"), HasExactly(IsTrue))
}

func TestAtomIsFalseForComplexExpres(t *testing.T) {
    AssertThat(t, Process("(atom ())"), HasExactly(IsFalse))
}
