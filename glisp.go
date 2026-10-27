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
	switch len(params) {
	case 1:
		return List{params[0]}
	case 2:
		switch rest := params[1].(type) {
		case nil:
			return List{params[0]}
		case List:
			return append(List{params[0]}, rest...)
		default:
			return List{params[0], rest}
		}
	}
	panic(fmt.Sprintf("cons: expects 1 or 2 arguments, got %d", len(params)))
}

func plus(_ *Scope, params List) interface{} {
	var sum int64 = 0
	for _, p := range params {
		sum += number("plus", p)
	}
	return sum
}

// number returns p as an int64, or panics naming the builtin fn.
func number(fn string, p interface{}) int64 {
	if n, isNumber := p.(int64); isNumber {
		return n
	}
	panic(fmt.Sprintf("%s: expected a number, got %T %v", fn, p, p))
}

// minus is (- a b c ...), a-b-c..., and (- a), -a.
func minus(_ *Scope, params List) interface{} {
	if len(params) == 0 {
		panic("-: expects at least 1 argument, got 0")
	}
	if len(params) == 1 {
		return -number("-", params[0])
	}
	result := number("-", params[0])
	for _, p := range params[1:] {
		result -= number("-", p)
	}
	return result
}

// compare returns a builtin (op a b) that compares two numbers with less.
func compare(op string, less func(a, b int64) bool) Function {
	return func(_ *Scope, params List) interface{} {
		if len(params) != 2 {
			panic(fmt.Sprintf("%s: expects 2 arguments, got %d", op, len(params)))
		}
		return less(number(op, params[0]), number(op, params[1]))
	}
}

func ifThenElse(scope *Scope, params List) interface{} {
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

// definedName returns the symbol a (def name ...) or (defmacro name ...) form
// defines, or panics naming the form fn.
func definedName(fn string, params List) string {
	if sym, isSymbol := params.First().(Symbol); isSymbol {
		return sym.Str()
	}
	panic(fmt.Sprintf("%s: expected a symbol to define, got %T %v", fn, params.First(), params.First()))
}

func define(scope *Scope, params List) interface{} {
	name := definedName("def", params)
	scope.add(name, GetValue(scope, params.Second()))
	return List{}
}

func macro(scope *Scope, params List) interface{} {
	name := definedName("defmacro", params)
	body := GetValue(scope, params.Second())
	macroFn := NonEvaluatingFunction(func(macroScope *Scope, macroParams List) interface{} {
		switch b := body.(type) {
		case Function:
			return GetValue(macroScope, b(macroScope, List{macroParams}))
		case NonEvaluatingFunction:
			return b(macroScope, macroParams)
		default:
			panic(fmt.Sprintf("defmacro: expected a function, got %T %v\n", b, b))
		}
	})
	scope.add(name, macroFn)
	return List{}
}

func printValues(scope *Scope, params List) interface{} {
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
	"-":        Function(minus),
	"<":        compare("<", func(a, b int64) bool { return a < b }),
	">":        compare(">", func(a, b int64) bool { return a > b }),
	"if":       NonEvaluatingFunction(ifThenElse),
	"eq":       Function(eq),
	"apply":    Function(apply),
	"def":      NonEvaluatingFunction(define),
	"defmacro": NonEvaluatingFunction(macro),
	"p":        Function(printValues),
	"true":     true,
	"false":    false,
	"nil":      nil,
}

func paramBinder(decls interface{}) func(interface{}) map[string]interface{} {
	names := []string{}
	switch x := decls.(type) {
	case List:
		for _, y := range x {
			switch z := y.(type) {
			case Token:
				names = append(names, z.Str())
			default:
				names = append(names, "")
			}
		}
	default:
		panic(fmt.Sprintf("Cannot build param bindings with unknown declaration type %T (expected a List of symbols)\n", x))
	}
	return func(theParams interface{}) map[string]interface{} {
		scope := map[string]interface{}{}
		switch params := theParams.(type) {
		case List:
			if len(params) != len(names) {
				arguments := "arguments"
				if len(names) == 1 {
					arguments = "argument"
				}
				panic(fmt.Sprintf("lambda (%s) expects %d %s, got %d: %v", strings.Join(names, " "), len(names), arguments, len(params), params))
			}
			for i := 0; i < len(names); i++ {
				scope[names[i]] = params[i]
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
		local := &Scope{prev: scope, table: l.bind(params)}
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
		return node.Value()
	case StringToken:
		return node.Value()
	case Symbol:
		return node
	case List:
		if node.IsLambda() {
			return Lambda{paramBinder(node.Second()), ParseMany(node.Rest().Rest())}
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

var builtinScope = &Scope{table: builtins}

// stdlibForms is stdlib.glisp, parsed once.
var stdlibForms = ParseMany(TokenizeString(stdlib))

// newScope makes the top-level scope for one run: its own definitions over
// the builtins, which it never changes, with the stdlib loaded.
func newScope() *Scope {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}}
	for _, form := range stdlibForms {
		GetValue(scope, form)
	}
	return scope
}

func ProcessTokens(scope *Scope, tokenized List) interface{} {
	var value interface{}
	for _, form := range ParseMany(tokenized) {
		value = GetValue(scope, form)
	}
	return value
}

// Process evaluates the program in input and returns the value of its last
// form. It panics on an error in the program; Run returns the error instead.
func Process(input string) interface{} {
	return ProcessTokens(newScope(), TokenizeString(input))
}

func ProcessFile(fname string) interface{} {
	return ProcessTokens(newScope(), TokenizeFile(fname))
}

// Run evaluates the program in input and returns the value of its last form.
// Errors in the program, such as unbalanced parens, an unknown symbol or an
// argument of the wrong type, are returned rather than panicking.
func Run(input string) (value interface{}, err error) {
	defer recoverError(&err)
	return Process(input), nil
}

// RunFile is Run for the program in the file fname.
func RunFile(fname string) (interface{}, error) {
	program, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return Run(string(program))
}

// recoverError, deferred, turns a panic into an error in *err.
func recoverError(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("%s", strings.TrimSpace(fmt.Sprint(r)))
	}
}
