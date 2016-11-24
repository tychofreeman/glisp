package glisp

import (
    "fmt"
    "strings"
    "strconv"
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


//
// It might not be a bad idea to move GetValues to a sum-type of {Symbol|Function|...}. Then,
// it might be good to add type info around that structure as well.
//
type Symbol struct { name string }
func (sym Symbol) Eval(scope *Scope) interface{} {
    if resolved, ok := scope.lookup(sym.name); ok {
        return resolved
    } else {
        panic(fmt.Sprintf("Cannot resolve symbol %v in lookup %v\n", sym.name, scope))
    }
}


type ParamsList List
type Function func(_ *Scope, params List) interface{}
type NonEvaluatingFunction func(_ *Scope, params List) interface{}

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

type Valuable interface {
    Eval(*Scope) interface{}
}

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
    return rest([]interface{}(all))
}

func rest(all []interface{}) List {
    if all != nil && len(all) > 0 {
        return List(all[1:])
    }
    return nil
}
func (value List) Eval(scope *Scope) interface{} {
    switch firstValue := value[0].(type) {
    case NonEvaluatingFunction:
        return firstValue(scope, value.rest())
    case Function:
        params := GetValues(scope, rest(value))
        return firstValue(scope, params)
    case Valuable:
        switch symb := firstValue.Eval(scope).(type) {
        case NonEvaluatingFunction:
            return symb(scope, rest(value))
        case Function:
            params := GetValues(scope, rest(value))
            return symb(scope, params)
        default:
            panic(fmt.Sprintf("A list should be either a function or a nested list (probably actually a high-order function) - found %T %v in %v\n", firstValue, firstValue, value))
        }
    case []interface{}:
        return List(firstValue).Eval(scope)
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
                return rest(x)
            }
        case []interface{}:
            if len(x) > 0 {
                return rest(x)
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
        if atom(nil, rest(params)) == true {
            return params
        } else {
            switch x := params[1].(type) {
            case List:
                output := []interface{}{params[0]}
                for _, i := range x {
                    output = append(output, i)
                }
                return output
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

func if_(_ *Scope, params List) interface{} {
    if len(params) != 3 {
        panic(fmt.Sprintf("IF requires 3 parts - conditional, true expression and false expression. You have %v parts - %v.", len(params), params))
    }
    if true == params[0] {
        return params[1]
    }
    return params[2]
}

func eq(_ *Scope, params List) interface{} {
    if len(params) != 2 {
        panic(fmt.Sprintf("EQ requires exactly 2 parameters; you have %v - %v", len(params), params))
    }
    return params[0] == params[1]
}

var builtins = map[string]interface{} {
    "quote": NonEvaluatingFunction(quote),
    "car"  : Function(car),
    "cdr"  : Function(cdr),
    "atom" : Function(atom),
    "cons" : Function(cons),
    "plus" : Function(plus),
    "if"   : Function(if_),
    "eq"   : Function(eq),
    // "apply" wouldn't suck...
}


func make_param_binding_fn(param_decls interface{}) (func([]interface{}) map[string]interface{}) {
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
            body := ParseMany(rest(rest(node)))
            param_binding_fn := make_param_binding_fn(List(node).second())
            return Function(func(scope *Scope, params List) interface{} {
                param_bindings := param_binding_fn(params)
                return GetValue(&Scope{scope, param_bindings}, last(body))
            })
        }
        return ParseMany(node)
    }
    return source
}

func ParseMany(input List) []interface{} {
    output := []interface{}{}
    for _, i := range input {
        output = append(output, Parse(i))
    }
    return output
}

func Process(input string)  interface{} {
    tokenized := TokenizeString(input)
    parsed := ParseMany(tokenized)
    //fmt.Printf("Parsed: %v\n", parsed)
    value := GetValue(&Scope{nil, builtins}, parsed)
    //fmt.Printf("Value: %v\n", value)
    switch v := value.(type) {
    case List:
        return []interface{}(v)
    default:
        return value
    }
}
