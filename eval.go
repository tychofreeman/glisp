package glisp

import (
	"fmt"
	"strings"
)

// Eval evaluates node in scope: the interpreter, a backend for core glisp.
func Eval(node Node, scope *Scope) interface{} {
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
		if Eval(n.Cond, scope) == true {
			return Eval(n.Then, scope)
		}
		return Eval(n.Else, scope)
	case Lambda:
		return closure(n, scope)
	case Def:
		scope.add(n.Name, Eval(n.Value, scope))
		return List{}
	case Apply:
		fn := Eval(n.Fn, scope)
		args := make(List, len(n.Args))
		for i, arg := range n.Args {
			args[i] = Eval(arg, scope)
		}
		if f, isFunction := fn.(Function); isFunction {
			return f(scope, args)
		}
		panic(fmt.Sprintf("Cannot call %s in %v\n", described(fn), n))
	}
	panic(fmt.Sprintf("Eval: unknown node %T %v", node, node))
}

// closure makes the Function a Lambda evaluates to: its body runs in a new
// scope, chained to scope, that binds its parameters.
func closure(lambda Lambda, scope *Scope) Function {
	return func(_ *Scope, args List) interface{} {
		if len(args) != len(lambda.Params) {
			arguments := "arguments"
			if len(lambda.Params) == 1 {
				arguments = "argument"
			}
			panic(fmt.Sprintf("lambda (%s) expects %d %s, got %d: %s", strings.Join(lambda.Params, " "), len(lambda.Params), arguments, len(args), Write(args)))
		}
		local := &Scope{prev: scope, table: make(map[string]interface{}, len(args))}
		for i, name := range lambda.Params {
			local.table[name] = args[i]
		}
		var last interface{}
		for _, node := range lambda.Body {
			last = Eval(node, local)
		}
		return last
	}
}
