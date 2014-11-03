package glisp

import (
    "testing"
    //. "github.com/tychofreeman/go-matchers"
    . "matchers"
    "fmt"
    "strconv"
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

func Process2(tok interface{}) []interface{} {
    name := car(tok)
    var result interface{}
    var rest = car(cdr(tok))
    if name == "quote" {
        result = cdr(tok)
    } else if name == "car" {
        result = car(rest)
    } else if name == "cdr" {
        result = cdr(rest)
    } else if name == "atom" {
        result = atom(rest)
    } else {
        result = tok
    }
    return []interface{}{result}
}

func Parse(input []interface{}) []interface{} {
    output := []interface{}{}
    for i := 0; i < len(input); i++ { 
        switch x := input[i].(type) {
        case string:
            if num, err := strconv.ParseInt(x, 10, 64); err == nil {
                output = append(output, num)
            } else {
                output = append(output, x)
            }
        case []interface{}:
            output = append(output, Parse(x))
        default:
            output = append(output, nil)
            fmt.Printf("Could not find type for %v: %T\n", input[i], input[i])
        }
    }
    return output
}

func Execute (input []interface{}) interface{} {
    var output interface{} = input
    return output
}

func Process(input string)  interface{} {
    return Execute(Process2(Parse(TokenizeString(input))[0]))
}

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
    AssertThat(t, Process("(quote (\"a\" \"b\" \"c\"))"), HasExactly(HasExactly(HasExactly("\"a\"", "\"b\"", "\"c\""))))
}

func TestCarGrabsFirstItem(t *testing.T) {
    AssertThat(t, Process("(car (\"a\" \"b\"))"), HasExactly("\"a\""))
}

func TestCdrGrabsTail(t *testing.T) {
    AssertThat(t, Process("(cdr (\"a\" \"b\" \"c\" (\"d\"))"), HasExactly(HasExactly("\"b\"", "\"c\"", HasExactly("\"d\""))))
}

func TestAtomIsTrueForSymbols(t *testing.T) {
    AssertThat(t, Process("(atom \"a\")"), HasExactly(IsTrue))
}

func TestAtomIsFalseForComplexExpres(t *testing.T) {
    AssertThat(t, Process("(atom ())"), HasExactly(IsFalse))
}

func IGNORE_TestCorrectlyHandlesNestedCalls(t *testing.T) {
    AssertThat(t, Process("(car (cdr (\"a\" \"b\" \"c\")))"), HasExactly("\"b\""))
}

func TestIntegerLiteralsAreImplemented(t *testing.T) {
    AssertThat(t, Process("(car (1))"), HasExactly(Equals(int64(1))))
}
