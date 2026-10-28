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

// Lambda is (lambda (params...) body...), (lambda (params... . rest)
// body...) or (lambda rest body...); the value of its last body form is its
// result.
type Lambda struct {
	Params   []string
	Rest     bool // the last parameter takes the arguments after the others, as a list
	Body     []Node
	Captured []string // filled in by Resolve: variables from enclosing lambdas
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
	Line int // the source line of the call, or 0
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
	return Write(n.Value)
}

func (n Quote) String() string { return "(quote " + Write(n.Datum) + ")" }

func (n Ref) String() string {
	if n.Loc != nil {
		return fmt.Sprintf("%s@%d.%d", n.Name, n.Loc.Depth, n.Loc.Index)
	}
	return n.Name
}

func (n If) String() string {
	return "(if " + n.Cond.String() + " " + n.Then.String() + " " + n.Else.String() + ")"
}

func (n Lambda) String() string {
	captured := ""
	if len(n.Captured) > 0 {
		captured = " [" + strings.Join(n.Captured, " ") + "]"
	}
	return "(lambda " + n.paramList() + captured + nodesString(n.Body) + ")"
}

// paramList is n's parameters as written: (a b), (a . rest) or rest.
func (n Lambda) paramList() string {
	if n.Rest && len(n.Params) == 1 {
		return n.Params[0]
	}
	params := n.Params
	if n.Rest {
		last := len(params) - 1
		params = append(append([]string{}, params[:last]...), ".", params[last])
	}
	return "(" + strings.Join(params, " ") + ")"
}

// arity is how many arguments n takes.
func (n Lambda) arity() arity {
	if n.Rest {
		return arity{len(n.Params) - 1, -1}
	}
	return arity{len(n.Params), len(n.Params)}
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
