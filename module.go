package glisp

import (
	"fmt"
	"strings"
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
	if head, _ := listHead(form); head == "module" {
		e.declareModule(form.(List), first)
		return []Node{Quote{List{}}}
	}
	nodes := e.ExpandTop(form)
	if e.defined == nil {
		e.defined = map[string]bool{}
	}
	for _, name := range definedNames(form, nodes) {
		e.defined[name] = true
	}
	return nodes
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

// local is name as the file's own: a qualified name, module/name, must name
// the file's own module, since no other can be imported yet.
func (e *Expander) local(name string) string {
	module, rest, qualified := qualify(name)
	if !qualified {
		return name
	}
	if e.module != nil && module == e.module.name {
		return rest
	}
	panic(fmt.Sprintf("%s: no module %s is imported", name, module))
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
