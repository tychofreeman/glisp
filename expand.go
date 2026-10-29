package glisp

import "fmt"

// Read turns tokenized source into data: number, float and string tokens
// become int64, float64 and string, symbols stay Symbols, and lists stay Lists.
func Read(token interface{}) interface{} {
	return readLines(token, nil, nil)
}

// readLines is Read, carrying the line of each list it reads, from from, to
// the list it makes, in to.
func readLines(token interface{}, from, to Lines) interface{} {
	switch t := token.(type) {
	case NumberToken:
		return t.Value()
	case FloatToken:
		return t.Value()
	case StringToken:
		return t.Value()
	case List:
		data := make(List, len(t))
		for i, item := range t {
			data[i] = readLines(item, from, to)
		}
		if len(t) > 0 && to != nil {
			if line, found := from[&t[0]]; found {
				to[&data[0]] = line
			}
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
	scope    *Scope
	lines    Lines                    // the line of each list read from source
	line     int                      // the line of the innermost list being expanded
	module   *module                  // what the file's module form declared, if it has one
	forms    int                      // how many top-level forms of the file ExpandFileForm has seen
	defined  map[string]bool          // the names the file's top-level forms define
	dir      string                   // the directory of the file, where its imports are found; "" if it isn't from one
	loader   *loader                  // what loads the run's modules
	imported map[string]*loadedModule // the modules the file imports, by name
	aliases  map[string]string        // the modules it imports under another name, by that name
	imports  map[string][]string      // what each name it imports unqualified stands for
	own      string                   // what qualifies the names a program defines (programPrefix, or its module's name); "" for a module loaded by an import, or the stdlib
	macros   map[string]bool          // the macros the file defines
	library  map[string]bool          // in a program, the names the builtins and the stdlib define (libraryNames)
}

// NewExpander returns an Expander that finds macros in, and defines them in,
// scope.
func NewExpander(scope *Scope) *Expander {
	return &Expander{scope: scope}
}

// Expand turns one form into a Node. A begin here is a sequence with a scope
// of its own: a call of a lambda without parameters.
func (e *Expander) Expand(form interface{}) Node {
	node := e.expand(form)
	if forms, isBegin := node.(begin); isBegin {
		return Apply{Fn: Lambda{Body: forms}, Line: e.line}
	}
	return node
}

// ExpandTop expands one form of a body, the top level's or a lambda's: a
// begin gives its forms, so that the defs in it define in that scope.
func (e *Expander) ExpandTop(form interface{}) []Node {
	node := e.expand(form)
	forms, isBegin := node.(begin)
	if !isBegin {
		return []Node{node}
	}
	if len(forms) == 0 {
		return []Node{Const{nil}}
	}
	return forms
}

// ExpandBody expands the forms of a body, each with ExpandTop.
func (e *Expander) ExpandBody(forms List) []Node {
	var nodes []Node
	for _, form := range forms {
		nodes = append(nodes, e.ExpandTop(form)...)
	}
	return nodes
}

// begin is (begin form...) while it is expanded: the forms of a sequence,
// which a body splices in and anywhere else is a call of a lambda without
// parameters. No backend sees one.
type begin []Node

func (begin) node() {}

func (n begin) String() string { return "(begin" + nodesString(n) + ")" }

// expand turns one form into a Node, which is a begin for (begin ...) or a
// macro that expands to one.
func (e *Expander) expand(form interface{}) Node {
	switch f := form.(type) {
	case Symbol:
		return Ref{Name: e.local(f.Str())}
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

// formLine is the line of the top-level form, if it is a list read from
// source, or 0.
func (e *Expander) formLine(form interface{}) int {
	if l, isList := form.(List); isList && len(l) > 0 {
		return e.lines[&l[0]]
	}
	return 0
}

func (e *Expander) expandList(f List) Node {
	if len(f) == 0 {
		return Const{nil}
	}
	// A call gets the line of the list it was read from; code a macro made
	// gets the line of the macro call.
	if line, found := e.lines[&f[0]]; found {
		saved := e.line
		e.line = line
		e.scope.at(line)
		defer func() { e.line = saved }()
	}
	if head, isSymbol := f[0].(Symbol); isSymbol {
		head = symbol(e.macroName(e.local(head.Str())))
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
			params, rest := paramNames(f[1])
			return Lambda{Params: params, Rest: rest, Body: e.ExpandBody(f[2:])}
		case "def":
			name := definedName("def", f.Rest())
			return Def{name, e.Expand(f[2])}
		case "defmacro":
			name := definedName("defmacro", f.Rest())
			value := e.evalMacroBody(f[2])
			fn, isFunction := asFunction(value)
			if !isFunction {
				panic(fmt.Sprintf("defmacro: expected a function, got %s\n", described(value)))
			}
			e.scope.add(name, Macro{fn})
			if e.own != "" {
				// Under the name a reference read before it means, too.
				e.scope.add(e.own+"/"+name, Macro{fn})
			}
			return Quote{List{}}
		case "begin":
			return begin(e.ExpandBody(f[1:]))
		}
		if macro, isMacro := e.macro(head); isMacro {
			return e.expand(withoutPairs(macro.fn(e.scope, List{f.Rest()})))
		}
	}
	return Apply{Fn: e.Expand(f[0]), Args: e.ExpandAll(f[1:]), Line: e.line}
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
	return Eval(e.topNames([]Node{e.Expand(form)})[0], e.scope)
}

// paramNames returns the names in a lambda's parameter list, and whether
// the last takes the rest of the arguments: (a b . rest), or one symbol.
func paramNames(decls interface{}) ([]string, bool) {
	if sym, isSymbol := decls.(Symbol); isSymbol {
		return []string{sym.Str()}, true
	}
	list, isList := decls.(List)
	if !isList {
		panic(fmt.Sprintf("Cannot build param bindings with unknown declaration type %s (expected a list of symbols, or one symbol)\n", typeName(decls)))
	}
	names := []string{}
	rest := false
	for i, decl := range list {
		sym, isSymbol := decl.(Symbol)
		if !isSymbol {
			panic(fmt.Sprintf("lambda: parameters must be symbols, got %s", described(decl)))
		}
		if sym.Str() == "." {
			if i != len(list)-2 {
				panic("lambda: a . in the parameters must come just before the last one")
			}
			rest = true
			continue
		}
		names = append(names, sym.Str())
	}
	return names, rest
}
