package glisp

import (
	"fmt"
	"strings"
)

// Eval evaluates node in scope: the interpreter, a backend for core glisp.
func Eval(node Node, scope *Scope) interface{} {
	for {
		switch n := node.(type) {
		case Const:
			return n.Value
		case Quote:
			return n.Datum
		case Ref:
			value, found := scope.lookup(symbol(n.Name))
			if !found {
				panic("Cannot resolve symbol " + n.Name)
			}
			if _, isMacro := value.(Macro); isMacro {
				panic(n.Name + " is a macro, not a value")
			}
			return value
		case If:
			// The branch is in tail position: evaluate it in this frame.
			if Eval(n.Cond, scope) == true {
				node = n.Then
			} else {
				node = n.Else
			}
		case Lambda:
			return &Closure{lambda: n, scope: scope}
		case Def:
			scope.add(n.Name, Eval(n.Value, scope))
			return List{}
		case Apply:
			scope.at(n.Line)
			fn := Eval(n.Fn, scope)
			args := make(List, len(n.Args))
			for i, arg := range n.Args {
				args[i] = Eval(arg, scope)
			}
			scope.at(n.Line) // again, after the arguments' calls
			switch f := fn.(type) {
			case *Closure:
				// The call's value is this Eval's, so the callee replaces this
				// frame: its body runs here, in a loop, not in a new Go call.
				// Tail calls, mutual recursion included, use no Go stack.
				scope = f.bind(args)
				body := f.lambda.Body
				if len(body) == 0 {
					return nil
				}
				for _, form := range body[:len(body)-1] {
					Eval(form, scope)
				}
				node = body[len(body)-1]
			case Function:
				return f(scope, args)
			default:
				panic(fmt.Sprintf("Cannot call %s in %v\n", described(fn), n))
			}
		default:
			panic(fmt.Sprintf("Eval: unknown node %T %v", node, node))
		}
	}
}

// Closure is the value of a lambda: its code and the scope it was made in.
// Eval runs a closure's body in place of the call; builtins are Functions.
type Closure struct {
	lambda Lambda
	scope  *Scope
}

// bind returns a new scope, chained to the closure's, that binds its
// parameters to args.
func (c *Closure) bind(args List) *Scope {
	params := c.lambda.Params
	if len(args) != len(params) {
		arguments := "arguments"
		if len(params) == 1 {
			arguments = "argument"
		}
		panic(fmt.Sprintf("lambda (%s) expects %d %s, got %d: %s", strings.Join(params, " "), len(params), arguments, len(args), Write(args)))
	}
	local := &Scope{prev: c.scope, table: make(map[string]interface{}, len(args)), run: c.scope.run}
	for i, name := range params {
		local.table[name] = args[i]
	}
	return local
}

// Call calls the closure with args, for callers outside Eval: apply and
// macros.
func (c *Closure) Call(args List) interface{} {
	scope := c.bind(args)
	var last interface{}
	for _, node := range c.lambda.Body {
		last = Eval(node, scope)
	}
	return last
}

// asFunction returns fn as a Function, if it is a builtin or a closure.
func asFunction(fn interface{}) (Function, bool) {
	switch f := fn.(type) {
	case Function:
		return f, true
	case *Closure:
		return func(_ *Scope, args List) interface{} { return f.Call(args) }, true
	}
	return nil, false
}
