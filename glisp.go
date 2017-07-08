package glisp

import (
    "fmt"
    "strings"
    "strconv"
    "os"
)

type Scope struct {
    prev *Scope
    table map[string]interface{}
    isMacroScope bool
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

func (scope *Scope) add(name string, defn interface{}) {
    scope.table[name] = defn
    return
}

func (scope *Scope) createNonEvaluatingScope() *Scope {
    return &Scope{scope, map[string]interface{}{}, true}
}


type Symbol struct { name string }

func (sym Symbol) Eval(scope *Scope) interface{} {
    if scope.isMacroScope {
        return sym
    } else if resolved, ok := scope.lookup(sym.name); ok {
        return resolved
    } else {
        panic(fmt.Sprintf("Cannot resolve symbol %v in lookup %v\n", sym.name, scope))
    }
}

type Valuable interface {
    Eval(*Scope) interface{}
}

type ParamsList List
type Function func(_ *Scope, params List) interface{}
type NonEvaluatingFunction func(_ *Scope, params List) interface{}

type List []interface{}

func (all List) first() interface{} {
    if all != nil && len(all) > 0 {
        return all[0]
    }
    return nil
}

func (all List) second() interface{} {
    return all.rest().first()
}

func (all List) rest() List {
    if all != nil && len(all) > 0 {
        return List(all[1:])
    }
    return nil
}
func (things List) GetValues(scope *Scope) List {
    output := List{}
    for _, i := range things {
        output = append(output, GetValue(scope, i))
    }
    return output
}

func (input List) last() interface{} {
    if len(input) > 0 {
        return input[len(input)-1]
    }
    return nil
}

func last(input []interface{}) interface{} {
    if len(input) > 0 {
        return input[len(input)-1]
    }
    return nil
}

func (value List) Eval(scope *Scope) interface{} {
    if scope.isMacroScope {
        return value
    }
    switch firstValue := value[0].(type) {
    case NonEvaluatingFunction:
        return firstValue(scope, value.rest())
    case Function:
        params := value.rest().GetValues(scope)
        return firstValue(scope, params)
    case Valuable:
        switch symb := firstValue.Eval(scope).(type) {
        case NonEvaluatingFunction:
            x := symb(scope, value.rest())
            return x
        case Function:
            params := value.rest().GetValues(scope)
            x := symb(scope, params)
            return x
        default:
            panic(fmt.Sprintf("A list should be either a function or a nested list (probably actually a high-order function) - found %T %v in %v\n", firstValue, firstValue, value))
        }
    case []interface{}:
        lastElement := interface{}(nil)
        for _, element := range value {
            lastElement = GetValue(scope, element)
        }
        return lastElement
    }
    panic(fmt.Sprintf("Could not evaluate list: %v\n", value))
}


func GetValue(scope *Scope, source interface{}) interface{} {

    switch value := source.(type) {
    case int64:
        return value
    case string:
        return value
    case Valuable:
        return value.Eval(scope)
    case []interface{}:
        return List(value).Eval(scope)
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
        case []interface{}:
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
                return x.rest()
            }
        case []interface{}:
            if len(x) > 0 {
                return List(x).rest()
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
        case []interface{}:
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
        if atom(nil, params.rest()) == true {
            return params
        } else {
            switch x := params[1].(type) {
            case List:
                output := List{params[0]}
                for _, i := range x {
                    output = append(output, i)
                }
                return output
            case []interface{}:
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
    name := params.first().(Symbol).name
    body := params.rest().first()

    scope.add(name, body)
    return List{}
}

func macro(scope *Scope, params List) interface{} {
    name := params.first().(Symbol).name
    body := params.rest().first()
    macroFn := NonEvaluatingFunction(func(macroScope *Scope, macroParams List) interface{} {
        switch b := body.(type) {
        case Function:
        return b(macroScope, macroParams)
        case NonEvaluatingFunction:
        return b(macroScope, macroParams)
        default:
        fmt.Printf("Could nt execute a function: %t %v\n%v\n", b, b, macroScope)
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
            case string:
                param_names = append(param_names, z)
            default:
                param_names = append(param_names, "")
            }
        }
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
    return func(theParams interface{}) map[string]interface{} {
        scope := map[string]interface{}{}
        switch params := theParams.(type) {
        case List:
            for i := 0; i < len(param_names); i++ {
                scope[param_names[i]] = params[i]
            }
        case []interface{}:
            for i := 0; i < len(param_names); i++ {
                scope[param_names[i]] = params[i]
            }
        }
        return scope
    }
}

func Parse(source interface{}) interface{} {
    switch node := source.(type) {
    case string:
        if strings.HasPrefix(node, "\"") {
            return node[1:len(node)-1]
        } else if num, err := strconv.ParseInt(strings.TrimSpace(node), 10, 64); err == nil {
            return num
        } else {
            return Symbol{node}
        }
    case []interface{}:
        if len(node) > 1 && node[0] == "lambda" {
            body := ParseMany(List(node).rest().rest())
            param_binding_fn := make_param_binding_fn(List(node).second())
            return Function(func(scope *Scope, params List) interface{} {
                param_bindings := param_binding_fn(params)
                var lastElement interface{} = nil
                for _, element := range body {
                    lastElement = GetValue(&Scope{scope, param_bindings, false}, element)
                }
                return lastElement
            })
        }
        x := ParseMany(node)
        return x
    case List:
        if len(node) > 1 && node[0] == "lambda" {
            body := ParseMany(node.rest().rest())
            param_binding_fn := make_param_binding_fn(node.second())
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

func ParseMany(input List) []interface{} {
    output := List{}
    for _, i := range input {
        output = append(output, Parse(i))
    }

    return List(output)
}

var baseScope = &Scope{nil, builtins, false}

func ProcessTokens(scope *Scope, tokenized []interface{}, includeStdLib bool) interface{} {
    if includeStdLib {
        ProcessTokens(scope, TokenizeFile("/Users/cwfreeman/dev/go/src/glisp/stdlib.glisp"), false)
    }
    parsed := ParseMany(tokenized)
    value := GetValue(scope, parsed)
    switch v := value.(type) {
    case List:
        return []interface{}(v)
    default:
        return value
    }
}

func Process(input string)  interface{} {
    return ProcessTokens(baseScope, TokenizeString(input), true)
}

func ProcessFile(fname string) interface{} {
    return ProcessTokens(baseScope, TokenizeFile(fname), true)
}
