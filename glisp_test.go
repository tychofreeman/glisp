package glisp

import (
    "testing"
    . "github.com/tychofreeman/go-matchers"
    "fmt"
    "strconv"
    "reflect"
)

type Scope struct {}

type Callable struct {
    Call func(scope *Scope, in []interface{}) interface{}
    Transform func(scope *Scope, in []interface{}) (*Scope, []interface{})
    y string
};


func car(scope *Scope, in []interface{}) interface{} {
    if in == nil || len(in) < 1 {
        return nil
    }
    switch t := in[0].(type) {
        case []interface{}:
            if len(t) > 0 {
                return t[0];
            }
    }
    return nil
}

func _cdr(in []interface{}) []interface{} {
    if in == nil || len(in) < 1 {
        fmt.Printf("Len: %v\n", len(in))
        return []interface{}{}
    }
    return in[1:]
}

func cdr(scope *Scope, in []interface{}) interface{} {
    if in == nil || len(in) < 1 {
        return []interface{}{}
    }
    switch x := in[0].(type) {
        case []interface{}:
            return _cdr(x)
    }
    return []interface{}{}
}

func cons(scope *Scope, in []interface{}) interface{} {
    out := []interface{}{in[0]}
    switch x := in[1].(type) {
    case []interface{}:
        out = append(out, x...)
    }
    return out
}

func atom(scope *Scope, in []interface{}) interface{} {
    if len(in) < 1 {
        return false
    }
    switch in[0].(type) {
        case []interface{}:
            return false
    }
    return true
}

func quote(scope *Scope, sexp []interface{}) interface{} {
    return sexp
}

func if_(scope *Scope, in []interface{}) interface{} {
    if in[0] == false || in[0] == nil {
        return in[2]
    } else {
        return in[1]
    }
}

func plusInt64(in []int64) int64 {
    sum := int64(0)
    for _, i := range in {
        sum = sum + i
    }
    return sum
}

func plus(scope *Scope, in []interface{}) interface{} {
    ints := []int64{}
    for _, i := range in {
        switch x := i.(type) {
        case int64:
            ints = append(ints, x)
        }
    }
    return plusInt64(ints)
}

func eq(scope *Scope, in []interface{}) interface{} {
    return reflect.DeepEqual(in[0], in[1])
}

func Execute (scope *Scope, input []interface{}) interface{} {
    var output interface{} = input
    //fmt.Printf("Execute %v\n", input)
    for x := 0; x < len(input); x++ {
        switch y := input[x].(type) {
        case []interface{}:
            //fmt.Printf("Recursing on %v\n", y)
            input[x] = Execute(scope, y)
        }
    }
    if len(input) > 0 {
        switch x := input[0].(type) {
        case Callable:
            //fmt.Printf("Calling %v function with %v\n", x.y, _cdr(input))
            rtn := x.Call(scope, _cdr(input))
            //fmt.Printf("\tReturned %v\n", rtn)
            return rtn
        default:
            //fmt.Printf("Failed to execute: %v\n", input)
            panic(fmt.Sprintf("Got the wrong type: %T : %v\n", x, x))
        }
    }
    return output
}

func Reparse(bindings map[string]interface{}, input []interface{}) []interface{} {
    output := []interface{}{}
    for _, i := range input {
        switch x := i.(type) {
        case string:
            if val, ok := bindings[x]; ok {
                output = append(output, val)
            } else {
                output = append(output, x)
            }
        default:
            output = append(output, i)
        }
    }
    return output
}

func copyToStrings(_input interface{}) []string {
    output := []string{}
    switch input := _input.(type) {
    case []interface{}:
        for _, i := range input {
            switch z := i.(type) {
                case string:
                    output = append(output, z)
                default:
                    output = append(output, "")
            }
        }
    default:
        panic("Well, fuck")
    }
    return output
}

func lambda(scope *Scope, input []interface{}) interface{} {
    paramBindings := copyToStrings(input[0])
    //fmt.Printf("ParamBindings: %v -> %v\n", input, paramBindings)

    body := input[1]
    //fmt.Printf("Got Body: %v\n", body)
    return Callable{func(scope *Scope, paramValues []interface{}) interface{} {
        bindings := map[string]interface{}{}
        for i := range paramBindings {
            bindings[paramBindings[i]] = paramValues[i]
        }
        switch body2 := body.(type) {
        case []interface{}:
            return Reparse(bindings, body2)
        default:
            return Reparse(bindings, _cdr(input))
        }
    }, nil, "-lambda-"}
}

func Parse(input [] interface{}) []interface{} {
    output := []interface{}{}
    for i := 0; i < len(input); i++ { 
        switch x := input[i].(type) {
        case string:
            if num, err := strconv.ParseInt(x, 10, 64); err == nil {
                output = append(output, num)
            } else {
                if x == "quote" {
                    output = append(output, Callable{quote, nil, "quote"})
                } else if x == "car" {
                    output = append(output, Callable{car, nil, "car"})
                } else if x == "cdr" {
                    output = append(output, Callable{cdr, nil, "cdr"})
                } else if x == "atom" {
                    output = append(output, Callable{atom, nil, "atom"})
                } else if x == "cons" {
                    output = append(output, Callable{cons, nil, "cons"})
                } else if x == "plus" {
                    output = append(output, Callable{plus, nil, "plus"})
                } else if x == "if" {
                    output = append(output, Callable{if_, nil, "if"})
                } else if x == "eq" {
                    output = append(output, Callable{eq, nil, "eq"})
                } else if x == "lambda" {
                    output = append(output, Callable{lambda, nil, "lambda"})
                } else {
                    output = append(output, x)
                }
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

func ParseWrapper(input interface{}) []interface{} {
    switch x := input.(type) {
    case []interface{}:
        return Parse(x)
    default:
        return []interface{}{}
    }
}

func Process(input string)  interface{} {
    scope := Scope{}
    return Execute(&scope, ParseWrapper(TokenizeString(input)[0]))
}

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
    AssertThat(t, Process("(quote \"a\" \"b\" \"c\")"), HasExactly("\"a\"", "\"b\"", "\"c\""))
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
    AssertThat(t, Process("((lambda (quote) 6))"), HasExactly(Equals(int64(6))))
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
