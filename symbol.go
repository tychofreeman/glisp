package glisp

type Symbol struct{ name string }

func (sym Symbol) Str() string {
	return sym.name
}

func (sym Symbol) Eval(scope *Scope) interface{} {
	if resolved, ok := scope.lookup(sym); ok {
		return resolved
	}
	panic("Cannot resolve symbol " + sym.name)
}

func symbol(s string) Symbol {
	return Symbol{s}
}

func (sym Symbol) Append(c rune) Symbol {
	sym.name += string(c)
	return sym
}

func (sym Symbol) IsEmpty() bool {
	return sym.name == ""
}

func (sym Symbol) Type() TokenType {
	return SYMBOL
}
