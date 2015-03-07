package glisp

import (
    "testing"
    . "github.com/tychofreeman/go-matchers"
    "fmt"
    "strconv"
    "reflect"
)

type Scope struct {}
type Calling func(scope *Scope, in []interface{}) interface{}

type Callable struct {
    Call Calling
    Transform func(scope *Scope, in []interface{}) (*Scope, []interface{})
    name string
};


func car(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
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
}

func _cdr(in []interface{}) []interface{} {
    if in == nil || len(in) < 1 {
        fmt.Printf("Len: %v\n", len(in))
        return []interface{}{}
    }
    return in[1:]
}

func cdr(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        if in == nil || len(in) < 1 {
            return []interface{}{}
        }
        switch x := in[0].(type) {
            case []interface{}:
                return _cdr(x)
        }
        return []interface{}{}
    }
}

func cons(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        out := []interface{}{in[0]}
        switch x := in[1].(type) {
        case []interface{}:
            out = append(out, x...)
        }
        return out
    }
}

func atom(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        if len(in) < 1 {
            return false
        }
        switch in[0].(type) {
            case []interface{}:
                return false
        }
        return true
    }
}

func quote(sexp []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        fmt.Printf("Returning %v\n", in)
        return in
    }
}

func if_(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        if in[0] == false || in[0] == nil {
            return in[2]
        } else {
            return in[1]
        }
    }
}

func plusInt64(in []int64) int64 {
    sum := int64(0)
    for _, i := range in {
        sum = sum + i
    }
    return sum
}

func plus(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        ints := []int64{}
        for _, i := range in {
            switch x := i.(type) {
            case int64:
                ints = append(ints, x)
            }
        }
        return plusInt64(ints)
    }
}


func eq(in []interface{}) Calling {
    return func(scope *Scope, in []interface{}) interface{} {
        return reflect.DeepEqual(in[0], in[1])
    }
}

func Execute (scope *Scope, input []Callable) interface{} {
    output := []interface{}{}
    
    fmt.Printf("Execute %v\n", input)
    for _, i := range input {
        output = append(output, i.Call(scope, []interface{}{}))
    }
    if len(output) > 0 {
        switch c := output[0].(type) {
        case Callable:
            return c.Call(scope, _cdr(output))
        default:
            return nil
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

/*
func lambda(input []interface{}) Calling {
    paramBindings := copyToStrings(input[0])
    //fmt.Printf("ParamBindings: %v -> %v\n", input, paramBindings)

    body := interface{}(_cdr(input))

    if body == nil {
        return func(scope *Scope, paramValues []interface{}) interface{} { return []interface{}{} }
    } else {
        return func(scope *Scope, paramValues []interface{}) interface{} {
            zz := func(bindings map[string]interface{}, i interface{}) interface{} {
                var last interface{}
                switch j:= i.(type) {
                case []Callable:
                    last = Execute(scope, j)
                case Callable:
                    last = j.Call([]interface{}{})
                }
                return last
            }
            bindings := map[string]interface{}{}
            for i := range paramBindings {
                bindings[paramBindings[i]] = paramValues[i]
            }
            switch body2 := body.(type) {
            case []Callable:
                var last interface{}
                for _, i := range body2 {
                    last = zz(bindings, i)
                }
                return last
            case Callable:
                return Excute(bindings, []body2)
            default:
                return zz(bindings, body2)
            }
        }
    }
}*/

var nilFunc Callable = Callable{func(s *Scope, in []interface{}) interface{} {return nil}, nil, "unknown"}

func literal(in []interface{}) Calling {
    return func(s *Scope, in []interface{}) interface{} {
        return in[0]
    }
}

func Parse(input [] interface{}) []Callable {
    output := []Callable{}
    for i := 0; i < len(input); i++ { 
        switch x := input[i].(type) {
        case string:
            if num, err := strconv.ParseInt(x, 10, 64); err == nil {
                output = append(output, Callable{literal([]interface{}{num}), nil, "literal"})
            } else {
                if x == "quote" {
                    output = append(output, Callable{quote(input), nil, "quote"})
                } else if x == "car" {
                    output = append(output, Callable{car(input), nil, "car"})
                } else if x == "cdr" {
                    output = append(output, Callable{cdr(input), nil, "cdr"})
                } else if x == "atom" {
                    output = append(output, Callable{atom(input), nil, "atom"})
                } else if x == "cons" {
                    output = append(output, Callable{cons(input), nil, "cons"})
                } else if x == "plus" {
                    output = append(output, Callable{plus(input), nil, "plus"})
                } else if x == "if" {
                    output = append(output, Callable{if_(input), nil, "if"})
                } else if x == "eq" {
                    output = append(output, Callable{eq(input), nil, "eq"})
                } else if x == "lambda" {
                    //output = append(output, Callable{lambda(input), nil, "lambda"})
                    output = append(output, nilFunc)
                } else {
                    output = append(output, Callable{literal([]interface{}{x}), nil, "literal-unknown"})
                }
            }
        case []interface{}:
            output = append(output, Callable{
                func(s *Scope, in []interface{}) interface{} {
                    z:= Parse(x);
                    params := []interface{}{}
                    for i := 1; i < len(z); i++ {
                        params = append(params, z[i].Call(s, []interface{}{}))
                    }
                    return z[0].Call(s, params)
                }, nil, "Parse"})
        default:
            output = append(output, nilFunc)
            fmt.Printf("Could not find type for %v: %T\n", input[i], input[i])
        }
    }
    return output
}

func ParseWrapper(input interface{}) []Callable {
    switch x := input.(type) {
    case []interface{}:
        return Parse(x)
    default:
        return []Callable{nilFunc}
    }
}

func Process(input string)  interface{} {
    scope := Scope{}
    return Execute(&scope, ParseWrapper(TokenizeString(input)[0]))
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
