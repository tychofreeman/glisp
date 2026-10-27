package glisp

type Scope struct {
	prev  *Scope
	table map[string]interface{}
}

func (scope *Scope) lookup(name Symbol) (interface{}, bool) {
	if val, ok := scope.table[name.Str()]; ok {
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
