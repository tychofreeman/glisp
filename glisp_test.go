package glisp

import (
    "testing"
    . "github.com/tychofreeman/go-matchers"
    "fmt"
    "strings"
    "strconv"
//    "reflect"
)

type Scope struct {
    prev *Scope
    table map[string]interface{}
}

func (scope *Scope) lookup(name string) (interface{}, bool) {
    if val, ok := scope.table[name]; ok {
        return val, ok
    }
    if scope.prev != nil {
        return scope.prev.lookup(name)
    }
    return nil, false
}


func rest(all []interface{}) []interface{} {
    if len(all) > 0 {
        return all[1:]
    }
    return nil
}


type Function func(_ *Scope, params []interface{}) interface{}

func GetValues(scope *Scope, things []interface{}) []interface{} {
    output := []interface{}{}
    for _, i := range things {
        output = append(output, GetValue(scope, i))
    }
    return output
}

func last(input []interface{}) interface{} {
    if len(input) > 0 {
        return input[len(input)-1]
    }
    return nil
}

func GetValue(scope *Scope, thing interface{}) interface{} {
    switch x := thing.(type) {
    case string:
        if strings.HasPrefix(x, "\"") {
            return x[1:len(x)-2]
        } else if num, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
            return num
        }
        if y, ok := scope.lookup(x); ok {
            return y
        }
        panic(fmt.Sprintf("Invalid value %v - not a string or number, and could not be found in look-up table %v.", x, scope.table))
    case []interface{}:
        switch y := x[0].(type) {
        case Function:
            params := GetValues(scope, rest(x))
            z := y(scope, params)
            return z
        case []interface{}:
            z := GetValue(scope, y)
            return z
        default:
            panic("A list should be either a function or a nested list (probably actually a high-order function)")
        }
    default:
        panic(fmt.Sprintf("Couldn't find thing of type %T (%v)\n", x, x))
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


func param_binding_form_to_scope_and_order(param_decls interface{}) (func([]interface{}) map[string]interface{}) {
    param_names := []string{}
    switch x := param_decls.(type) {
    case []interface{}:
        for _, y := range x {
            switch z := y.(type) {
            case string:
                param_names = append(param_names, z)
            default:
                param_names = append(param_names, "")
            }
        }
    }
    return func(params []interface{}) map[string]interface{} {

        scope := map[string]interface{}{}
        for i := 0; i < len(param_names); i++ {
            scope[param_names[i]] = params[i]
        }
        return scope
    }
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
            param_binding_fn := param_binding_form_to_scope_and_order(rest(x)[0])
            return Function(func(scope *Scope, params[]interface{}) interface{} {
                param_bindings := param_binding_fn(params)
                scope2 := &Scope{scope, param_bindings}
                l := last(body)
                v := GetValue(scope2, l)
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
    //fmt.Printf("Tokenized: %v\n", tokenized)
    parsed := ParseMany(tokenized)
    //fmt.Printf("Parsed: %v\n", parsed)
    value := GetValue(&Scope{nil, map[string]interface{}{}}, parsed)
    //fmt.Printf("Value: %v\n", value)
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
    x := Process("((lambda () 6))")
    fmt.Printf("TestSupportsLambdas: %v\n", x)
    AssertThat(t, x, Equals(int64(6)))
}

func TestSupportsExpressionsInLambdas(t *testing.T) {
    AssertThat(t, Process("((lambda () (quote 1 2 3)))"), HasExactly(Equals(int64(1)), Equals(int64(2)), Equals(int64(3))))
}

func TestSupportsLambdaParameters(t *testing.T) {
    AssertThat(t, Process("((lambda (a) a) 1)"), Equals(int64(1)))
}

func TestLambdasAreClosures(t *testing.T) {
    AssertThat(t, Process("((lambda (a) ((lambda () a))) 1)"), Equals(int64(1)))
}

func NOT_YET_DO_IT_WITH_MACROS_TestSupportsLetBindings(t *testing.T) {
    AssertThat(t, Process("(let (quote a 1) a)"), Equals(int64(1)))
}
