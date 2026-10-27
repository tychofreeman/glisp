package glisp

import (
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type Function func(_ *Scope, params List) interface{}

// listParam returns fn's first parameter as a List, treating nil (what car and
// cdr give for an empty list) as an empty list. Anything else is an error.
func listParam(fn string, params List) List {
	if len(params) == 0 || params[0] == nil {
		return nil
	}
	if x, isList := params[0].(List); isList {
		return x
	}
	panic(fmt.Sprintf("%s: expected a list, got %s", fn, described(params[0])))
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
	panic(fmt.Sprintf("%s: expected a number, got %s", fn, described(p)))
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

// times is (* a b c ...), the product; (*) is 1.
func times(_ *Scope, params List) interface{} {
	var product int64 = 1
	for _, p := range params {
		product *= number("*", p)
	}
	return product
}

// divide returns a builtin (op a b) that divides two numbers with div, as
// Go does: / truncates toward zero, and mod's result has a's sign.
func divide(op string, div func(a, b int64) int64) Function {
	return func(_ *Scope, params List) interface{} {
		if len(params) != 2 {
			panic(fmt.Sprintf("%s: expects 2 arguments, got %d", op, len(params)))
		}
		a, b := number(op, params[0]), number(op, params[1])
		if b == 0 {
			panic(op + ": division by zero")
		}
		return div(a, b)
	}
}

// isType returns the builtin name, (name x), which reports whether x's
// type is typ, as typeName names it.
func isType(name, typ string) Function {
	return func(_ *Scope, params List) interface{} {
		if len(params) != 1 {
			panic(fmt.Sprintf("%s: expects 1 argument, got %d", name, len(params)))
		}
		return typeName(params[0]) == typ
	}
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

func eq(_ *Scope, params List) interface{} {
	if len(params) != 2 {
		panic(fmt.Sprintf("EQ requires exactly 2 parameters; you have %d - %s", len(params), Write(params)))
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
	if _, isClosure := a.(*Closure); isClosure {
		return false // functions are never equal, as builtins aren't
	}
	ta := reflect.TypeOf(a)
	if ta != reflect.TypeOf(b) || !ta.Comparable() {
		return false
	}
	return a == b
}

func apply(scope *Scope, params List) interface{} {
	if len(params) == 0 {
		panic("apply: expects a function")
	}
	fn, isFunction := asFunction(params[0])
	if !isFunction {
		panic(fmt.Sprintf("apply: expected a function, got %s", described(params[0])))
	}
	return fn(scope, params.Rest())
}

// definedName returns the symbol a (def name value) or (defmacro name value)
// form defines, or panics naming the form fn if the form doesn't have exactly
// a symbol and a value.
func definedName(fn string, params List) string {
	sym, isSymbol := params.First().(Symbol)
	if !isSymbol {
		panic(fmt.Sprintf("%s: expected a symbol to define, got %s", fn, described(params.First())))
	}
	if len(params) != 2 {
		arguments := "arguments"
		if len(params) == 1 {
			arguments = "argument"
		}
		panic(fmt.Sprintf("%s: expects a name and a value, got %d %s", fn, len(params), arguments))
	}
	return sym.Str()
}

func printValues(scope *Scope, params List) interface{} {
	shown := make([]string, len(params))
	for i, v := range params {
		shown[i] = Show(v)
	}
	fmt.Println(strings.Join(shown, " "))
	os.Stdout.Sync()
	return List{}
}

var builtins = map[string]interface{}{
	"car":  Function(car),
	"cdr":  Function(cdr),
	"atom": Function(atom),
	"cons": Function(cons),
	"plus": Function(plus),
	"-":    Function(minus),
	"<":    compare("<", func(a, b int64) bool { return a < b }),
	">":    compare(">", func(a, b int64) bool { return a > b }),
	"<=":   compare("<=", func(a, b int64) bool { return a <= b }),
	">=":   compare(">=", func(a, b int64) bool { return a >= b }),
	"*":    Function(times),
	"/":    divide("/", func(a, b int64) int64 { return a / b }),
	"mod":  divide("mod", func(a, b int64) int64 { return a % b }),

	"number?":   isType("number?", "number"),
	"string?":   isType("string?", "string"),
	"symbol?":   isType("symbol?", "symbol"),
	"list?":     isType("list?", "list"),
	"function?": isType("function?", "function"),
	"boolean?":  isType("boolean?", "bool"),
	"nil?":      isType("nil?", "nil"),
	"eq":        Function(eq),
	"apply":     Function(apply),
	"p":         Function(printValues),
	"true":      true,
	"false":     false,
	"nil":       nil,
}

//go:embed stdlib.glisp
var stdlib string

var builtinScope = &Scope{table: builtins}

// stdlibForms is stdlib.glisp, read once.
var stdlibForms = Read(TokenizeString(stdlib)).(List)

// newScope makes the top-level scope for one run: its own definitions over
// the builtins, which it never changes, with the stdlib loaded.
func newScope() *Scope {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}}
	expander := NewExpander(scope)
	for _, form := range stdlibForms {
		Eval(expander.Expand(form), scope)
	}
	return scope
}

func ProcessTokens(scope *Scope, tokenized List) interface{} {
	expander := NewExpander(scope)
	var value interface{}
	for _, token := range tokenized {
		value = Eval(expander.Expand(Read(token)), scope)
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
