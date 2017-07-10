package glisp

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
