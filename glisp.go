package glisp

import (
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type Valuable interface {
	Eval(*Scope) interface{}
}

type Function func(_ *Scope, params List) interface{}
type NonEvaluatingFunction func(_ *Scope, params List) interface{}

func GetValue(scope *Scope, source interface{}) interface{} {
	switch value := source.(type) {
	case int64:
		return value
	case string:
		return value
	case List:
		return value.Eval(scope)
	case Valuable:
		return value.Eval(scope)
	case Function, NonEvaluatingFunction:
		return value
	default:
		panic(fmt.Sprintf("Couldn't find anything of type %T (%v)\n", value, value))
	}
}

func quote(_ *Scope, params List) interface{} {
	if len(params) > 0 {
		return params[0]
	}
	panic(fmt.Sprintf("QUOTE takes exactly 1 argument; you have %v - %v\n", len(params), params))
}

// listParam returns fn's first parameter as a List, treating nil (what car and
// cdr give for an empty list) as an empty list. Anything else is an error.
func listParam(fn string, params List) List {
	if len(params) == 0 || params[0] == nil {
		return nil
	}
	if x, isList := params[0].(List); isList {
		return x
	}
	panic(fmt.Sprintf("%s: expected a list, got %T %v", fn, params[0], params[0]))
}

func car(_ *Scope, params List) interface{} {
	x := listParam("car", params)
	if len(x) > 0 {
		return x[0]
	}
	return nil
}

func cdr(_ *Scope, params List) interface{} {
	x := listParam("cdr", params)
	if len(x) > 0 {
		return x.Rest()
	}
	return nil
}

func atom(_ *Scope, params List) interface{} {
	if len(params) > 0 {
		switch params[0].(type) {
		case List:
			return false
		default:
			return true
		}
	}
	return false
}

func cons(_ *Scope, params List) interface{} {
	if len(params) == 1 {
		return params
	} else if len(params) == 2 {
		if atom(nil, params.Rest()) == true {
			return params
		} else {
			switch x := params[1].(type) {
			case List:
				output := List{params[0]}
				for _, i := range x {
					output = append(output, i)
				}
				return output
			}
		}
	}
	return nil
}

func plus(_ *Scope, params List) interface{} {
	var sum int64 = 0
	for _, p := range params {
		switch x := p.(type) {
		case int64:
			sum = sum + x
		case NumberToken:
			sum = sum + x.Value()
		default:
			panic(fmt.Sprintf("plus: expected a number, got %T %v", p, p))
		}
	}
	return sum
}

func if_(scope *Scope, params List) interface{} {
	if len(params) != 3 {
		panic(fmt.Sprintf("IF requires 3 parts - conditional, true expression and false expression. You have %v parts - %v.", len(params), params))
	}
	if true == GetValue(scope, params[0]) {
		return GetValue(scope, params[1])
	}
	return GetValue(scope, params[2])
}

func eq(_ *Scope, params List) interface{} {
	if len(params) != 2 {
		panic(fmt.Sprintf("EQ requires exactly 2 parameters; you have %v - %v", len(params), params))
	}
	return equal(params[0], params[1])
}

// equal reports whether two glisp values are equal: lists element by element,
// other values with ==. Values whose type can't be compared with ==, such as
// functions, are never equal.
func equal(a, b interface{}) bool {
	if la, isList := a.(List); isList {
		lb, isList := b.(List)
		if !isList || len(la) != len(lb) {
			return false
		}
		for i := range la {
			if !equal(la[i], lb[i]) {
				return false
			}
		}
		return true
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta := reflect.TypeOf(a)
	if ta != reflect.TypeOf(b) || !ta.Comparable() {
		return false
	}
	return a == b
}

func apply(scope *Scope, params List) interface{} {
	return GetValue(scope, params)
}

func define_(scope *Scope, params List) interface{} {
	name := params.First().(Token).Str()
	scope.add(name, GetValue(scope, params.Second()))
	return List{}
}

func macro(scope *Scope, params List) interface{} {
	name := params.First().(Token).Str()
	body := GetValue(scope, params.Second())
	macroFn := NonEvaluatingFunction(func(macroScope *Scope, macroParams List) interface{} {
		switch b := body.(type) {
		case Function:
			return b(macroScope, List{macroParams})
		case NonEvaluatingFunction:
			return b(macroScope, macroParams)
		default:
			panic(fmt.Sprintf("Could nt execute a function: %t %v\n%v\n", b, b, macroScope))
		}
	})
	scope.add(name, macroFn)
	return List{}
}

func print(scope *Scope, params List) interface{} {
	fmt.Printf("%v\n", params)
	os.Stdout.Sync()
	return List{}
}

var builtins = map[string]interface{}{
	"quote":    NonEvaluatingFunction(quote),
	"car":      Function(car),
	"cdr":      Function(cdr),
	"atom":     Function(atom),
	"cons":     Function(cons),
	"plus":     Function(plus),
	"if":       NonEvaluatingFunction(if_),
	"eq":       Function(eq),
	"apply":    Function(apply),
	"def":      NonEvaluatingFunction(define_),
	"defmacro": NonEvaluatingFunction(macro),
	"p":        Function(print),
}

func make_param_binding_fn(param_decls interface{}) func(interface{}) map[string]interface{} {
	param_names := []string{}
	switch x := param_decls.(type) {
	case List:
		for _, y := range x {
			switch z := y.(type) {
			case Token:
				param_names = append(param_names, z.Str())
			default:
				param_names = append(param_names, "")
			}
		}
	default:
		panic(fmt.Sprintf("Cannot build param bindings with unknown declaration type %T (expected a List of symbols)\n", x))
	}
	return func(theParams interface{}) map[string]interface{} {
		scope := map[string]interface{}{}
		switch params := theParams.(type) {
		case List:
			if len(params) != len(param_names) {
				arguments := "arguments"
				if len(param_names) == 1 {
					arguments = "argument"
				}
				panic(fmt.Sprintf("lambda (%s) expects %d %s, got %d: %v", strings.Join(param_names, " "), len(param_names), arguments, len(params), params))
			}
			for i := 0; i < len(param_names); i++ {
				scope[param_names[i]] = params[i]
			}
		default:
			panic(fmt.Sprintf("Cannot associate param names with values if values aren't in a List - found type %T\n", params))
		}
		return scope
	}
}

// Lambda is a parsed (lambda (params...) body...) form. Evaluating it makes a
// Function that closes over the scope it was evaluated in.
type Lambda struct {
	bind func(interface{}) map[string]interface{}
	body List
}

func (l Lambda) Eval(scope *Scope) interface{} {
	return Function(func(_ *Scope, params List) interface{} {
		local := &Scope{scope, l.bind(params), false}
		var last interface{}
		for _, element := range l.body {
			last = GetValue(local, element)
		}
		return last
	})
}

func Parse(source interface{}) interface{} {
	switch node := source.(type) {
	case NumberToken:
		return node
	case StringToken:
		return node
	case Symbol:
		return node
	case List:
		if node.IsLambda() {
			return Lambda{make_param_binding_fn(node.Second()), ParseMany(node.Rest().Rest())}
		}
		x := ParseMany(node)
		return x
	}
	return source
}

func ParseMany(input List) List {
	output := List{}
	for _, i := range input {
		output = append(output, Parse(i))
	}

	return List(output)
}

//go:embed stdlib.glisp
var stdlib string

var baseScope = &Scope{nil, builtins, false}

func ProcessTokens(scope *Scope, tokenized List, includeStdLib bool) interface{} {
	if includeStdLib {
		ProcessTokens(scope, TokenizeString(stdlib), false)
	}
	var value interface{}
	for _, form := range ParseMany(tokenized) {
		value = GetValue(scope, form)
	}
	return value
}

func Process(input string) interface{} {
	return ProcessTokens(baseScope, TokenizeString(input), true)
}

func ProcessFile(fname string) interface{} {
	return ProcessTokens(baseScope, TokenizeFile(fname), true)
}
