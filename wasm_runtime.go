package glisp

import "github.com/tychofreeman/glisp/wasm"

// wasmRuntime holds the function indices of the runtime that compiled glisp
// calls: memory, printing, errors, calls and the builtins.
type wasmRuntime struct {
	fdWrite, procExit                               uint32 // WASI imports
	alloc, flush, putByte, putBytes, putStr, putInt uint32
	putValue, putType, putArgs, failBegin, failEnd  uint32
	tagOf, num, listArg, pair, cell, equal, invoke  uint32
	putShow, putQuoted, putDescribed                uint32
	builtins                                        map[string]uint32 // table index of each builtin
	strArg, runeCount, runeOffset, newString        uint32            // string helpers
	objSize, gcCopy, gcReloc, gc                    uint32            // the garbage collector
}

var (
	i32 = wasm.I32
	i64 = wasm.I64
)

func sig(params []wasm.ValType, results ...wasm.ValType) wasm.FuncType {
	return wasm.FuncType{Params: params, Results: results}
}

func vals(types ...wasm.ValType) []wasm.ValType { return types }

// runtime declares the runtime's functions, then defines them.
func (c *wasmCompiler) runtime() {
	m := c.m
	m.Imports = []wasm.Import{
		{Module: "wasi_snapshot_preview1", Name: "fd_write", Type: m.Type(sig(vals(i32, i32, i32, i32), i32))},
		{Module: "wasi_snapshot_preview1", Name: "proc_exit", Type: m.Type(sig(vals(i32)))},
	}
	rt := &c.rt
	rt.fdWrite, rt.procExit = 0, 1
	rt.alloc = c.declare(sig(vals(i32), i32))
	rt.flush = c.declare(sig(nil))
	rt.putByte = c.declare(sig(vals(i32)))
	rt.putBytes = c.declare(sig(vals(i32, i32)))
	rt.putStr = c.declare(sig(vals(i32)))
	rt.putInt = c.declare(sig(vals(i64)))
	rt.putValue = c.declare(sig(vals(i64)))
	rt.putType = c.declare(sig(vals(i64)))
	rt.putArgs = c.declare(sig(vals(i32, i32)))
	rt.putShow = c.declare(sig(vals(i64)))
	rt.putQuoted = c.declare(sig(vals(i32)))
	rt.putDescribed = c.declare(sig(vals(i64)))
	rt.failBegin = c.declare(sig(nil))
	rt.failEnd = c.declare(sig(nil))
	rt.tagOf = c.declare(sig(vals(i64), i32))
	rt.num = c.declare(sig(vals(i64, i32), i64))
	rt.listArg = c.declare(sig(vals(i64, i32), i64))
	rt.pair = c.declare(sig(vals(i64, i64), i64))
	rt.cell = c.declare(sig(vals(i64), i64))
	rt.equal = c.declare(sig(vals(i64, i64), i32))
	rt.invoke = c.declare(sig(vals(i64, i32, i32, i32), i64))
	c.declareStrings()
	c.declareGC()
	builtinFuncs := map[string]uint32{}
	rt.builtins = map[string]uint32{}
	for _, name := range builtinNames {
		builtinFuncs[name], rt.builtins[name] = c.declareFn()
	}

	c.define(rt.alloc, vals(i32, i32, i32), c.allocBody())
	c.define(rt.flush, nil, c.flushBody())
	c.define(rt.putByte, nil, c.putByteBody())
	c.define(rt.putBytes, vals(i32), c.putBytesBody())
	c.define(rt.putStr, nil, c.putStrBody())
	c.define(rt.putInt, vals(i32), c.putIntBody())
	c.define(rt.putValue, vals(i32, i32), c.putValueBody())
	c.define(rt.putType, vals(i32), c.putTypeBody())
	c.define(rt.putArgs, vals(i32), c.putArgsBody())
	c.define(rt.putShow, nil, c.putShowBody())
	c.define(rt.putQuoted, vals(i32, i32, i32), c.putQuotedBody())
	c.define(rt.putDescribed, nil, c.putDescribedBody())
	c.define(rt.failBegin, nil, c.failBeginBody())
	c.define(rt.failEnd, nil, c.failEndBody())
	c.define(rt.tagOf, nil, c.tagOfBody())
	c.define(rt.num, nil, c.numBody())
	c.define(rt.listArg, nil, c.listArgBody())
	c.define(rt.pair, vals(i32), c.pairBody())
	c.define(rt.cell, vals(i32), c.cellBody())
	c.define(rt.equal, vals(i32, i32, i32, i32), c.equalBody())
	c.define(rt.invoke, vals(i64, i32, i32), c.invokeBody())
	c.defineStrings()
	for _, name := range builtinNames {
		locals, body := c.builtinBody(name)
		c.define(builtinFuncs[name], locals, body)
	}
}

// Small sequences used throughout.

// isList leaves whether the tag on the stack is a list's: empty or a pair.
func isList(b *wasm.Code) {
	b.I32Const(tagEmpty)
	b.Op(wasm.I32Sub)
	b.I32Const(1)
	b.Op(wasm.I32LeU)
}

// tagIs leaves whether value local v has tag t.
func (c *wasmCompiler) tagIs(b *wasm.Code, v uint32, t int32) {
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.I32Const(t)
	b.Op(wasm.I32Eq)
}

// field loads the i64 at offset in the object whose value is local v.
func field(b *wasm.Code, v uint32, offset uint32) {
	b.LocalGet(v)
	b.Op(wasm.I32WrapI64)
	b.I64Load(offset)
}

// arg loads argument i of a glisp function (args is local 1).
func arg(b *wasm.Code, i uint32) {
	b.LocalGet(1)
	b.I64Load(8 * i)
}

// argAt loads the argument whose index is in local i.
func argAt(b *wasm.Code, i uint32) {
	b.LocalGet(1)
	b.LocalGet(i)
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
	b.I64Load(0)
}

func incr(b *wasm.Code, i uint32) {
	b.LocalGet(i)
	b.I32Const(1)
	b.Op(wasm.I32Add)
	b.LocalSet(i)
}

// boolean turns the i32 condition on the stack into true or false.
func boolean(b *wasm.Code) {
	b.If(wasm.BlockI64)
	b.I64Const(addrTrue)
	b.Op(wasm.Else)
	b.I64Const(addrFalse)
	b.Op(wasm.End)
}

// encode turns the integer on the stack into a value.
func encode(b *wasm.Code) {
	b.I64Const(1)
	b.Op(wasm.I64Shl)
	b.I64Const(1)
	b.Op(wasm.I64Or)
}

// returnIf returns from the function if the i32 on the stack is true,
// leaving what then leaves.
func returnIf(b *wasm.Code, then func()) {
	b.If(wasm.BlockVoid)
	then()
	b.Op(wasm.Return, wasm.End)
}

// typeAndValue prints the type and value in local v, as an error does.
func (c *wasmCompiler) typeAndValue(b *wasm.Code, v uint32) {
	b.LocalGet(v)
	b.Call(c.rt.putDescribed)
}

// alloc(size) returns size bytes of heap, doubling memory when it runs out.
func (c *wasmCompiler) allocBody() wasm.Code {
	const size, p, end, need = 0, 1, 2, 3
	var b wasm.Code
	b.GlobalGet(gHP)
	b.LocalTee(p)
	b.LocalGet(size)
	b.Op(wasm.I32Add)
	b.I32Const(7)
	b.Op(wasm.I32Add)
	b.I32Const(-8)
	b.Op(wasm.I32And)
	b.LocalTee(end)
	b.MemorySize()
	b.I32Const(16)
	b.Op(wasm.I32Shl, wasm.I32GtU)
	b.If(wasm.BlockVoid)
	// need: the pages end is past memory.
	b.LocalGet(end)
	b.MemorySize()
	b.I32Const(16)
	b.Op(wasm.I32Shl, wasm.I32Sub)
	b.I32Const(65535)
	b.Op(wasm.I32Add)
	b.I32Const(16)
	b.Op(wasm.I32ShrU)
	b.LocalSet(need)
	// Grow by the larger of need and the memory's size, doubling it, so a
	// program that allocates a lot grows memory a few times, not once per
	// allocation. If that much isn't available, grow by need alone.
	b.MemorySize()
	b.LocalGet(need)
	b.MemorySize()
	b.LocalGet(need)
	b.Op(wasm.I32GtU, wasm.Select)
	b.MemoryGrow()
	b.I32Const(-1)
	b.Op(wasm.I32Eq)
	b.If(wasm.BlockVoid)
	b.LocalGet(need)
	b.MemoryGrow()
	b.I32Const(-1)
	b.Op(wasm.I32Eq)
	b.If(wasm.BlockVoid)
	c.fail(&b, "out of memory", nil)
	b.Op(wasm.End, wasm.End, wasm.End)
	b.LocalGet(end)
	b.GlobalSet(gHP)
	b.LocalGet(p)
	return b
}

// flush writes the output buffer to the current fd.
func (c *wasmCompiler) flushBody() wasm.Code {
	var b wasm.Code
	b.GlobalGet(gOutLen)
	b.Op(wasm.I32Eqz)
	returnIf(&b, func() {})
	b.I32Const(addrIovec)
	b.GlobalGet(gOutBuf)
	b.I32Store(0)
	b.I32Const(addrIovec)
	b.GlobalGet(gOutLen)
	b.I32Store(4)
	b.GlobalGet(gFD)
	b.I32Const(addrIovec)
	b.I32Const(1)
	b.I32Const(addrWritten)
	b.Call(c.rt.fdWrite)
	b.Op(wasm.Drop)
	b.I32Const(0)
	b.GlobalSet(gOutLen)
	return b
}

func (c *wasmCompiler) putByteBody() wasm.Code {
	var b wasm.Code
	// While str captures, the byte goes on the end of the string it builds,
	// growing memory a page at a time.
	b.GlobalGet(gCapture)
	b.If(wasm.BlockVoid)
	b.GlobalGet(gCapEnd)
	b.MemorySize()
	b.I32Const(16)
	b.Op(wasm.I32Shl, wasm.I32GeU)
	b.If(wasm.BlockVoid)
	b.I32Const(1)
	b.MemoryGrow()
	b.I32Const(-1)
	b.Op(wasm.I32Eq)
	b.If(wasm.BlockVoid)
	b.I32Const(0)
	b.GlobalSet(gCapture)
	c.fail(&b, "out of memory", nil)
	b.Op(wasm.End, wasm.End)
	b.GlobalGet(gCapEnd)
	b.LocalGet(0)
	b.I32Store8(0)
	b.GlobalGet(gCapEnd)
	b.I32Const(1)
	b.Op(wasm.I32Add)
	b.GlobalSet(gCapEnd)
	b.Op(wasm.Return, wasm.End)
	b.GlobalGet(gOutBuf)
	b.GlobalGet(gOutLen)
	b.Op(wasm.I32Add)
	b.LocalGet(0)
	b.I32Store8(0)
	b.GlobalGet(gOutLen)
	b.I32Const(1)
	b.Op(wasm.I32Add)
	b.GlobalSet(gOutLen)
	b.GlobalGet(gOutLen)
	b.I32Const(outCap)
	b.Op(wasm.I32Eq)
	b.If(wasm.BlockVoid)
	b.Call(c.rt.flush)
	b.Op(wasm.End)
	return b
}

// putBytes(ptr, n) prints n bytes from ptr.
func (c *wasmCompiler) putBytesBody() wasm.Code {
	const ptr, n, i = 0, 1, 2
	var b wasm.Code
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(n)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	b.LocalGet(ptr)
	b.LocalGet(i)
	b.Op(wasm.I32Add)
	b.I32Load8U(0)
	b.Call(c.rt.putByte)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	return b
}

// putStr(s) prints the string object s.
func (c *wasmCompiler) putStrBody() wasm.Code {
	var b wasm.Code
	b.LocalGet(0)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(0)
	b.I32Load(4)
	b.Call(c.rt.putBytes)
	return b
}

// putInt(n) prints n in decimal.
func (c *wasmCompiler) putIntBody() wasm.Code {
	const n, p = 0, 1
	const end = addrDigits + 24
	var b wasm.Code
	b.LocalGet(n)
	b.I64Const(0)
	b.Op(wasm.I64LtS)
	b.If(wasm.BlockVoid)
	b.I32Const('-')
	b.Call(c.rt.putByte)
	b.I64Const(0)
	b.LocalGet(n)
	b.Op(wasm.I64Sub)
	b.LocalSet(n)
	b.Op(wasm.End)
	b.I32Const(end)
	b.LocalSet(p)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(p)
	b.I32Const(1)
	b.Op(wasm.I32Sub)
	b.LocalTee(p)
	b.LocalGet(n)
	b.I64Const(10)
	b.Op(wasm.I64RemU, wasm.I32WrapI64)
	b.I32Const('0')
	b.Op(wasm.I32Add)
	b.I32Store8(0)
	b.LocalGet(n)
	b.I64Const(10)
	b.Op(wasm.I64DivU)
	b.LocalTee(n)
	b.Op(wasm.I64Eqz, wasm.I32Eqz)
	b.BrIf(0)
	b.Op(wasm.End)
	b.LocalGet(p)
	b.I32Const(end)
	b.LocalGet(p)
	b.Op(wasm.I32Sub)
	b.Call(c.rt.putBytes)
	return b
}

// putValue(v) prints v as Write does.
func (c *wasmCompiler) putValueBody() wasm.Code {
	const v, t, first = 0, 1, 2
	var b wasm.Code
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.LocalSet(t)
	when := func(tag int32, then func()) {
		b.LocalGet(t)
		b.I32Const(tag)
		b.Op(wasm.I32Eq)
		returnIf(&b, then)
	}
	when(tagInt, func() {
		b.LocalGet(v)
		b.I64Const(1)
		b.Op(wasm.I64ShrS)
		b.Call(c.rt.putInt)
	})
	when(tagNil, func() { c.text(&b, "nil") })
	when(tagTrue, func() { c.text(&b, "true") })
	when(tagFalse, func() { c.text(&b, "false") })
	when(tagString, func() {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.Call(c.rt.putQuoted)
	})
	when(tagSymbol, func() {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.Call(c.rt.putStr)
	})
	when(tagClosure, func() { c.text(&b, "<function>") })
	// A list: (a b c).
	b.I32Const('(')
	b.Call(c.rt.putByte)
	b.I32Const(1)
	b.LocalSet(first)
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(v)
	b.I64Const(addrEmpty)
	b.Op(wasm.I64Eq)
	b.BrIf(1)
	b.LocalGet(first)
	b.Op(wasm.I32Eqz)
	b.If(wasm.BlockVoid)
	b.I32Const(' ')
	b.Call(c.rt.putByte)
	b.Op(wasm.End)
	b.I32Const(0)
	b.LocalSet(first)
	field(&b, v, 8)
	b.Call(c.rt.putValue)
	field(&b, v, 16)
	b.LocalSet(v)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.I32Const(')')
	b.Call(c.rt.putByte)
	return b
}

// putType(v) prints the glisp name of v's type, as typeName does.
func (c *wasmCompiler) putTypeBody() wasm.Code {
	const v, t = 0, 1
	var b wasm.Code
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.LocalSet(t)
	names := []struct {
		tag  int32
		name string
	}{
		{tagInt, "number"}, {tagNil, "nil"}, {tagTrue, "bool"}, {tagFalse, "bool"},
		{tagString, "string"}, {tagSymbol, "symbol"}, {tagClosure, "function"},
	}
	for _, n := range names {
		b.LocalGet(t)
		b.I32Const(n.tag)
		b.Op(wasm.I32Eq)
		returnIf(&b, func() { c.text(&b, n.name) })
	}
	c.text(&b, "list")
	return b
}

// putArgs(args, argc) prints the arguments as a list: (a b c).
func (c *wasmCompiler) putArgsBody() wasm.Code {
	const args, argc, i = 0, 1, 2
	var b wasm.Code
	b.I32Const('(')
	b.Call(c.rt.putByte)
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(argc)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	b.LocalGet(i)
	b.If(wasm.BlockVoid)
	b.I32Const(' ')
	b.Call(c.rt.putByte)
	b.Op(wasm.End)
	b.LocalGet(args)
	b.LocalGet(i)
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
	b.I64Load(0)
	b.Call(c.rt.putValue)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.I32Const(')')
	b.Call(c.rt.putByte)
	return b
}

// putShow(v) prints v as Show does: a string as its text, anything else as
// putValue writes it.
func (c *wasmCompiler) putShowBody() wasm.Code {
	var b wasm.Code
	c.tagIs(&b, 0, tagString)
	returnIf(&b, func() {
		b.LocalGet(0)
		b.Op(wasm.I32WrapI64)
		b.Call(c.rt.putStr)
	})
	b.LocalGet(0)
	b.Call(c.rt.putValue)
	return b
}

// putQuoted(s) prints the string object s in quotes, escaping as quote does.
func (c *wasmCompiler) putQuotedBody() wasm.Code {
	const s, i, n, ch = 0, 1, 2, 3
	var b wasm.Code
	put := func(bytes ...int32) {
		for _, x := range bytes {
			b.I32Const(x)
			b.Call(c.rt.putByte)
		}
	}
	is := func(x int32) {
		b.LocalGet(ch)
		b.I32Const(x)
		b.Op(wasm.I32Eq)
	}
	put('"')
	b.LocalGet(s)
	b.I32Load(4)
	b.LocalSet(n)
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(n)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	b.LocalGet(s)
	b.LocalGet(i)
	b.Op(wasm.I32Add)
	b.I32Load8U(8)
	b.LocalSet(ch)
	is('\n')
	b.If(wasm.BlockVoid)
	put('\\', 'n')
	b.Op(wasm.Else)
	is('\t')
	b.If(wasm.BlockVoid)
	put('\\', 't')
	b.Op(wasm.Else)
	is('"')
	is('\\')
	b.Op(wasm.I32Or)
	b.If(wasm.BlockVoid)
	put('\\')
	b.Op(wasm.End)
	b.LocalGet(ch)
	b.Call(c.rt.putByte)
	b.Op(wasm.End, wasm.End)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	put('"')
	return b
}

// putDescribed(v) prints v's type and value, as described does.
func (c *wasmCompiler) putDescribedBody() wasm.Code {
	var b wasm.Code
	b.LocalGet(0)
	b.I64Const(addrNil)
	b.Op(wasm.I64Eq)
	returnIf(&b, func() { c.text(&b, "nil") })
	b.LocalGet(0)
	b.Call(c.rt.putType)
	b.I32Const(' ')
	b.Call(c.rt.putByte)
	b.LocalGet(0)
	b.Call(c.rt.putValue)
	return b
}

// failBegin flushes the program's output and starts an error message on
// stderr; failEnd ends it and exits with status 1.
func (c *wasmCompiler) failBeginBody() wasm.Code {
	var b wasm.Code
	b.Call(c.rt.flush)
	b.I32Const(2)
	b.GlobalSet(gFD)
	c.text(&b, "glisp: ")
	b.GlobalGet(gLine)
	b.If(wasm.BlockVoid)
	c.text(&b, "line ")
	b.GlobalGet(gLine)
	b.Op(wasm.I64ExtendI32U)
	b.Call(c.rt.putInt)
	c.text(&b, ": ")
	b.Op(wasm.End)
	return b
}

func (c *wasmCompiler) failEndBody() wasm.Code {
	var b wasm.Code
	b.I32Const('\n')
	b.Call(c.rt.putByte)
	b.Call(c.rt.flush)
	b.I32Const(1)
	b.Call(c.rt.procExit)
	return b
}

// tagOf(v) is v's tag; tagInt for an integer.
func (c *wasmCompiler) tagOfBody() wasm.Code {
	var b wasm.Code
	b.LocalGet(0)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I32WrapI64)
	b.If(wasm.BlockI32)
	b.I32Const(tagInt)
	b.Op(wasm.Else)
	b.LocalGet(0)
	b.Op(wasm.I32WrapI64)
	b.I32Load(0)
	b.Op(wasm.End)
	return b
}

// num(v, fn) is the integer v holds, or an error naming the builtin fn.
func (c *wasmCompiler) numBody() wasm.Code {
	const v, fn = 0, 1
	var b wasm.Code
	b.LocalGet(v)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I64Eqz)
	b.If(wasm.BlockVoid)
	b.Call(c.rt.failBegin)
	b.LocalGet(fn)
	b.Call(c.rt.putStr)
	c.text(&b, ": expected a number, got ")
	c.typeAndValue(&b, v)
	b.Call(c.rt.failEnd)
	b.Op(wasm.Unreachable, wasm.End)
	b.LocalGet(v)
	b.I64Const(1)
	b.Op(wasm.I64ShrS)
	return b
}

// listArg(v, fn) is the list v, the empty list for nil, or an error naming
// the builtin fn: the interpreter's listParam.
func (c *wasmCompiler) listArgBody() wasm.Code {
	const v, fn = 0, 1
	var b wasm.Code
	b.LocalGet(v)
	b.I64Const(addrNil)
	b.Op(wasm.I64Eq)
	returnIf(&b, func() { b.I64Const(addrEmpty) })
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	isList(&b)
	returnIf(&b, func() { b.LocalGet(v) })
	b.Call(c.rt.failBegin)
	b.LocalGet(fn)
	b.Call(c.rt.putStr)
	c.text(&b, ": expected a list, got ")
	c.typeAndValue(&b, v)
	b.Call(c.rt.failEnd)
	b.Op(wasm.Unreachable)
	return b
}

// pair(car, cdr) makes a list cell.
func (c *wasmCompiler) pairBody() wasm.Code {
	const car, cdr, p = 0, 1, 2
	var b wasm.Code
	b.I32Const(24)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.I32Const(tagPair)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(car)
	b.I64Store(8)
	b.LocalGet(p)
	b.LocalGet(cdr)
	b.I64Store(16)
	b.LocalGet(p)
	b.Op(wasm.I64ExtendI32U)
	return b
}

// cell(v) makes a cell holding v.
func (c *wasmCompiler) cellBody() wasm.Code {
	const v, p = 0, 1
	var b wasm.Code
	b.I32Const(16)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.I32Const(tagCell)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(v)
	b.I64Store(8)
	b.LocalGet(p)
	b.Op(wasm.I64ExtendI32U)
	return b
}

// equal(a, b) is the interpreter's equal: lists element by element, strings
// and symbols by their text, functions never.
func (c *wasmCompiler) equalBody() wasm.Code {
	const a, bv, ta, tb, n, i = 0, 1, 2, 3, 4, 5
	var b wasm.Code
	b.LocalGet(a)
	b.Call(c.rt.tagOf)
	b.LocalSet(ta)
	b.LocalGet(bv)
	b.Call(c.rt.tagOf)
	b.LocalSet(tb)
	no := func() { b.I32Const(0) }

	b.LocalGet(ta)
	isList(&b)
	b.If(wasm.BlockVoid)
	b.LocalGet(tb)
	isList(&b)
	b.Op(wasm.I32Eqz)
	returnIf(&b, no)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(a)
	b.I64Const(addrEmpty)
	b.Op(wasm.I64Eq)
	b.LocalGet(bv)
	b.I64Const(addrEmpty)
	b.Op(wasm.I64Eq)
	b.Op(wasm.I32Or)
	returnIf(&b, func() {
		b.LocalGet(a)
		b.LocalGet(bv)
		b.Op(wasm.I64Eq)
	})
	field(&b, a, 8)
	field(&b, bv, 8)
	b.Call(c.rt.equal)
	b.Op(wasm.I32Eqz)
	returnIf(&b, no)
	field(&b, a, 16)
	b.LocalSet(a)
	field(&b, bv, 16)
	b.LocalSet(bv)
	b.Br(0)
	b.Op(wasm.End, wasm.End)

	b.LocalGet(ta)
	b.LocalGet(tb)
	b.Op(wasm.I32Ne)
	returnIf(&b, no)
	b.LocalGet(ta)
	b.I32Const(tagInt)
	b.Op(wasm.I32Eq)
	returnIf(&b, func() {
		b.LocalGet(a)
		b.LocalGet(bv)
		b.Op(wasm.I64Eq)
	})
	b.LocalGet(ta)
	b.I32Const(tagClosure)
	b.Op(wasm.I32Eq)
	returnIf(&b, no)
	b.LocalGet(ta)
	b.I32Const(tagString)
	b.Op(wasm.I32Eq)
	b.LocalGet(ta)
	b.I32Const(tagSymbol)
	b.Op(wasm.I32Eq)
	b.Op(wasm.I32Or, wasm.I32Eqz)
	returnIf(&b, func() { b.I32Const(1) }) // nil, true, false
	// Strings and symbols: the same length and bytes.
	field32 := func(v uint32, offset uint32) {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.I32Load(offset)
	}
	field32(a, 4)
	b.LocalTee(n)
	field32(bv, 4)
	b.Op(wasm.I32Ne)
	returnIf(&b, no)
	byteAt := func(v uint32) {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.LocalGet(i)
		b.Op(wasm.I32Add)
		b.I32Load8U(8)
	}
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(n)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	byteAt(a)
	byteAt(bv)
	b.Op(wasm.I32Ne)
	returnIf(&b, no)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.I32Const(1)
	return b
}

// invoke(f, args, argc, site) calls the function f with argc arguments at
// args. site is the call, printed if f isn't a function.
func (c *wasmCompiler) invokeBody() wasm.Code {
	const f, args, argc, site, r, entry, i = 0, 1, 2, 3, 4, 5, 6
	var b wasm.Code
	b.GlobalGet(gSP)
	b.GlobalGet(gStackEnd)
	b.Op(wasm.I32GtU)
	b.If(wasm.BlockVoid)
	c.fail(&b, "stack overflow", nil)
	b.Op(wasm.End)
	c.tagIs(&b, f, tagClosure)
	b.Op(wasm.I32Eqz)
	b.If(wasm.BlockVoid)
	c.fail(&b, "Cannot call ", func() {
		c.typeAndValue(&b, f)
		c.text(&b, " in ")
		b.LocalGet(site)
		b.Call(c.rt.putStr)
	})
	b.Op(wasm.End)
	b.GlobalGet(gSP)
	b.LocalSet(entry)
	b.LocalGet(f)
	b.LocalGet(args)
	b.LocalGet(argc)
	b.LocalGet(f)
	b.Op(wasm.I32WrapI64)
	b.I32Load(4)
	b.CallIndirect(c.fnType)
	b.LocalSet(r)
	// While the function called made a tail call, make that call: move its
	// arguments down to where this invoke's stack starts, so a chain of tail
	// calls never grows the stack, and call it.
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(r)
	b.I64Const(pending)
	b.Op(wasm.I64Ne)
	b.BrIf(1)
	b.GlobalGet(gPendingFn)
	b.LocalSet(f)
	b.GlobalGet(gPendingArgc)
	b.LocalSet(argc)
	b.I32Const(0)
	b.LocalSet(i)
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(argc)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	b.LocalGet(entry)
	b.LocalGet(i)
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
	b.GlobalGet(gPendingArgs)
	b.LocalGet(i)
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
	b.I64Load(0)
	b.I64Store(0)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.LocalGet(entry)
	b.LocalSet(args)
	b.LocalGet(entry)
	b.LocalGet(argc)
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
	b.GlobalSet(gSP)
	b.GlobalGet(gSP)
	b.GlobalGet(gStackEnd)
	b.Op(wasm.I32GtU)
	b.If(wasm.BlockVoid)
	c.fail(&b, "stack overflow", nil)
	b.Op(wasm.End)
	b.LocalGet(f)
	b.LocalGet(args)
	b.LocalGet(argc)
	b.LocalGet(f)
	b.Op(wasm.I32WrapI64)
	b.I32Load(4)
	b.CallIndirect(c.fnType)
	b.LocalSet(r)
	b.LocalGet(entry)
	b.GlobalSet(gSP)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.LocalGet(r)
	return b
}

// builtinBody compiles the builtin name as a glisp function:
// (self, args, argc) -> value.
func (c *wasmCompiler) builtinBody(name string) ([]wasm.ValType, wasm.Code) {
	const argc = 2
	var b wasm.Code
	argcIs := func(n int32, op byte) {
		b.LocalGet(argc)
		b.I32Const(n)
		b.Op(op)
	}
	putArgc := func() {
		b.LocalGet(argc)
		b.Op(wasm.I64ExtendI32U)
		b.Call(c.rt.putInt)
	}
	switch name {
	case "car", "cdr":
		const v = 3
		offset := uint32(8)
		if name == "cdr" {
			offset = 16
		}
		argcIs(0, wasm.I32Eq)
		b.If(wasm.BlockI64)
		b.I64Const(addrNil)
		b.Op(wasm.Else)
		arg(&b, 0)
		b.Op(wasm.End)
		b.I32Const(int32(c.str(name)))
		b.Call(c.rt.listArg)
		b.LocalSet(v)
		c.tagIs(&b, v, tagPair)
		b.If(wasm.BlockI64)
		field(&b, v, offset)
		b.Op(wasm.Else)
		b.I64Const(addrNil)
		b.Op(wasm.End)
		return vals(i64), b
	case "atom":
		argcIs(0, wasm.I32Eq)
		returnIf(&b, func() { b.I64Const(addrFalse) })
		arg(&b, 0)
		b.Call(c.rt.tagOf)
		isList(&b)
		b.Op(wasm.I32Eqz)
		boolean(&b)
		return nil, b
	case "cons":
		const rest = 3
		argcIs(1, wasm.I32Eq)
		returnIf(&b, func() {
			arg(&b, 0)
			b.I64Const(addrEmpty)
			b.Call(c.rt.pair)
		})
		argcIs(2, wasm.I32Ne)
		b.If(wasm.BlockVoid)
		c.fail(&b, "cons: expects 1 or 2 arguments, got ", putArgc)
		b.Op(wasm.End)
		arg(&b, 1)
		b.LocalTee(rest)
		b.I64Const(addrNil)
		b.Op(wasm.I64Eq)
		returnIf(&b, func() {
			arg(&b, 0)
			b.I64Const(addrEmpty)
			b.Call(c.rt.pair)
		})
		b.LocalGet(rest)
		b.Call(c.rt.tagOf)
		isList(&b)
		returnIf(&b, func() {
			arg(&b, 0)
			b.LocalGet(rest)
			b.Call(c.rt.pair)
		})
		arg(&b, 0)
		b.LocalGet(rest)
		b.I64Const(addrEmpty)
		b.Call(c.rt.pair)
		b.Call(c.rt.pair)
		return vals(i64), b
	case "plus", "-", "*":
		const sum, i = 3, 4
		fn := int32(c.str(name))
		number := func() {
			argAt(&b, i)
			b.I32Const(fn)
			b.Call(c.rt.num)
		}
		op := wasm.I64Add
		if name == "*" {
			op = wasm.I64Mul
			b.I64Const(1)
			b.LocalSet(sum)
		}
		if name == "-" {
			op = wasm.I64Sub
			argcIs(0, wasm.I32Eq)
			b.If(wasm.BlockVoid)
			c.fail(&b, "-: expects at least 1 argument, got 0", nil)
			b.Op(wasm.End)
			number()
			b.LocalSet(sum)
			argcIs(1, wasm.I32Eq)
			returnIf(&b, func() {
				b.I64Const(0)
				b.LocalGet(sum)
				b.Op(wasm.I64Sub)
				encode(&b)
			})
			b.I32Const(1)
			b.LocalSet(i)
		}
		b.Block(wasm.BlockVoid)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(i)
		b.LocalGet(argc)
		b.Op(wasm.I32GeU)
		b.BrIf(1)
		b.LocalGet(sum)
		number()
		b.Op(op)
		b.LocalSet(sum)
		incr(&b, i)
		b.Br(0)
		b.Op(wasm.End, wasm.End)
		b.LocalGet(sum)
		encode(&b)
		return vals(i64, i32), b
	case "/", "mod":
		const a, d = 3, 4
		fn := int32(c.str(name))
		argcIs(2, wasm.I32Ne)
		b.If(wasm.BlockVoid)
		c.fail(&b, name+": expects 2 arguments, got ", putArgc)
		b.Op(wasm.End)
		for i, local := range []uint32{a, d} {
			arg(&b, uint32(i))
			b.I32Const(fn)
			b.Call(c.rt.num)
			b.LocalSet(local)
		}
		b.LocalGet(d)
		b.Op(wasm.I64Eqz)
		b.If(wasm.BlockVoid)
		c.fail(&b, name+": division by zero", nil)
		b.Op(wasm.End)
		b.LocalGet(a)
		b.LocalGet(d)
		if name == "/" {
			b.Op(wasm.I64DivS)
		} else {
			b.Op(wasm.I64RemS)
		}
		encode(&b)
		return vals(i64, i64), b
	case "<", ">", "<=", ">=":
		fn := int32(c.str(name))
		argcIs(2, wasm.I32Ne)
		b.If(wasm.BlockVoid)
		c.fail(&b, name+": expects 2 arguments, got ", putArgc)
		b.Op(wasm.End)
		for i := uint32(0); i < 2; i++ {
			arg(&b, i)
			b.I32Const(fn)
			b.Call(c.rt.num)
		}
		b.Op(map[string]byte{"<": wasm.I64LtS, ">": wasm.I64GtS, "<=": wasm.I64LeS, ">=": wasm.I64GeS}[name])
		boolean(&b)
		return nil, b
	case "number?", "string?", "symbol?", "list?", "function?", "boolean?", "nil?":
		const t = 3
		argcIs(1, wasm.I32Ne)
		b.If(wasm.BlockVoid)
		c.fail(&b, name+": expects 1 argument, got ", putArgc)
		b.Op(wasm.End)
		arg(&b, 0)
		b.Call(c.rt.tagOf)
		b.LocalSet(t)
		tags := map[string][]int32{
			"number?": {tagInt}, "string?": {tagString}, "symbol?": {tagSymbol},
			"list?": {tagEmpty, tagPair}, "function?": {tagClosure},
			"boolean?": {tagTrue, tagFalse}, "nil?": {tagNil},
		}[name]
		for k, tag := range tags {
			b.LocalGet(t)
			b.I32Const(tag)
			b.Op(wasm.I32Eq)
			if k > 0 {
				b.Op(wasm.I32Or)
			}
		}
		boolean(&b)
		return vals(i32), b
	case "str", "string_length", "substring", "chars", "string<", "symbol", "parse_number":
		return c.stringBuiltin(name)
	case "eq":
		argcIs(2, wasm.I32Ne)
		b.If(wasm.BlockVoid)
		c.fail(&b, "EQ requires exactly 2 parameters; you have ", func() {
			putArgc()
			c.text(&b, " - ")
			b.LocalGet(1)
			b.LocalGet(argc)
			b.Call(c.rt.putArgs)
		})
		b.Op(wasm.End)
		arg(&b, 0)
		arg(&b, 1)
		b.Call(c.rt.equal)
		boolean(&b)
		return nil, b
	case "apply":
		const f = 3
		argcIs(0, wasm.I32Eq)
		b.If(wasm.BlockVoid)
		c.fail(&b, "apply: expects a function", nil)
		b.Op(wasm.End)
		arg(&b, 0)
		b.LocalSet(f)
		c.tagIs(&b, f, tagClosure)
		b.Op(wasm.I32Eqz)
		b.If(wasm.BlockVoid)
		c.fail(&b, "apply: expected a function, got ", func() { c.typeAndValue(&b, f) })
		b.Op(wasm.End)
		b.LocalGet(f)
		b.LocalGet(1)
		b.I32Const(8)
		b.Op(wasm.I32Add)
		b.LocalGet(argc)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.I32Const(0)
		b.Call(c.rt.invoke)
		return vals(i64), b
	case "p":
		const i = 3
		b.Block(wasm.BlockVoid)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(i)
		b.LocalGet(argc)
		b.Op(wasm.I32GeU)
		b.BrIf(1)
		b.LocalGet(i)
		b.If(wasm.BlockVoid)
		b.I32Const(' ')
		b.Call(c.rt.putByte)
		b.Op(wasm.End)
		argAt(&b, i)
		b.Call(c.rt.putShow)
		incr(&b, i)
		b.Br(0)
		b.Op(wasm.End, wasm.End)
		b.I32Const('\n')
		b.Call(c.rt.putByte)
		b.Call(c.rt.flush)
		b.I64Const(addrEmpty)
		return vals(i32), b
	}
	panic("wasm: no builtin " + name)
}
