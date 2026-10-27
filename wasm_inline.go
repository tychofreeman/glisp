package glisp

import "github.com/tychofreeman/glisp/wasm"

// Direct calls to builtins. When a call names a builtin and passes the
// arguments its inline path takes, the compiled call first checks that the
// name still holds that builtin (a program may redefine it), and if so runs
// the operation inline instead of going through invoke. Whatever the inline
// path doesn't handle, such as an argument of the wrong type, falls back to
// the ordinary call, so results and errors are the builtin's own.

// inlineBuiltin is a builtin's inline path, for calls with argc arguments.
// emit leaves the call's value, reading argument i with arg and making the
// ordinary call with slow.
type inlineBuiltin struct {
	argc int
	emit func(b *wasmBody, rt *wasmRuntime, arg func(i int), slow func())
}

// ifIntegers runs then if both arguments are integers, and slow if not.
func (b *wasmBody) ifIntegers(arg func(i int), then, slow func()) {
	arg(0)
	arg(1)
	b.Op(wasm.I64And)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I32WrapI64)
	b.ifElse(wasm.BlockI64, func() {
		then()
	}, func() {
		slow()
	})
}

// arithmetic is an inline path for two integers: ops applied to the two
// encoded values (2a+1 and 2b+1).
func arithmetic(ops func(b *wasmBody)) inlineBuiltin {
	return inlineBuiltin{2, func(b *wasmBody, _ *wasmRuntime, arg func(int), slow func()) {
		b.ifIntegers(arg, func() {
			arg(0)
			arg(1)
			ops(b)
		}, slow)
	}}
}

// comparison compares two integers: encoding keeps their order.
func comparison(op byte) inlineBuiltin {
	return arithmetic(func(b *wasmBody) {
		b.Op(op)
		b.boolean()
	})
}

// ifTag runs then if the tag of argument 0 satisfies test, and slow if not.
func (b *wasmBody) ifTag(rt *wasmRuntime, arg func(int), test func(), then, slow func()) {
	arg(0)
	b.Call(rt.tagOf)
	test()
	b.ifElse(wasm.BlockI64, func() {
		then()
	}, func() {
		slow()
	})
}

// pairField is car or cdr of a pair, inline: the field at offset.
func pairField(offset uint32) inlineBuiltin {
	return inlineBuiltin{1, func(b *wasmBody, rt *wasmRuntime, arg func(int), slow func()) {
		b.ifTag(rt, arg, func() {
			b.I32Const(tagPair)
			b.Op(wasm.I32Eq)
		}, func() {
			arg(0)
			b.Op(wasm.I32WrapI64)
			b.I64Load(offset)
		}, slow)
	}}
}

// predicate is a one-argument test that never fails: test leaves an i32.
func predicate(test func(b *wasmBody, rt *wasmRuntime, arg func(int))) inlineBuiltin {
	return inlineBuiltin{1, func(b *wasmBody, rt *wasmRuntime, arg func(int), _ func()) {
		test(b, rt, arg)
		b.boolean()
	}}
}

var inlineBuiltins = map[string]inlineBuiltin{
	"plus": arithmetic(func(b *wasmBody) {
		b.Op(wasm.I64Add)
		b.I64Const(1)
		b.Op(wasm.I64Sub)
	}),
	"-": arithmetic(func(b *wasmBody) {
		b.Op(wasm.I64Sub)
		b.I64Const(1)
		b.Op(wasm.I64Add)
	}),
	"*": inlineBuiltin{2, func(b *wasmBody, _ *wasmRuntime, arg func(int), slow func()) {
		b.ifIntegers(arg, func() {
			for i := 0; i < 2; i++ {
				arg(i)
				b.I64Const(1)
				b.Op(wasm.I64ShrS)
			}
			b.Op(wasm.I64Mul)
			b.encode()
		}, slow)
	}},
	"<":   comparison(wasm.I64LtS),
	">":   comparison(wasm.I64GtS),
	"<=":  comparison(wasm.I64LeS),
	">=":  comparison(wasm.I64GeS),
	"car": pairField(8),
	"cdr": pairField(16),
	"eq": inlineBuiltin{2, func(b *wasmBody, rt *wasmRuntime, arg func(int), _ func()) {
		arg(0)
		arg(1)
		b.Call(rt.equal)
		b.boolean()
	}},
	"cons": inlineBuiltin{2, func(b *wasmBody, rt *wasmRuntime, arg func(int), slow func()) {
		// Onto a list: a new pair. Onto nil or an atom, the builtin's rules.
		arg(1)
		b.Call(rt.tagOf)
		b.isList()
		b.ifElse(wasm.BlockI64, func() {
			arg(0)
			arg(1)
			b.Call(rt.pair)
		}, func() {
			slow()
		})
	}},
	"atom": predicate(func(b *wasmBody, rt *wasmRuntime, arg func(int)) {
		arg(0)
		b.Call(rt.tagOf)
		b.isList()
		b.Op(wasm.I32Eqz)
	}),
	"list?": predicate(func(b *wasmBody, rt *wasmRuntime, arg func(int)) {
		arg(0)
		b.Call(rt.tagOf)
		b.isList()
	}),
	"number?": predicate(func(b *wasmBody, _ *wasmRuntime, arg func(int)) {
		arg(0)
		b.I64Const(1)
		b.Op(wasm.I64And, wasm.I32WrapI64)
	}),
	"nil?": predicate(func(b *wasmBody, _ *wasmRuntime, arg func(int)) {
		arg(0)
		b.I64Const(addrNil)
		b.Op(wasm.I64Eq)
	}),
}

// inlineCalls turns direct calls to builtins on; tests turn it off to
// compare.
var inlineCalls = true

// inlineFor is the builtin a call names, and its inline path, if the call
// passes the arguments the path takes.
func (c *wasmCompiler) inlineFor(n Apply) (string, inlineBuiltin, bool) {
	ref, isRef := n.Fn.(Ref)
	if !inlineCalls || !isRef || ref.Loc != nil {
		return "", inlineBuiltin{}, false
	}
	inline, found := inlineBuiltins[ref.Name]
	if !found || inline.argc != len(n.Args) {
		return "", inlineBuiltin{}, false
	}
	return ref.Name, inline, true
}
