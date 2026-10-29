package glisp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// A module is what a file's (module name (export name ...)) form declares:
// its name, the names it lets other modules use, and the line of the form.
type module struct {
	name    string
	exports []string
	line    int
}

// ExpandFileForm expands a form at the top level of a file, as ExpandTop
// does, except that a (module ...) form, which only the first may be,
// declares the file's module. It notes the names each form defines, which
// Finish checks the module's exports against.
func (e *Expander) ExpandFileForm(form interface{}) []Node {
	first := e.forms == 0
	e.forms++
	head, _ := listHead(form)
	switch head {
	case "module":
		e.declareModule(form.(List), first)
		return []Node{Quote{List{}}}
	case "import":
		return e.importModule(form.(List))
	}
	nodes := e.ExpandTop(form)
	if e.defined == nil {
		e.defined = map[string]bool{}
	}
	// An instance adds to a generic: it defines no name of the file's.
	if head != "definstance" {
		for _, name := range definedNames(form, nodes) {
			if globals := e.imports[name]; len(globals) > 0 {
				e.scope.at(e.formLine(form))
				panic(fmt.Sprintf("def: %s is imported from %s", name, strings.Split(globals[0], "/")[0]))
			}
			e.defined[name] = true
		}
		if head == "defmacro" {
			if e.macros == nil {
				e.macros = map[string]bool{}
			}
			e.macros[definedNames(form, nodes)[0]] = true
		}
	}
	return e.topNames(nodes)
}

// declareModule reads the (module name (export name ...)) form f.
func (e *Expander) declareModule(f List, first bool) {
	line := e.formLine(f)
	e.scope.at(line)
	if !first {
		panic("module: must be the first form of a file")
	}
	bad := func() {
		panic(fmt.Sprintf("module: expected (module name (export name ...)), got %s", Write(f)))
	}
	if len(f) < 2 || len(f) > 3 {
		bad()
	}
	name, isSymbol := f[1].(Symbol)
	if _, _, qualified := qualify(name.Str()); !isSymbol || qualified {
		bad()
	}
	m := &module{name: name.Str(), line: line}
	if len(f) == 3 {
		list, isList := f[2].(List)
		if head, _ := listHead(list); !isList || head != "export" {
			bad()
		}
		for _, x := range list[1:] {
			export, isSymbol := x.(Symbol)
			if !isSymbol {
				bad()
			}
			m.exports = append(m.exports, export.Str())
		}
	}
	e.module = m
	if e.own != "" {
		e.own = m.name
	}
}

// Finish checks the file's module, once every form is expanded: each name
// it exports must be one the file defines.
func (e *Expander) Finish() {
	if e.module == nil {
		return
	}
	for _, name := range e.module.exports {
		if !e.defined[name] {
			e.scope.at(e.module.line)
			panic(fmt.Sprintf("module %s exports %s, which it doesn't define", e.module.name, name))
		}
	}
}

// local is the top-level name a symbol of the file stands for: a qualified
// name, module/name, of the file's own module is its own name; of a module
// it imports, under its name or another, the name that module exports.
func (e *Expander) local(name string) string {
	module, rest, qualified := qualify(name)
	if !qualified {
		return name
	}
	if e.module != nil && module == e.module.name {
		return rest
	}
	if real, isAlias := e.aliases[module]; isAlias {
		module = real
	}
	m, imported := e.imported[module]
	if !imported {
		panic(fmt.Sprintf("%s: no module %s is imported", name, module))
	}
	if !m.exports[rest] {
		panic(fmt.Sprintf("%s: %s doesn't export %s", name, module, rest))
	}
	return module + "/" + rest
}

// A loader loads the modules a run imports, each once, for every file of
// the run.
type loader struct {
	root    *Scope                   // the run's top-level scope, which an exported macro is added to
	loaded  map[string]*loadedModule // the modules loaded so far, by name
	loading []string                 // the modules being loaded, outermost first
}

// A loadedModule is an imported module: its name and what it exports.
type loadedModule struct {
	name    string
	exports map[string]bool
}

// importModule reads an (import name), (import name (name ...)) or
// (import name as alias) form, and loads the module if no file of the run
// has yet: the module's definitions, under qualified names, are the form's
// nodes.
func (e *Expander) importModule(f List) []Node {
	line := e.formLine(f)
	e.scope.at(line)
	bad := func() {
		panic(fmt.Sprintf("import: expected (import name), (import name (name ...)) or (import name as alias), got %s", Write(f)))
	}
	name, isSymbol := symbolAt(f, 1)
	if !isSymbol || len(f) > 4 {
		bad()
	}
	var names []string
	alias := ""
	switch {
	case len(f) == 3:
		list, isList := f[2].(List)
		if !isList {
			bad()
		}
		for i := range list {
			n, isSymbol := symbolAt(list, i)
			if !isSymbol {
				bad()
			}
			names = append(names, n)
		}
	case len(f) == 4:
		as, _ := symbolAt(f, 2)
		a, isSymbol := symbolAt(f, 3)
		if as != "as" || !isSymbol {
			bad()
		}
		alias = a
	case len(f) != 2:
		bad()
	}
	m, nodes := e.load(name, line)
	e.scope.at(line)
	if e.imported == nil {
		e.imported, e.aliases, e.imports = map[string]*loadedModule{}, map[string]string{}, map[string][]string{}
	}
	e.imported[name] = m
	if alias != "" {
		e.aliases[alias] = name
	}
	for _, n := range names {
		if !m.exports[n] {
			panic(fmt.Sprintf("import: %s doesn't export %s", name, n))
		}
		e.imports[n] = appendNew(e.imports[n], name+"/"+n)
	}
	return append(nodes, Quote{List{}})
}

// symbolAt is the name of the symbol at l[i], if there is one.
func symbolAt(l List, i int) (string, bool) {
	if i >= len(l) {
		return "", false
	}
	s, isSymbol := l[i].(Symbol)
	if !isSymbol {
		return "", false
	}
	return s.Str(), true
}

// load loads the module name from the file name.glisp next to this file,
// unless it is loaded already (line is the import's): it expands the file's forms in a scope of
// their own, where its macros and their helpers run, then renames the names
// the module defines to name/..., and gives the nodes that define them.
func (e *Expander) load(name string, line int) (*loadedModule, []Node) {
	if e.loader == nil {
		e.loader = &loader{root: e.scope, loaded: map[string]*loadedModule{}}
	}
	l := e.loader
	if m, loaded := l.loaded[name]; loaded {
		return m, nil
	}
	if i := slices.Index(l.loading, name); i >= 0 {
		panic(fmt.Sprintf("import: a cycle: %s", strings.Join(append(append([]string{}, l.loading[i:]...), name), " imports ")))
	}
	if e.dir == "" {
		panic(fmt.Sprintf("import: no module %s: a program not read from a file can import only the stdlib's modules, and it has none yet", name))
	}
	path := filepath.Join(e.dir, name+".glisp")
	source, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("import: no module %s: there is no %s.glisp next to the file that imports it", name, name))
	}
	l.loading = append(l.loading, name)
	defer func() { l.loading = l.loading[:len(l.loading)-1] }()
	tokens, lines := tokenizeLines(string(source))
	child := NewExpander(&Scope{prev: l.root, table: map[string]interface{}{}, run: e.scope.run})
	child.lines, child.dir, child.loader, child.declared = Lines{}, filepath.Dir(path), l, declaredNames(tokens)
	var nodes []Node
	for _, token := range tokens {
		form := readLines(token, lines, child.lines)
		for _, node := range child.ExpandFileForm(form) {
			if def, isDef := node.(Def); isDef {
				if _, isLambda := def.Value.(Lambda); isLambda {
					Eval(node, child.scope) // so the module's later macros can call it
				}
			}
			nodes = append(nodes, node)
		}
	}
	child.Finish()
	e.scope.at(line)
	if child.module == nil || child.module.name != name {
		panic(fmt.Sprintf("import: %s.glisp is not the module %s: it must start with (module %s ...)", name, name, name))
	}
	m := &loadedModule{name: name, exports: map[string]bool{}}
	for _, export := range child.module.exports {
		m.exports[export] = true
	}
	// Every macro, so that a template can name one: privacy is for what a
	// program writes.
	for n, value := range child.scope.table {
		if macro, isMacro := value.(Macro); isMacro {
			l.root.add(name+"/"+n, macro)
		}
	}
	rename := func(n string) string {
		if child.defined[n] {
			return name + "/" + n
		}
		return n
	}
	for i, node := range nodes {
		nodes[i] = renameFree(node, rename, nil)
	}
	l.loaded[name] = m
	return m, nodes
}

// topNames is nodes, a top-level form of the file, with each top-level
// name it uses or defines made the global it means (topName).
func (e *Expander) topNames(nodes []Node) []Node {
	if len(e.imports) == 0 && e.own == "" {
		return nodes
	}
	for i, node := range nodes {
		nodes[i] = renameFree(node, e.topName, nil)
	}
	return nodes
}

// topName is the global a top-level name of the file means as a form is
// read: a name imported unqualified stands for the import; in a program,
// its own definition, by an earlier form or this one, comes next, then
// the stdlib's or a builtin's, and a name none of them has is the
// program's, defined later. A module's own names are renamed when the
// whole module is read (load), and its other names stay as they are.
func (e *Expander) topName(name string) string {
	if _, _, qualified := qualify(name); qualified {
		return name
	}
	switch globals := e.imports[name]; len(globals) {
	case 0:
	case 1:
		return globals[0]
	default:
		panic(fmt.Sprintf("%s is imported from more than one module: %s", name, strings.Join(globals, ", ")))
	}
	if e.own == "" || e.macros[name] {
		return name
	}
	if e.defined[name] || !e.library[name] {
		return e.own + "/" + name
	}
	return name
}

// libraryNames are the names the builtins and the stdlib define: what a
// program's name means unless the program defines it first.
var libraryNames = sync.OnceValue(func() map[string]bool {
	names := map[string]bool{}
	for name := range builtins {
		names[name] = true
	}
	nodes, macros := stdlibExpanded()
	for name := range macros {
		names[name] = true
	}
	for _, node := range nodes {
		if def, isDef := node.(Def); isDef {
			names[def.Name] = true
		}
	}
	return names
})

// programPrefix qualifies the names a program defines, so that they are
// apart from the stdlib's and the builtins': no symbol can be written with
// it. A program that is a module uses its module's name instead.
const programPrefix = "(program)"

// sourceName is a global's name as the program wrote it.
func sourceName(name string) string {
	return strings.TrimPrefix(name, programPrefix+"/")
}

// macroName is the name a call's head names a macro by: an imported one's
// qualified name, if the head is the name it was imported by.
func (e *Expander) macroName(head string) string {
	if globals := e.imports[head]; len(globals) == 1 {
		return globals[0]
	}
	return head
}

// renameFree is node with each name it uses or defines without a lambda in
// it binding the name, a top-level one, replaced by rename's. bound are the
// names the lambdas around node bind.
func renameFree(node Node, rename func(string) string, bound map[string]bool) Node {
	switch n := node.(type) {
	case Ref:
		if bound[n.Name] {
			return n
		}
		return Ref{Name: rename(n.Name), Loc: n.Loc}
	case Def:
		name := n.Name
		if !bound[name] {
			name = rename(name)
		}
		return Def{name, renameFree(n.Value, rename, bound)}
	case If:
		return If{renameFree(n.Cond, rename, bound), renameFree(n.Then, rename, bound), renameFree(n.Else, rename, bound)}
	case Apply:
		args := make([]Node, len(n.Args))
		for i, arg := range n.Args {
			args[i] = renameFree(arg, rename, bound)
		}
		return Apply{Fn: renameFree(n.Fn, rename, bound), Args: args, Line: n.Line}
	case Lambda:
		inner := map[string]bool{}
		for name := range bound {
			inner[name] = true
		}
		for _, name := range append(append([]string{}, n.Params...), localDefs(n.Body)...) {
			inner[name] = true
		}
		body := make([]Node, len(n.Body))
		for i, form := range n.Body {
			body[i] = renameFree(form, rename, inner)
		}
		return Lambda{Params: n.Params, Rest: n.Rest, Body: body, Captured: n.Captured}
	}
	return node
}

// qualify splits a qualified name, module/name, at its first /: a / with
// something on each side. / alone, division, is not one.
func qualify(name string) (module, rest string, qualified bool) {
	i := strings.Index(name, "/")
	if i <= 0 || i == len(name)-1 {
		return "", name, false
	}
	return name[:i], name[i+1:], true
}

// quasi is the code that builds the quasiquote template x at depth (1 for
// the outermost): x as data, except that (unquote y) at this depth is y's
// value and, in a list, (unquote-splicing y) is y's elements, and its
// symbols are what templateSymbol makes them. fresh holds the names this
// template made for those that end in #.
func (e *Expander) quasi(x interface{}, depth int, fresh map[string]Symbol) Node {
	switch x := x.(type) {
	case Symbol:
		return Quote{e.templateSymbol(x, fresh)}
	case List:
		if len(x) == 0 {
			return Quote{List{}}
		}
		if head, _ := listHead(x); len(x) == 2 {
			switch head {
			case "unquote":
				if depth == 1 {
					return e.Expand(x[1])
				}
				return e.quasiList(x, depth-1, fresh)
			case "unquote-splicing":
				if depth == 1 {
					panic("unquote-splicing: only in a list inside a quasiquote")
				}
				return e.quasiList(x, depth-1, fresh)
			case "quasiquote":
				return e.quasiList(x, depth+1, fresh)
			}
		}
		return e.quasiList(x, depth, fresh)
	}
	return Quote{x}
}

// quasiList is quasi for the elements of a list, built from the last with
// cons, and append for a (unquote-splicing y) before the last.
func (e *Expander) quasiList(l List, depth int, fresh map[string]Symbol) Node {
	var built Node = Quote{List{}}
	for i := len(l) - 1; i >= 0; i-- {
		if head, _ := listHead(l[i]); head == "unquote-splicing" && depth == 1 && len(l[i].(List)) == 2 {
			spliced := e.Expand(l[i].(List)[1])
			if i == len(l)-1 {
				built = spliced
			} else {
				built = Apply{Fn: Ref{Name: "append"}, Args: []Node{spliced, built}, Line: e.line}
			}
			continue
		}
		built = Apply{Fn: Ref{Name: "cons"}, Args: []Node{e.quasi(l[i], depth, fresh), built}, Line: e.line}
	}
	return built
}

// templateSymbol is a symbol a quasiquote template writes: a name ending in
// # becomes one no other code has, the same throughout the template; a
// name the file defines becomes that definition's global, which means it
// wherever the template's code ends up.
func (e *Expander) templateSymbol(s Symbol, fresh map[string]Symbol) Symbol {
	name := s.Str()
	if s.global {
		return s
	}
	if len(name) > 1 && strings.HasSuffix(name, "#") {
		if made, found := fresh[name]; found {
			return made
		}
		made := symbol(fmt.Sprintf("%s%d", name, freshNames.Add(1)))
		fresh[name] = made
		return made
	}
	if !e.defined[name] && !e.declared[name] {
		return s
	}
	switch {
	case e.own != "":
		return Symbol{name: e.own + "/" + name, global: true}
	case e.module != nil:
		return Symbol{name: e.module.name + "/" + name, global: true}
	}
	return s
}

// freshNames counts the names templates have made for those ending in #.
var freshNames atomic.Int64

// declaredNames are the names a module's forms define by def, defn,
// defmacro or defgeneric, read before the module is expanded, so that a
// template can mean a definition that comes after it.
func declaredNames(tokens List) map[string]bool {
	names := map[string]bool{}
	for _, token := range tokens {
		form, _ := Read(token).(List)
		switch head, _ := listHead(form); head {
		case "def", "defn", "defmacro", "defgeneric":
			if name, isSymbol := symbolAt(form, 1); isSymbol {
				names[name] = true
			}
		}
	}
	return names
}
