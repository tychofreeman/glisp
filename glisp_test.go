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

func quote(sexp []interface{}) interface{} {
    return sexp[0]
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

func Execute (input []interface{}) interface{} {
    var output interface{} = input
    if len(input) > 0 {
        switch x := input[0].(type) {
        case reflect.Value:
            rtn := x.Call([]reflect.Value{reflect.ValueOf(cdr(input))})[0].Interface()
            return rtn
        }
    }
    return output
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

func IGNORE_TestCorrectlyHandlesNestedCalls(t *testing.T) {
    AssertThat(t, Process("(car (cdr (\"a\" \"b\" \"c\")))"), HasExactly("\"b\""))
}

func TestIntegerLiteralsAreImplemented(t *testing.T) {
    AssertThat(t, Process("(car (1))"), Equals(int64(1)))
}
