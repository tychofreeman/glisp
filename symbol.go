package glisp

// A Symbol is a name. A global one is a name a quasiquote template made
// mean a top-level definition, which the expander takes as it is.
type Symbol struct {
	name   string
	global bool
}

func (sym Symbol) Str() string {
	return sym.name
}

func symbol(s string) Symbol {
	return Symbol{name: s}
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
