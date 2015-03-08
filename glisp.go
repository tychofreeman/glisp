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

func GetValueFromString(scope *Scope, value string) interface{} {
    if strings.HasPrefix(value, "\"") {
        return value[1:len(value)-1]
    } else if num, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
        return num
    }
    if y, ok := scope.lookup(value); ok {
        return y
    }
    panic(fmt.Sprintf("Invalid value %v - not a string or number, and could not be found in look-up table %v.", value, scope.table))
}



func GetValue(scope *Scope, source interface{}) interface{} {
    switch value := source.(type) {
    case string:
        return GetValueFromString(scope, value)
    case []interface{}:
        switch firstValue := value[0].(type) {
        case Function:
            params := GetValues(scope, rest(value))
            return firstValue(scope, params)
        case []interface{}:
            return GetValue(scope, firstValue)
        default:
            panic("A list should be either a function or a nested list (probably actually a high-order function)")
        }
    default:
        panic(fmt.Sprintf("Couldn't find anything of type %T (%v)\n", value, value))
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

func plus(_ *Scope, params[]interface{}) interface{} {
    var sum int64 = 0
    for _, p := range params {
        switch x := p.(type) {
        case int64:
            sum = sum + x
        }
    }
    return sum
}

func if_(_ *Scope, params[]interface{}) interface{} {
    if len(params) != 3 {
        panic(fmt.Sprintf("IF requires 3 parts - conditional, true expression and false expression. You have %v parts - %v.", len(params), params))
    }
    if true == params[0] {
        return params[1]
    }
    return params[2]
}

func eq(_ *Scope, params []interface{}) interface{} {
    if len(params) != 2 {
        panic(fmt.Sprintf("EQ requires exactly 2 parameters; you have %v - %v", len(params), params))
    }
    return params[0] == params[1]
}

var lookup = map[string]Function {
    "quote": quote,
    "car"  : car,
    "cdr"  : cdr,
    "atom" : atom,
    "cons" : cons,
    "plus" : plus,
    "if"   : if_,
    "eq"   : eq,
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

func Parse(source interface{}) interface{} {
    switch x := source.(type) {
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
    return source
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
