package glisp

import "fmt"

// Read turns tokenized source into data: number and string tokens become
// int64 and string, symbols stay Symbols, and lists stay Lists.
func Read(token interface{}) interface{} {
	switch t := token.(type) {
	case NumberToken:
		return t.Value()
	case StringToken:
		return t.Value()
	case List:
		data := make(List, len(t))
		for i, item := range t {
			data[i] = Read(item)
		}
		return data
	default:
		return token
	}
}

// Macro is a macro defined with defmacro: a function from the macro's
// unevaluated arguments, as one list, to the code that replaces the call.
type Macro struct{ fn Function }

// Expander turns data read from source into core glisp Nodes, expanding
// macros as it goes. Macros are found in scope, where defmacro puts them,
// so a macro applies to the forms expanded after its definition.
type Expander struct {
	scope *Scope
}

// NewExpander returns an Expander that finds macros in, and defines them in,
// scope.
func NewExpander(scope *Scope) *Expander {
	return &Expander{scope: scope}
}

// Expand turns one form into a Node.
func (e *Expander) Expand(form interface{}) Node {
	switch f := form.(type) {
	case Symbol:
		return Ref{Name: f.Str()}
	case List:
		return e.expandList(f)
	default:
		return Const{form}
	}
}

// ExpandAll expands each of forms.
func (e *Expander) ExpandAll(forms List) []Node {
	nodes := make([]Node, len(forms))
	for i, form := range forms {
		nodes[i] = e.Expand(form)
	}
	return nodes
}

func (e *Expander) expandList(f List) Node {
	if len(f) == 0 {
		return Const{nil}
	}
	if head, isSymbol := f[0].(Symbol); isSymbol {
		switch head.Str() {
		case "quote":
			if len(f) < 2 {
				panic(fmt.Sprintf("QUOTE takes exactly 1 argument; you have %d - %s\n", len(f)-1, Write(f.Rest())))
			}
			return Quote{f[1]}
		case "if":
			if len(f) != 4 {
				panic(fmt.Sprintf("IF requires 3 parts - conditional, true expression and false expression. You have %d parts - %s.", len(f)-1, Write(f.Rest())))
			}
			return If{e.Expand(f[1]), e.Expand(f[2]), e.Expand(f[3])}
		case "lambda":
			if len(f) < 2 {
				panic("lambda: expects a parameter list")
			}
			return Lambda{Params: paramNames(f[1]), Body: e.ExpandAll(f[2:])}
		case "def":
			name := definedName("def", f.Rest())
			return Def{name, e.Expand(f[2])}
		case "defmacro":
			name := definedName("defmacro", f.Rest())
			value := e.evalMacroBody(f[2])
			fn, isFunction := value.(Function)
			if !isFunction {
				panic(fmt.Sprintf("defmacro: expected a function, got %s\n", described(value)))
			}
			e.scope.add(name, Macro{fn})
			return Quote{List{}}
		}
		if macro, isMacro := e.macro(head); isMacro {
			return e.Expand(macro.fn(e.scope, List{f.Rest()}))
		}
	}
	return Apply{e.Expand(f[0]), e.ExpandAll(f[1:])}
}

// macro returns the macro sym names, if it names one.
func (e *Expander) macro(sym Symbol) (Macro, bool) {
	value, found := e.scope.lookup(sym)
	if !found {
		return Macro{}, false
	}
	macro, isMacro := value.(Macro)
	return macro, isMacro
}

// evalMacroBody evaluates the value of a defmacro, which should be a function.
func (e *Expander) evalMacroBody(form interface{}) interface{} {
	return Eval(e.Expand(form), e.scope)
}

// paramNames returns the names in a lambda's parameter list.
func paramNames(decls interface{}) []string {
	list, isList := decls.(List)
	if !isList {
		panic(fmt.Sprintf("Cannot build param bindings with unknown declaration type %s (expected a list of symbols)\n", typeName(decls)))
	}
	names := make([]string, len(list))
	for i, decl := range list {
		sym, isSymbol := decl.(Symbol)
		if !isSymbol {
			panic(fmt.Sprintf("lambda: parameters must be symbols, got %s", described(decl)))
		}
		names[i] = sym.Str()
	}
	return names
}
