package glisp

import (
    "fmt"
    "strings"
    "strconv"
    "os"
)


type Valuable interface {
    Eval(*Scope) interface{}
}

type ParamsList List
type Function func(_ *Scope, params List) interface{}
type NonEvaluatingFunction func(_ *Scope, params List) interface{}


// TODO We want this to live on a different type than List. But first, we must tokenize->parse->pass1->pass2->...->eval
func (things List) GetValues(scope *Scope) List {
    output := List{}
    for _, i := range things {
        output = append(output, GetValue(scope, i))
    }
    return output
}


func GetValue(scope *Scope, source interface{}) interface{} {
    switch value := source.(type) {
    case StringToken:
        return value.Str()
    case int64:
        return value
    case string:
        return value
    case List:
        return value.Eval(scope)
    case Valuable:
        return value.Eval(scope)
    default:
        panic(fmt.Sprintf("Couldn't find anything of type %T (%v)\n", value, value))
    }
    return nil
}


func quote(_ *Scope, params List) interface{} {
    if len(params) > 0 {
        return params[0]
    }
    panic(fmt.Sprintf("QUOTE takes exactly 1 argument; you have %v - %v\n", len(params), params))
}

func car(_ *Scope, params List) interface{} {
    if len(params) > 0 {
        switch x := params[0].(type) {
        case List:
            if len(x) > 0 {
                return x[0]
            }
        }
    }
    return nil
}

func cdr(_ *Scope, params List) interface{} {
    if len(params) > 0 {
        switch x := params[0].(type) {
        case List:
            if len(x) > 0 {
                return x.Rest()
            }
        }
    }
    return nil
}

func atom(_ *Scope, params List) interface{} {
    if len(params) > 0 {
        switch params[0].(type) {
        case List:
            return false
        default:
            return true
        }
    } else {
        return false
    }
}

func cons(_ *Scope, params List) interface{} {
    if len(params) == 1 {
        return params
    } else if len(params) == 2 {
        if atom(nil, params.Rest()) == true {
            return params
        } else {
            switch x := params[1].(type) {
            case List:
                output := List{params[0]}
                for _, i := range x {
                    output = append(output, i)
                }
                return output
            }
        }
    }
    return nil
}

func plus(_ *Scope, params List) interface{} {
    var sum int64 = 0
    for _, p := range params {
        switch x := p.(type) {
        case int64:
            sum = sum + x
        }
    }
    return sum
}

func if_(scope *Scope, params List) interface{} {
    if len(params) != 3 {
        panic(fmt.Sprintf("IF requires 3 parts - conditional, true expression and false expression. You have %v parts - %v.", len(params), params))
    }
    if true == GetValue(scope, params[0]) {
        return GetValue(scope, params[1])
    }
    return GetValue(scope, params[2])
}

func eq(_ *Scope, params List) interface{} {
    if len(params) != 2 {
        panic(fmt.Sprintf("EQ requires exactly 2 parameters; you have %v - %v", len(params), params))
    }
    return params[0] == params[1]
}

func apply(scope *Scope, params List) interface{} {
    return GetValue(scope, params)
}

func define_(scope *Scope, params List) interface{} {
    name := params.First().(Token).Str()
    body := params.Rest().First()

    scope.add(name, body)
    return List{}
}

func macro(scope *Scope, params List) interface{} {
    name := params.First().(Token).Str()
    body := params.Rest().First()
    macroFn := NonEvaluatingFunction(func(macroScope *Scope, macroParams List) interface{} {
        switch b := body.(type) {
        case Function:
        return b(macroScope, macroParams)
        case NonEvaluatingFunction:
        return b(macroScope, macroParams)
        default:
        panic(fmt.Sprintf("Could nt execute a function: %t %v\n%v\n", b, b, macroScope))
        return nil
        }
    })
    scope.add(name, macroFn)
    return List{}
}

func print(scope *Scope, params List) interface{} {
    fmt.Printf("%v\n", params)
    os.Stdout.Sync()
    return List{}
}

var builtins = map[string]interface{} {
    "quote": NonEvaluatingFunction(quote),
    "car"  : Function(car),
    "cdr"  : Function(cdr),
    "atom" : Function(atom),
    "cons" : Function(cons),
    "plus" : Function(plus),
    "if"   : NonEvaluatingFunction(if_),
    "eq"   : Function(eq),
    "apply": Function(apply),
    "def"  : NonEvaluatingFunction(define_),
    "defmacro" : NonEvaluatingFunction(macro),
    "p"    : Function(print),
}

func make_param_binding_fn(param_decls interface{}) (func(interface{}) map[string]interface{}) {
    param_names := []string{}
    switch x := param_decls.(type) {
    case List:
        for _, y := range x {
            switch z := y.(type) {
            case Token:
                param_names = append(param_names, z.Str())
            default:
                param_names = append(param_names, "")
            }
        }
    default:
        panic(fmt.Sprintf("Cannot build param bindings with unknown declaration type %T (expected a List of symbols)\n", x))
    }
    return func(theParams interface{}) map[string]interface{} {
        scope := map[string]interface{}{}
        switch params := theParams.(type) {
        case List:
            for i := 0; i < len(param_names); i++ {
                scope[param_names[i]] = params[i]
            }
        default:
            panic(fmt.Sprintf("Cannot associate param names with values if values aren't in a List - found type %T\n", params))
        }
        return scope
    }
}

func Parse(source interface{}) interface{} {
    switch node := source.(type) {
    case Token:
        if strings.HasPrefix(node.Str(), "\"") {
            return node.Str()[1:len(node.Str())-1]
        } else if num, err := strconv.ParseInt(strings.TrimSpace(node.Str()), 10, 64); err == nil {
            return num
        } else {
            return node
        }
    case List:
        if node.IsLambda() {
            body := ParseMany(node.Rest().Rest())
            param_binding_fn := make_param_binding_fn(node.Second())
            return Function(func(scope *Scope, params List) interface{} {
                param_bindings := param_binding_fn(params)
                var lastElement interface{} = nil
                for _, element := range body {
                    lastElement = GetValue(&Scope{scope, param_bindings, false}, element)
                }
                return lastElement
            })
        }
        x :=  ParseMany(node)
        return x
    }
    return source
}

func ParseMany(input List) List {
    output := List{}
    for _, i := range input {
        output = append(output, Parse(i))
    }

    return List(output)
}

var baseScope = &Scope{nil, builtins, false}

func ProcessTokens(scope *Scope, tokenized List, includeStdLib bool) interface{} {
    if includeStdLib {
        ProcessTokens(scope, TokenizeFile("/Users/cwfreeman/dev/go/src/glisp/stdlib.glisp"), false)
    }
    parsed := ParseMany(tokenized)
    value := GetValue(scope, parsed)
    return value
}

func Process(input string)  interface{} {
    return ProcessTokens(baseScope, TokenizeString(input), true)
}

func ProcessFile(fname string) interface{} {
    return ProcessTokens(baseScope, TokenizeFile(fname), true)
}
