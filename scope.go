package glisp

type Scope struct {
	prev  *Scope
	table map[string]interface{}
	run   *runState // the run this scope belongs to
}

// runState is what a run knows about where it is, for errors.
type runState struct {
	line int // the line of the call being made
}

// at records that the run is making the call on line, if line is known.
func (scope *Scope) at(line int) {
	if line > 0 && scope.run != nil {
		scope.run.line = line
	}
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
