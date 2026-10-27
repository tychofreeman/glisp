package glisp

import "fmt"

// A builtin is a function every backend provides. Its spec says what
// arguments it takes; each backend checks them from the spec, with the same
// messages, before the builtin's operation runs.
type builtin struct {
	name     string
	min, max int    // how many arguments it takes; max is -1 for no limit
	kinds    []kind // the kind of each argument; the last stands for the rest
	run      Function
}

// A kind is a kind of value an argument must be.
type kind int

const (
	anyKind kind = iota
	numberKind
	stringKind
	listKind // a list, or nil as the empty list
	functionKind
)

// noun names k in a message: "expected a number".
func (k kind) noun() string {
	return [...]string{"anything", "a number", "a string", "a list", "a function"}[k]
}

// has reports whether v is of kind k.
func (k kind) has(v interface{}) bool {
	switch k {
	case numberKind:
		_, is := v.(int64)
		return is
	case stringKind:
		_, is := v.(string)
		return is
	case listKind:
		_, is := v.(List)
		return is || v == nil
	case functionKind:
		_, is := asFunction(v)
		return is
	}
	return true
}

// kind is the kind of argument i.
func (b builtin) kind(i int) kind {
	if len(b.kinds) == 0 {
		return anyKind
	}
	return b.kinds[min(i, len(b.kinds)-1)]
}

// expects says how many arguments b takes: "expects 2 arguments".
func (b builtin) expects() string {
	switch {
	case b.max < 0:
		return "expects at least " + arguments(b.min)
	case b.min == b.max:
		return "expects " + arguments(b.min)
	case b.max == b.min+1:
		return fmt.Sprintf("expects %d or %s", b.min, arguments(b.max))
	}
	return fmt.Sprintf("expects %d to %s", b.min, arguments(b.max))
}

// arguments is "1 argument" or "n arguments".
func arguments(n int) string {
	if n == 1 {
		return "1 argument"
	}
	return fmt.Sprintf("%d arguments", n)
}

// check panics unless args are what b takes.
func (b builtin) check(args List) {
	if len(args) < b.min || b.max >= 0 && len(args) > b.max {
		panic(fmt.Sprintf("%s: %s, got %d", b.name, b.expects(), len(args)))
	}
	for i, arg := range args {
		if k := b.kind(i); !k.has(arg) {
			panic(fmt.Sprintf("%s: expected %s, got %s", b.name, k.noun(), described(arg)))
		}
	}
}

// function is b as the interpreter calls it: its arguments checked, then
// its operation.
func (b builtin) function() Function {
	return func(scope *Scope, args List) interface{} {
		b.check(args)
		return b.run(scope, args)
	}
}

var (
	anyOne    = []kind{anyKind}
	numbers   = []kind{numberKind}
	oneString = []kind{stringKind}
)

// builtinSpecs are the builtin functions, in the order the wasm backend
// numbers them.
var builtinSpecs = []builtin{
	{"car", 1, 1, []kind{listKind}, car},
	{"cdr", 1, 1, []kind{listKind}, cdr},
	{"atom", 1, 1, anyOne, atom},
	{"cons", 1, 2, nil, cons},
	{"plus", 0, -1, numbers, plus},
	{"-", 1, -1, numbers, minus},
	{"*", 0, -1, numbers, times},
	{"/", 2, 2, numbers, divide("/", func(a, b int64) int64 { return a / b })},
	{"mod", 2, 2, numbers, divide("mod", func(a, b int64) int64 { return a % b })},
	{"<", 2, 2, numbers, compare(func(a, b int64) bool { return a < b })},
	{">", 2, 2, numbers, compare(func(a, b int64) bool { return a > b })},
	{"<=", 2, 2, numbers, compare(func(a, b int64) bool { return a <= b })},
	{">=", 2, 2, numbers, compare(func(a, b int64) bool { return a >= b })},
	{"number?", 1, 1, anyOne, isType("number")},
	{"string?", 1, 1, anyOne, isType("string")},
	{"symbol?", 1, 1, anyOne, isType("symbol")},
	{"list?", 1, 1, anyOne, isType("list")},
	{"function?", 1, 1, anyOne, isType("function")},
	{"boolean?", 1, 1, anyOne, isType("bool")},
	{"nil?", 1, 1, anyOne, isType("nil")},
	{"eq", 2, 2, nil, eq},
	{"apply", 1, -1, []kind{functionKind, anyKind}, apply},
	{"p", 0, -1, nil, printValues},
	{"str", 0, -1, nil, showJoined},
	{"string_length", 1, 1, oneString, stringLength},
	{"substring", 3, 3, []kind{stringKind, numberKind, numberKind}, substring},
	{"chars", 1, 1, oneString, chars},
	{"string<", 2, 2, oneString, stringLess},
	{"symbol", 1, 1, oneString, toSymbol},
	{"parse_number", 1, 1, oneString, parseNumber},
}

// builtinTable is the builtins as the interpreter's top scope holds them,
// with the constants.
func builtinTable() map[string]interface{} {
	table := map[string]interface{}{"true": true, "false": false, "nil": nil}
	for _, b := range builtinSpecs {
		table[b.name] = b.function()
	}
	return table
}
