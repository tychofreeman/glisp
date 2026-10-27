package glisp

import (
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"strings"
)

type Function func(_ *Scope, params List) interface{}

func car(_ *Scope, args List) interface{} {
	if x, _ := args[0].(List); len(x) > 0 {
		return x[0]
	}
	return nil
}

func cdr(_ *Scope, args List) interface{} {
	if x, _ := args[0].(List); len(x) > 0 {
		return x.Rest()
	}
	return nil
}

func atom(_ *Scope, args List) interface{} {
	_, isList := args[0].(List)
	return !isList
}

func cons(_ *Scope, args List) interface{} {
	if len(args) == 1 {
		return List{args[0]}
	}
	switch rest := args[1].(type) {
	case nil:
		return List{args[0]}
	case List:
		return append(List{args[0]}, rest...)
	default:
		return List{args[0], rest}
	}
}

func plus(_ *Scope, args List) interface{} {
	var sum int64 = 0
	for _, n := range args {
		sum += n.(int64)
	}
	return sum
}

// minus is (- a b c ...), a-b-c..., and (- a), -a.
func minus(_ *Scope, args List) interface{} {
	if len(args) == 1 {
		return -args[0].(int64)
	}
	result := args[0].(int64)
	for _, n := range args[1:] {
		result -= n.(int64)
	}
	return result
}

// times is (* a b c ...), the product; (*) is 1.
func times(_ *Scope, args List) interface{} {
	var product int64 = 1
	for _, n := range args {
		product *= n.(int64)
	}
	return product
}

// divide returns a builtin (op a b) that divides two numbers with div, as
// Go does: / truncates toward zero, and mod's result has a's sign.
func divide(op string, div func(a, b int64) int64) Function {
	return func(_ *Scope, args List) interface{} {
		a, b := args[0].(int64), args[1].(int64)
		if b == 0 {
			panic(op + ": division by zero")
		}
		return div(a, b)
	}
}

// isType returns the builtin name, (name x), which reports whether x's
// type is typ, as typeName names it.
func isType(typ string) Function {
	return func(_ *Scope, args List) interface{} {
		return typeName(args[0]) == typ
	}
}

// compare returns a builtin (op a b) that compares two numbers with less.
func compare(less func(a, b int64) bool) Function {
	return func(_ *Scope, args List) interface{} {
		return less(args[0].(int64), args[1].(int64))
	}
}

func eq(_ *Scope, args List) interface{} {
	return equal(args[0], args[1])
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

func apply(scope *Scope, args List) interface{} {
	fn, _ := asFunction(args[0])
	return fn(scope, args.Rest())
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

var builtins = builtinTable()

//go:embed stdlib.glisp
var stdlib string

var builtinScope = &Scope{table: builtins}

// stdlibForms is stdlib.glisp, read once.
var stdlibForms = Read(TokenizeString(stdlib)).(List)

// newScope makes the top-level scope for one run: its own definitions over
// the builtins, which it never changes, with the stdlib loaded.
func newScope() *Scope {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}, run: &runState{}}
	expander := NewExpander(scope)
	for _, form := range stdlibForms {
		Eval(expander.Expand(form), scope)
	}
	return scope
}

func ProcessTokens(scope *Scope, tokenized List) interface{} {
	return processLines(scope, tokenized, nil)
}

// processLines evaluates the forms in tokens, whose lists start on the lines
// in lines, and returns the value of the last one.
func processLines(scope *Scope, tokens List, lines Lines) interface{} {
	expander := NewExpander(scope)
	expander.lines = Lines{}
	var value interface{}
	for _, token := range tokens {
		form := readLines(token, lines, expander.lines)
		node := expander.Expand(form)
		if scope.run != nil {
			scope.run.line = expander.formLine(form)
		}
		value = Eval(node, scope)
	}
	return value
}

// Process evaluates the program in input and returns the value of its last
// form. It panics on an error in the program; Run returns the error instead.
func Process(input string) interface{} {
	tokens, lines := tokenizeLines(input)
	return processLines(newScope(), tokens, lines)
}

func ProcessFile(fname string) interface{} {
	return ProcessTokens(newScope(), TokenizeFile(fname))
}

// Run evaluates the program in input and returns the value of its last form.
// Errors in the program, such as unbalanced parens, an unknown symbol or an
// argument of the wrong type, are returned rather than panicking.
func Run(input string) (value interface{}, err error) {
	scope := newScope()
	defer recoverAt(&err, scope.run)
	tokens, lines := tokenizeLines(input)
	return processLines(scope, tokens, lines), nil
}

// RunFile is Run for the program in the file fname.
func RunFile(fname string) (interface{}, error) {
	program, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return Run(string(program))
}

// recoverAt, deferred, turns a panic into an error in *err, naming the line
// of the call the run was making, if it knows it.
func recoverAt(err *error, run *runState) {
	if r := recover(); r != nil {
		msg := strings.TrimSpace(fmt.Sprint(r))
		if run != nil && run.line > 0 {
			msg = fmt.Sprintf("line %d: %s", run.line, msg)
		}
		*err = fmt.Errorf("%s", msg)
	}
}
