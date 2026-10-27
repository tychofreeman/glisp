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
	if len(value) == 0 {
		return nil
	}
	switch fn := GetValue(scope, value[0]).(type) {
	case NonEvaluatingFunction:
		return fn(scope, value.Rest())
	case Function:
		return fn(scope, value.Rest().GetValues(scope))
	default:
		panic(fmt.Sprintf("Cannot call %T %v in %v\n", fn, fn, value))
	}
}

func (things List) GetValues(scope *Scope) List {
    output := List{}
    for _, i := range things {
        output = append(output, GetValue(scope, i))
    }
    return output
}
