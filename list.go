package glisp

import ( "fmt" )

type List []interface{}

func (all List) First() interface{} {
    if all != nil && len(all) > 0 {
        return all[0]
    }
    return nil
}

func (all List) Second() interface{} {
    return all.Rest().First()
}

func (all List) Rest() List {
    if all != nil && len(all) > 0 {
        return List(all[1:])
    }
    return nil
}

func (node List) IsLambda() bool {
    if len(node) > 1 {
        token, isToken := node[0].(Token)
        return isToken && token.Str() == "lambda"
    }
    return false
}

func (value List) Eval(scope *Scope) interface{} {
    if scope.isMacroScope {
        return value
    }
    switch firstValue := value[0].(type) {
    case NonEvaluatingFunction:
        return firstValue(scope, value.Rest())
    case Function:
        params := value.Rest().GetValues(scope)
        return firstValue(scope, params)
    case List:
        lastElement := interface{}(nil)
        for _, element := range value {
            lastElement = GetValue(scope, element)
        }
        return lastElement
    case Valuable:
        switch symb := firstValue.Eval(scope).(type) {
        case NonEvaluatingFunction:
            x := symb(scope, value.Rest())
            return x
        case Function:
            params := value.Rest().GetValues(scope)
            x := symb(scope, params)
            return x
        default:
            panic(fmt.Sprintf("A list should be either a function or a nested list (probably actually a high-order function) - found %T %v in %v\n", firstValue, firstValue, value))
        }
    }
    panic(fmt.Sprintf("Could not evaluate list: %v\n", value))
}

func (things List) GetValues(scope *Scope) List {
    output := List{}
    for _, i := range things {
        output = append(output, GetValue(scope, i))
    }
    return output
}
