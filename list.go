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
