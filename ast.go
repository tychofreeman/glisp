package glisp

import (
	"fmt"
	"strings"
)

// Node is a form of core glisp: what is left of a program once its macros
// are expanded. Every backend (the interpreter, and later compilers) handles
// exactly these types.
type Node interface {
	node()
	String() string
}

// Const is a constant: an int64, a string, true, false or nil.
type Const struct{ Value interface{} }

// Quote is (quote datum): datum as data, not evaluated.
type Quote struct{ Datum interface{} }

// Ref is a reference to a variable. Loc is nil until a resolve pass fills it
// in for backends that compile variable access.
type Ref struct {
	Name string
	Loc  *Loc
}

// If is (if cond then else).
type If struct{ Cond, Then, Else Node }

// Lambda is (lambda (params...) body...); the value of its last body form is
// its result.
type Lambda struct {
	Params []string
	Body   []Node
}

// Def is (def name value).
type Def struct {
	Name  string
	Value Node
}

// Apply is a function call, (fn args...).
type Apply struct {
	Fn   Node
	Args []Node
}

// Loc is where a variable lives, for backends that compile variable access:
// Depth scopes out from the reference, at slot Index.
type Loc struct{ Depth, Index int }

func (Const) node()  {}
func (Quote) node()  {}
func (Ref) node()    {}
func (If) node()     {}
func (Lambda) node() {}
func (Def) node()    {}
func (Apply) node()  {}

func (n Const) String() string {
	if s, isString := n.Value.(string); isString {
		return fmt.Sprintf("%q", s)
	}
	if n.Value == nil {
		return "nil"
	}
	return fmt.Sprint(n.Value)
}

func (n Quote) String() string { return "(quote " + datumString(n.Datum) + ")" }

func (n Ref) String() string { return n.Name }

func (n If) String() string {
	return "(if " + n.Cond.String() + " " + n.Then.String() + " " + n.Else.String() + ")"
}

func (n Lambda) String() string {
	return "(lambda (" + strings.Join(n.Params, " ") + ")" + nodesString(n.Body) + ")"
}

func (n Def) String() string { return "(def " + n.Name + " " + n.Value.String() + ")" }

func (n Apply) String() string { return "(" + n.Fn.String() + nodesString(n.Args) + ")" }

// nodesString is the nodes, each preceded by a space.
func nodesString(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(" " + n.String())
	}
	return b.String()
}

// datumString prints quoted data the way it would be written in glisp.
func datumString(datum interface{}) string {
	switch d := datum.(type) {
	case List:
		parts := make([]string, len(d))
		for i, item := range d {
			parts[i] = datumString(item)
		}
		return "(" + strings.Join(parts, " ") + ")"
	case Symbol:
		return d.Str()
	case nil:
		return "nil"
	case string:
		return fmt.Sprintf("%q", d)
	default:
		return fmt.Sprint(d)
	}
}
