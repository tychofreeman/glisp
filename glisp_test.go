package glisp

import (
    "testing"
    . "github.com/tychofreeman/go-matchers"
    "fmt"
    "strconv"
    "reflect"
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

func cdr(in interface{}) []interface{} {
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

func cons(in []interface{}) interface{} {
    out := []interface{}{in[0]}
    switch x := in[1].(type) {
    case []interface{}:
        out = append(out, x...)
    }
    return out
}

func cadr(in []interface{}) interface{} {
    return cdr(car(in))
}

func atom(in interface{}) bool {
    switch x := in.(type) {
        case []interface{}:
            switch x[0].(type) {
                case []interface{}:
                    return false
            }
    }
    return true
}

func quote(sexp []interface{}) interface{} {
    return sexp[0]
}

func if_(in []interface{}) interface{} {
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

func plus(in []interface{}) interface{} {
    ints := []int64{}
    for _, i := range in {
        switch x := i.(type) {
        case int64:
            ints = append(ints, x)
        }
    }
    return plusInt64(ints)
}

func eq(in []interface{}) interface{} {
    return reflect.DeepEqual(in[0], in[1])
}

func Execute (input []interface{}) interface{} {
    var output interface{} = input
    for x := 0; x < len(input); x++ {
        switch y := input[x].(type) {
        case []interface{}:
            input[x] = Execute(y)
        }
    }
    if len(input) > 0 {
        switch x := input[0].(type) {
        case reflect.Value:
            rtn := x.Call([]reflect.Value{reflect.ValueOf(cdr(input))})[0].Interface()
            return rtn
        }
    }
    return output
}

func lambda(input []interface{}) interface{} {
    //paramBindings := car(input)
    body := cdr(input)
    return reflect.ValueOf(func(paramValues []interface{}) interface{} {
        return Execute(body)   
    })
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
                    output = append(output, reflect.ValueOf(quote))
                } else if x == "car" {
                    output = append(output, reflect.ValueOf(caar))
                } else if x == "cdr" {
                    output = append(output, reflect.ValueOf(cadr))
                } else if x == "atom" {
                    output = append(output, reflect.ValueOf(atom))
                } else if x == "cons" {
                    output = append(output, reflect.ValueOf(cons))
                } else if x == "plus" {
                    output = append(output, reflect.ValueOf(plus))
                } else if x == "if" {
                    output = append(output, reflect.ValueOf(if_))
                } else if x == "eq" {
                    output = append(output, reflect.ValueOf(eq))
                } else if x == "lambda" {
                    output = append(output, reflect.ValueOf(lambda))
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
    return Execute(ParseWrapper(TokenizeString(input)[0]))
}

func TestQuoteSpitsOutRemainderOfExpression(t *testing.T) {
    AssertThat(t, Process("(quote (\"a\" \"b\" \"c\"))"), HasExactly("\"a\"", "\"b\"", "\"c\""))
}

func TestCarGrabsFirstItem(t *testing.T) {
    AssertThat(t, Process("(car (\"a\" \"b\"))"), Equals("\"a\""))
}

func TestCdrGrabsTail(t *testing.T) {
    AssertThat(t, Process("(cdr (\"a\" \"b\" \"c\" (\"d\"))"), HasExactly("\"b\"", "\"c\"", HasExactly("\"d\"")))
}

func TestAtomIsTrueForSymbols(t *testing.T) {
    AssertThat(t, Process("(atom \"a\")"), IsTrue)
}

func TestAtomIsFalseForComplexExpres(t *testing.T) {
    AssertThat(t, Process("(atom ())"), IsFalse)
}

func TestIntegerLiteralsAreImplemented(t *testing.T) {
    AssertThat(t, Process("(car (1))"), Equals(int64(1)))
}

func TestCorrectlyHandlesNestedCalls(t *testing.T) {
    AssertThat(t, Process("(car (cdr (\"a\" \"b\" \"c\")))"), Equals("\"b\""))
}

func TestConsCreatesLists(t *testing.T) {
    AssertThat(t, Process("(cons \"a\" (quote (\"b\")))"), HasExactly("\"a\"", "\"b\""))
}

func TestOnePlusOneEqualsTwo(t *testing.T) {
    AssertThat(t, Process("(plus 1 1)"), Equals(int64(2)))
}

func TestConditional(t *testing.T) {
    AssertThat(t, Process("(if (atom ()) 1 2)"), Equals(int64(2)))
}

func TestOneEqualsOne(t *testing.T) {
    AssertThat(t, Process("(eq 1 1)"), IsTrue)
}

func TestOneNotEqualTwo(t *testing.T) {
    AssertThat(t, Process("(eq 1 2)"), IsFalse)
}

func TestSupportsLambdas(t *testing.T) {
    AssertThat(t, Process("((lambda () 6))"), HasExactly(Equals(int64(6))))
}
