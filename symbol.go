package glisp

import ( "fmt" )


type Symbol struct { name string }

func (sym Symbol) Str() string {
    return sym.name
}

func (sym Symbol) Eval(scope *Scope) interface{} {
    if scope.isMacroScope {
        return sym
    } else if resolved, ok := scope.lookup(sym.Sym()); ok {
        return resolved
    } else {
        panic(fmt.Sprintf("Cannot resolve symbol %v in lookup %#v\n", sym.name, scope))
    }
}

func (sym Symbol) Sym() Sym {
    return Sym(sym)
}

type Sym Symbol
func symbol(s string) Sym {
    return Sym(Symbol{s})
}

func (sym Sym) Str() string {
    return Symbol(sym).Str()
}

func (sym Sym) Append(c rune) Sym {
    sym.name += string(c)
    return sym
}

func (sym Sym) IsEmpty() bool {
    return sym.name == ""
}
