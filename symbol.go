package glisp

import ( "fmt" )


type Symbol struct { name string }

func (sym Symbol) Str() string {
    return sym.name
}

func (sym Symbol) Sym() Sym {
    return Sym(sym.name)
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

type Sym string
func (sym Sym) Str() string { return string(sym) }
func symbol(s string) Sym {
    return Sym(s)
}
func (sym Sym) Append(c rune) Sym {
    return Sym(string(sym) + string(c))
}
