package glisp

import (
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
)

type Function func(_ *Scope, params List) interface{}

func car(_ *Scope, args List) interface{} {
	first, _, _ := uncons(args[0])
	return first
}

func cdr(_ *Scope, args List) interface{} {
	_, rest, _ := uncons(args[0])
	return rest
}

func atom(_ *Scope, args List) interface{} {
	return !isList(args[0])
}

// cons puts a value in front of a list, sharing it: a Pair.
func cons(_ *Scope, args List) interface{} {
	if len(args) == 1 {
		return List{args[0]}
	}
	switch rest := args[1].(type) {
	case nil:
		return List{args[0]}
	case List:
		if len(rest) == 0 {
			return List{args[0]}
		}
		return &Pair{args[0], rest}
	case *Pair:
		return &Pair{args[0], rest}
	default:
		return List{args[0], rest}
	}
}

func plus(_ *Scope, args List) interface{} {
	if fs, isFloat := asFloats(args); isFloat {
		sum := 0.0
		for _, f := range fs {
			sum += f
		}
		return sum
	}
	var sum int64 = 0
	for _, n := range args {
		sum += n.(int64)
	}
	return int63(sum)
}

// minus is (- a b c ...), a-b-c..., and (- a), -a.
func minus(_ *Scope, args List) interface{} {
	if fs, isFloat := asFloats(args); isFloat {
		if len(fs) == 1 {
			return -fs[0]
		}
		result := fs[0]
		for _, f := range fs[1:] {
			result -= f
		}
		return result
	}
	if len(args) == 1 {
		return int63(-args[0].(int64))
	}
	result := args[0].(int64)
	for _, n := range args[1:] {
		result -= n.(int64)
	}
	return int63(result)
}

// times is (* a b c ...), the product; (*) is 1.
func times(_ *Scope, args List) interface{} {
	if fs, isFloat := asFloats(args); isFloat {
		product := 1.0
		for _, f := range fs {
			product *= f
		}
		return product
	}
	var product int64 = 1
	for _, n := range args {
		product *= n.(int64)
	}
	return int63(product)
}

// divide returns a builtin (op a b) that divides two integers with div, as
// Go does: / truncates toward zero, and mod's result has a's sign. Floats,
// which only / takes, divide as floats.
func divide(op string, div func(a, b int64) int64) Function {
	return func(_ *Scope, args List) interface{} {
		if fs, isFloat := asFloats(args); isFloat {
			return fs[0] / fs[1]
		}
		a, b := args[0].(int64), args[1].(int64)
		if b == 0 {
			panic(op + ": division by zero")
		}
		return int63(div(a, b))
	}
}

// isType returns the builtin name, (name x), which reports whether x's
// type is one of typs, as typeName names them.
func isType(typs []string) Function {
	return func(_ *Scope, args List) interface{} {
		return slices.Contains(typs, typeName(args[0]))
	}
}

// compare returns a builtin (op a b) that compares two integers with ints,
// or, if either is a float, two floats with floats.
func compare(ints func(a, b int64) bool, floats func(a, b float64) bool) Function {
	return func(_ *Scope, args List) interface{} {
		if fs, isFloat := asFloats(args); isFloat {
			return floats(fs[0], fs[1])
		}
		return ints(args[0].(int64), args[1].(int64))
	}
}

func eq(_ *Scope, args List) interface{} {
	return equal(args[0], args[1])
}

// equal reports whether two glisp values are equal: lists and vectors element by element, maps entry by entry,
// data by constructor and fields,
// other values with ==. Values whose type can't be compared with ==, such as
// functions, are never equal.
func equal(a, b interface{}) bool {
	if isList(a) {
		if !isList(b) {
			return false
		}
		// Element by element, without copying a Pair into a List: (eq l
		// (quote ())) is how a loop over a list ends.
		for {
			fa, ra, moreA := uncons(a)
			fb, rb, moreB := uncons(b)
			if !moreA || !moreB {
				return moreA == moreB
			}
			if !equal(fa, fb) {
				return false
			}
			a, b = ra, rb
		}
	}
	if ma, isMap := a.(Map); isMap {
		mb, isMap := b.(Map)
		return isMap && equalMaps(ma, mb)
	}
	if va, isVector := a.(Vector); isVector {
		vb, isVector := b.(Vector)
		return isVector && equal(List(va), List(vb))
	}
	if da, isData := a.(Data); isData {
		db, isData := b.(Data)
		return isData && da.name == db.name && equal(List(da.fields), List(db.fields))
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
	last := args[len(args)-1]
	checkKind("apply", listKind, last)
	list, _ := asList(last)
	spread := append(append(List{}, args[1:len(args)-1]...), list...)
	return fn(scope, spread)
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

// raise is (error x ...): it fails with its arguments as str joins them.
func raise(scope *Scope, args List) interface{} {
	panic(showJoined(scope, args).(string))
}

var builtins = builtinTable()

//go:embed stdlib.glisp
var stdlib string

var builtinScope = &Scope{table: builtins}

// stdlibForms is stdlib.glisp, read once.
var stdlibForms = Read(TokenizeString(stdlib)).(List)

// stdlibExpanded is the stdlib expanded once, in a scope of its own: its
// nodes, and the macros it defines. A run evaluates the nodes in its own
// scope; the macros, closures over the stdlib's scope, expand the run's
// forms, so a program's definitions don't change what a stdlib macro does.
var stdlibExpanded = sync.OnceValues(func() ([]Node, map[string]Macro) {
	return expandLibrary(stdlibForms)
})

// expandLibrary expands and evaluates the forms of a library, the stdlib, and
// returns its nodes and macros.
func expandLibrary(forms List) ([]Node, map[string]Macro) {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}}
	expander := NewExpander(scope)
	var nodes []Node
	defined := map[string]bool{}
	for _, form := range forms {
		expanded := expander.ExpandTop(form)
		if head, _ := listHead(form); head != "definstance" {
			for _, name := range definedNames(form, expanded) {
				if _, isBuiltin := builtinScope.lookup(symbol(name)); isBuiltin {
					panic(fmt.Sprintf("stdlib: %s is a builtin; name it something else", name))
				}
				if defined[name] {
					panic(fmt.Sprintf("stdlib: %s is defined twice; name it something else, or add to a generic with definstance", name))
				}
				defined[name] = true
			}
		}
		for _, node := range expanded {
			Eval(node, scope)
			nodes = append(nodes, node)
		}
	}
	macros := map[string]Macro{}
	for name, value := range scope.table {
		if macro, isMacro := value.(Macro); isMacro {
			macros[name] = macro
		}
	}
	return nodes, macros
}

// listHead is the name a list form starts with, if it starts with a symbol.
func listHead(form interface{}) (string, bool) {
	if l, isList := form.(List); isList && len(l) > 0 {
		if head, isSymbol := l[0].(Symbol); isSymbol {
			return head.Str(), true
		}
	}
	return "", false
}

// definedNames are the names a top-level form defines: the defs it expanded
// to, or the macro it defines.
func definedNames(form interface{}, expanded []Node) []string {
	if head, _ := listHead(form); head == "defmacro" {
		if name, isSymbol := form.(List)[1].(Symbol); isSymbol {
			return []string{name.Str()}
		}
	}
	var names []string
	for _, node := range expanded {
		if def, isDef := node.(Def); isDef {
			names = append(names, def.Name)
		}
	}
	return names
}

// newScope makes the top-level scope for one run: its own definitions over
// the builtins, which it never changes, with the stdlib loaded: its macros,
// and its definitions, evaluated from the nodes expanded once.
func newScope() *Scope {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}, run: &runState{}}
	nodes, macros := stdlibExpanded()
	for name, macro := range macros {
		scope.add(name, macro)
	}
	for _, node := range nodes {
		Eval(node, scope)
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
		nodes := expander.ExpandTop(form)
		if scope.run != nil {
			scope.run.line = expander.formLine(form)
		}
		for _, node := range nodes {
			value = Eval(node, scope)
		}
	}
	return withoutPairs(value)
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
