package glisp

import (
    "testing"
    . "github.com/tychofreeman/go-matchers"
    "fmt"
    "strings"
    "strconv"
//    "reflect"
)

type Scope struct {}

func rest(all []interface{}) []interface{} {
    if len(all) > 0 {
        return all[1:]
    }
    return nil
}


type Function func(_ *Scope, params []interface{}) interface{}

//type Lambda func(s *Scope, params []interface{}) (*Scope, Function)
type Lambda struct{}

func GetValues(things []interface{}) []interface{} {
    output := []interface{}{}
    for _, i := range things {
        output = append(output, GetValue(i))
    }
    return output
}

func last(input []interface{}) interface{} {
    if len(input) > 0 {
        return input[len(input)-1]
    }
    return nil
}

func GetValue(thing interface{}) interface{} {
    switch x := thing.(type) {
    case string:
        if strings.HasPrefix(x, "\"") {
            return x[1:len(x)-2]
        } else if num, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
            return num
        }
        return x
    case []interface{}:
        switch y := x[0].(type) {
        case Function:
            fmt.Printf("Got function %v\n", x)
            params := GetValues(rest(x))
            z := y(nil, params)
            fmt.Printf("Calling function %v(%v) -> %v\n", y, params, z)
            return z
        case []interface{}:
            z := GetValue(y)
            fmt.Printf("...So, I called it recursively... %T %v\n", z, z)
            return z
        default:
            fmt.Printf("Oops!!\n")
        }
    default:
        fmt.Printf("Couldn't find thing of type %T (%v)\n", x, x)
    }
    return nil
}

func quote(_ *Scope, params []interface{}) interface{} {
    return params
}

func car(_ *Scope, params []interface{}) interface{} {
    if len(params) > 0 {
        switch x := params[0].(type) {
        case []interface{}:
            if len(x) > 0 {
                return x[0]
            }
        }
    }
    return nil
}

func cdr(_ *Scope, params []interface{}) interface{} {
    if len(params) > 0 {
        switch x := params[0].(type) {
        case []interface{}:
            if len(x) > 0 {
                return rest(x)
            }
        }
    }
    return nil
}

func atom(_ *Scope, params []interface{}) interface{} {
    if len(params) > 0 {
        switch params[0].(type) {
        case []interface{}:
            return false
        default:
            return true
        }
    } else {
        return false
    }
}

func cons(_ *Scope, params []interface{}) interface{} {
    if len(params) == 1 {
        return params
    } else if len(params) == 2 {
        if atom(nil, rest(params)) == true {
            return params
        } else {
            switch x := params[1].(type) {
            case []interface{}:
                output := []interface{}{params[0]}
                for _, i := range x {
                    output = append(output, i)
                }
                return output
            }
        }
    }
    return nil
}

var lookup = map[string]Function {
    "quote": quote,
    "car"  : car,
    "cdr"  : cdr,
    "atom" : atom,
    "cons" : cons,
}

func Parse(thing interface{}) interface{} {
    switch x := thing.(type) {
    case string:
        if fn, ok := lookup[x]; ok {
            return fn
        }
    case []interface{}:
        if len(x) > 1 && x[0] == "lambda" {
            body := ParseMany(rest(rest(x)))
            return Function(func(_ *Scope, params[]interface{}) interface{} {
                l := last(body)
                fmt.Printf("Lambda has expression: %v\n", l)
                v := GetValue(l)
                fmt.Printf("                          -> %v\n", v)
                return v
            })
        }
        return ParseMany(x)
    }
    return thing
}

func ParseMany(input []interface{}) []interface{} {
    output := []interface{}{}
    for _, i := range input {
        output = append(output, Parse(i))
    }
    return output
}

func Process(input string)  interface{} {
    tokenized := TokenizeString(input)
    fmt.Printf("Tokenized: %v\n", tokenized)
    parsed := ParseMany(tokenized)
    fmt.Printf("Parsed: %v\n", parsed)
    value := GetValue(parsed)
    fmt.Printf("Value: %v\n", value)
    return value
}

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
    x := Process("(quote \"a\" \"b\" \"c\")")
    fmt.Printf("X: %v\n", x)
    AssertThat(t, x, HasExactly("\"a\"", "\"b\"", "\"c\""))
}

func TestCarGrabsFirstItem(t *testing.T) {
    AssertThat(t, Process("(car (quote \"a\" \"b\"))"), Equals("\"a\""))
}

func TestCdrGrabsTail(t *testing.T) {
    AssertThat(t, Process("(cdr (quote \"a\" \"b\" \"c\" (quote \"d\"))"), HasExactly("\"b\"", "\"c\"", HasExactly("\"d\"")))
}

func TestAtomIsTrueForSymbols(t *testing.T) {
    AssertThat(t, Process("(atom \"a\")"), IsTrue)
}

func TestAtomIsFalseForComplexExpres(t *testing.T) {
    AssertThat(t, Process("(atom (quote)"), IsFalse)
}

func TestIntegerLiteralsAreImplemented(t *testing.T) {
    AssertThat(t, Process("(car (quote 1))"), Equals(int64(1)))
}

func TestCorrectlyHandlesNestedCalls(t *testing.T) {
    AssertThat(t, Process("(car (cdr (quote \"a\" \"b\" \"c\")))"), Equals("\"b\""))
}

func TestConsCreatesLists(t *testing.T) {
    AssertThat(t, Process("(cons \"a\" (quote \"b\"))"), HasExactly("\"a\"", "\"b\""))
}

func TestOnePlusOneEqualsTwo(t *testing.T) {
    AssertThat(t, Process("(plus 1 1)"), Equals(int64(2)))
}

func TestConditional(t *testing.T) {
    AssertThat(t, Process("(if (atom (quote)) 1 2)"), Equals(int64(2)))
}

func TestOneEqualsOne(t *testing.T) {
    AssertThat(t, Process("(eq 1 1)"), IsTrue)
}

func TestOneNotEqualTwo(t *testing.T) {
    AssertThat(t, Process("(eq 1 2)"), IsFalse)
}

func TestSupportsLambdas(t *testing.T) {
    x := Process("((lambda (quote) 6))")
    fmt.Printf("TestSupportsLambdas: %v\n", x)
    AssertThat(t, x, Equals(int64(6)))
}

func TestSupportsExpressionsInLambdas(t *testing.T) {
    AssertThat(t, Process("((lambda (quote) (quote 1 2 3)))"), HasExactly(Equals(int64(1)), Equals(int64(2)), Equals(int64(3))))
}

func TestSupportsLambdaParameters(t *testing.T) {
    AssertThat(t, Process("((lambda (quote a) a) 1)"), HasExactly(Equals(int64(1))))
}

func TestLambdasAreClosures(t *testing.T) {
    AssertThat(t, Process("((lambda (quote a) (lambda (quote ) a)) 1)"), HasExactly(Equals(int64(1))))
}

func NOT_YET_DO_IT_WITH_MACROS_TestSupportsLetBindings(t *testing.T) {
    AssertThat(t, Process("(let (quote a 1) a)"), Equals(int64(1)))
}
