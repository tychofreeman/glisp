package glisp

import "github.com/tychofreeman/glisp/wasm"

// wasmRuntime holds the function indices of the runtime that compiled glisp
// calls: memory, printing, errors, calls and the builtins.
type wasmRuntime struct {
	fdWrite, procExit                                        uint32 // WASI imports
	reserve, alloc, flush, putByte, putBytes, putStr, putInt uint32
	putValue, putType, putArgs, failBegin, failEnd           uint32
	tagOf, pair, cell, equal, invoke                         uint32
	putShow, putQuoted, putDescribed                         uint32
	builtins                                                 map[string]uint32 // table index of each builtin
	runeCount, runeOffset, newString                         uint32            // string helpers
	objSize, gcCopy, gcReloc, gc                             uint32            // the garbage collector
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
	rt.reserve = c.declare(sig(vals(i32)))
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
	rt.pair = c.declare(sig(vals(i64, i64), i64))
	rt.cell = c.declare(sig(vals(i64), i64))
	rt.equal = c.declare(sig(vals(i64, i64), i32))
	rt.invoke = c.declare(sig(vals(i64, i32, i32, i32), i64))
	c.declareStrings()
	c.declareGC()
	builtinFuncs := map[string]uint32{}
	rt.builtins = map[string]uint32{}
	for _, spec := range builtinSpecs {
		builtinFuncs[spec.name], rt.builtins[spec.name] = c.declareFn()
	}

	c.define(rt.reserve, c.reserveBody)
	c.define(rt.alloc, c.allocBody)
	c.define(rt.flush, c.flushBody)
	c.define(rt.putByte, c.putByteBody)
	c.define(rt.putBytes, c.putBytesBody)
	c.define(rt.putStr, c.putStrBody)
	c.define(rt.putInt, c.putIntBody)
	c.define(rt.putValue, c.putValueBody)
	c.define(rt.putType, c.putTypeBody)
	c.define(rt.putArgs, c.putArgsBody)
	c.define(rt.putShow, c.putShowBody)
	c.define(rt.putQuoted, c.putQuotedBody)
	c.define(rt.putDescribed, c.putDescribedBody)
	c.define(rt.failBegin, c.failBeginBody)
	c.define(rt.failEnd, c.failEndBody)
	c.define(rt.tagOf, c.tagOfBody)
	c.define(rt.pair, c.pairBody)
	c.define(rt.cell, c.cellBody)
	c.define(rt.equal, c.equalBody)
	c.define(rt.invoke, c.invokeBody)
	c.defineStrings()
	for _, spec := range builtinSpecs {
		c.define(builtinFuncs[spec.name], func(b *wasmBody) { c.builtinBody(b, spec) })
	}
}

// Small sequences used throughout.

// isList leaves whether the tag on the stack is a list's: empty or a pair.
func (b *wasmBody) isList() {
	b.I32Const(tagEmpty)
	b.Op(wasm.I32Sub)
	b.I32Const(1)
	b.Op(wasm.I32LeU)
}

// tagIs leaves whether value local v has tag t.
func (c *wasmCompiler) tagIs(b *wasmBody, v uint32, t int32) {
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.I32Const(t)
	b.Op(wasm.I32Eq)
}

// field loads the i64 at offset in the object whose value is local v.
func (b *wasmBody) field(v uint32, offset uint32) {
	b.LocalGet(v)
	b.Op(wasm.I32WrapI64)
	b.I64Load(offset)
}

// arg loads argument i of a glisp function.
func (b *wasmBody) arg(i uint32) {
	b.LocalGet(paramArgs)
	b.I64Load(8 * i)
}

// argAt loads the argument whose index is in local i.
func (b *wasmBody) argAt(i uint32) {
	b.LocalGet(paramArgs)
	b.LocalGet(i)
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
	b.I64Load(0)
}

func (b *wasmBody) incr(i uint32) {
	b.LocalGet(i)
	b.I32Const(1)
	b.Op(wasm.I32Add)
	b.LocalSet(i)
}

// boolean turns the i32 condition on the stack into true or false.
func (b *wasmBody) boolean() {
	b.ifElse(wasm.BlockI64, func() {
		b.I64Const(addrTrue)
	}, func() {
		b.I64Const(addrFalse)
	})
}

// encode turns the integer on the stack into a value.
func (b *wasmBody) encode() {
	b.I64Const(1)
	b.Op(wasm.I64Shl)
	b.I64Const(1)
	b.Op(wasm.I64Or)
}

// returnIf returns from the function if the i32 on the stack is true,
// leaving what then leaves.
func (b *wasmBody) returnIf(then func()) {
	b.when(func() {
		then()
		b.Op(wasm.Return)
	})
}

// when emits an if without an else: then runs only if the i32 on the
// stack is true.
func (b *wasmBody) when(then func()) {
	b.If(wasm.BlockVoid)
	then()
	b.Op(wasm.End)
}

// ifElse emits an if of block type bt: then if the i32 on the stack is
// true, otherwise els.
func (b *wasmBody) ifElse(bt wasm.BlockType, then, els func()) {
	b.If(bt)
	then()
	b.Op(wasm.Else)
	els()
	b.Op(wasm.End)
}

// until emits a loop that runs body until done leaves true. Inside body, a
// br 0 starts the next pass and a br 1 leaves the loop.
func (b *wasmBody) until(done, body func()) {
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	done()
	b.BrIf(1)
	body()
	b.Br(0)
	b.Op(wasm.End, wasm.End)
}

// typeAndValue prints the type and value in local v, as an error does.
func (c *wasmCompiler) typeAndValue(b *wasmBody, v uint32) {
	b.LocalGet(v)
	b.Call(c.rt.putDescribed)
}

// reserve(end) grows memory, if it doesn't reach end yet, by the larger of
// the pages needed and its size, doubling it, so a program that allocates
// a lot grows memory a few times rather than once per allocation; if that
// much isn't available, by the pages needed alone.
func (c *wasmCompiler) reserveBody(b *wasmBody) {
	end, need := b.param(0), b.local(i32)
	b.LocalGet(end)
	b.MemorySize()
	b.I32Const(16)
	b.Op(wasm.I32Shl, wasm.I32LeU)
	b.returnIf(func() {})
	b.LocalGet(end)
	b.MemorySize()
	b.I32Const(16)
	b.Op(wasm.I32Shl, wasm.I32Sub)
	b.I32Const(65535)
	b.Op(wasm.I32Add)
	b.I32Const(16)
	b.Op(wasm.I32ShrU)
	b.LocalSet(need)
	b.MemorySize()
	b.LocalGet(need)
	b.MemorySize()
	b.LocalGet(need)
	b.Op(wasm.I32GtU, wasm.Select)
	b.MemoryGrow()
	b.I32Const(-1)
	b.Op(wasm.I32Ne)
	b.returnIf(func() {})
	b.LocalGet(need)
	b.MemoryGrow()
	b.I32Const(-1)
	b.Op(wasm.I32Eq)
	b.when(func() {
		// Report the error, not capture it into a string.
		b.I32Const(0)
		b.GlobalSet(gCapture)
		c.fail(b, "out of memory", nil)
	})
}

// alloc(size) returns size bytes of heap, doubling memory when it runs out.
func (c *wasmCompiler) allocBody(b *wasmBody) {
	size, p, end := b.param(0), b.local(i32), b.local(i32)
	b.GlobalGet(gHP)
	b.LocalTee(p)
	b.LocalGet(size)
	b.Op(wasm.I32Add)
	b.I32Const(7)
	b.Op(wasm.I32Add)
	b.I32Const(-8)
	b.Op(wasm.I32And)
	b.LocalTee(end)
	b.Call(c.rt.reserve)
	b.LocalGet(end)
	b.GlobalSet(gHP)
	b.LocalGet(p)
}

// flush writes the output buffer to the current fd.
func (c *wasmCompiler) flushBody(b *wasmBody) {
	b.GlobalGet(gOutLen)
	b.Op(wasm.I32Eqz)
	b.returnIf(func() {})
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
}

func (c *wasmCompiler) putByteBody(b *wasmBody) {
	ch := b.param(0)
	// While str captures, the byte goes on the end of the string it builds.
	b.GlobalGet(gCapture)
	b.when(func() {
		b.GlobalGet(gCapEnd)
		b.I32Const(1)
		b.Op(wasm.I32Add)
		b.Call(c.rt.reserve)
		b.GlobalGet(gCapEnd)
		b.LocalGet(ch)
		b.I32Store8(0)
		b.GlobalGet(gCapEnd)
		b.I32Const(1)
		b.Op(wasm.I32Add)
		b.GlobalSet(gCapEnd)
		b.Op(wasm.Return)
	})
	b.GlobalGet(gOutBuf)
	b.GlobalGet(gOutLen)
	b.Op(wasm.I32Add)
	b.LocalGet(ch)
	b.I32Store8(0)
	b.GlobalGet(gOutLen)
	b.I32Const(1)
	b.Op(wasm.I32Add)
	b.GlobalSet(gOutLen)
	b.GlobalGet(gOutLen)
	b.I32Const(outCap)
	b.Op(wasm.I32Eq)
	b.when(func() {
		b.Call(c.rt.flush)
	})
}

// putBytes(ptr, n) prints n bytes from ptr.
func (c *wasmCompiler) putBytesBody(b *wasmBody) {
	ptr, n, i := b.param(0), b.param(1), b.local(i32)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
	}, func() {
		b.LocalGet(ptr)
		b.LocalGet(i)
		b.Op(wasm.I32Add)
		b.I32Load8U(0)
		b.Call(c.rt.putByte)
		b.incr(i)
	})
}

// putStr(s) prints the string object s.
func (c *wasmCompiler) putStrBody(b *wasmBody) {
	s := b.param(0)
	b.LocalGet(s)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(s)
	b.I32Load(4)
	b.Call(c.rt.putBytes)
}

// putInt(n) prints n in decimal.
func (c *wasmCompiler) putIntBody(b *wasmBody) {
	n, p := b.param(0), b.local(i32)
	const end = addrDigits + 24
	b.LocalGet(n)
	b.I64Const(0)
	b.Op(wasm.I64LtS)
	b.when(func() {
		b.I32Const('-')
		b.Call(c.rt.putByte)
		b.I64Const(0)
		b.LocalGet(n)
		b.Op(wasm.I64Sub)
		b.LocalSet(n)
	})
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
}

// putValue(v) prints v as Write does.
func (c *wasmCompiler) putValueBody(b *wasmBody) {
	v, t, first := b.param(0), b.local(i32), b.local(i32)
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.LocalSet(t)
	when := func(tag int32, then func()) {
		b.LocalGet(t)
		b.I32Const(tag)
		b.Op(wasm.I32Eq)
		b.returnIf(then)
	}
	when(tagInt, func() {
		b.LocalGet(v)
		b.I64Const(1)
		b.Op(wasm.I64ShrS)
		b.Call(c.rt.putInt)
	})
	when(tagNil, func() { c.text(b, "nil") })
	when(tagTrue, func() { c.text(b, "true") })
	when(tagFalse, func() { c.text(b, "false") })
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
	when(tagClosure, func() { c.text(b, "<function>") })
	// A list: (a b c).
	b.I32Const('(')
	b.Call(c.rt.putByte)
	b.I32Const(1)
	b.LocalSet(first)
	b.until(func() {
		b.LocalGet(v)
		b.I64Const(addrEmpty)
		b.Op(wasm.I64Eq)
	}, func() {
		b.LocalGet(first)
		b.Op(wasm.I32Eqz)
		b.when(func() {
			b.I32Const(' ')
			b.Call(c.rt.putByte)
		})
		b.I32Const(0)
		b.LocalSet(first)
		b.field(v, 8)
		b.Call(c.rt.putValue)
		b.field(v, 16)
		b.LocalSet(v)
	})
	b.I32Const(')')
	b.Call(c.rt.putByte)
}

// putArgs(args, argc) prints the arguments as a list: (a b c).
func (c *wasmCompiler) putArgsBody(b *wasmBody) {
	args, argc, i := b.param(0), b.param(1), b.local(i32)
	b.I32Const('(')
	b.Call(c.rt.putByte)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(argc)
		b.Op(wasm.I32GeU)
	}, func() {
		b.LocalGet(i)
		b.when(func() {
			b.I32Const(' ')
			b.Call(c.rt.putByte)
		})
		b.LocalGet(args)
		b.LocalGet(i)
		b.I32Const(3)
		b.Op(wasm.I32Shl, wasm.I32Add)
		b.I64Load(0)
		b.Call(c.rt.putValue)
		b.incr(i)
	})
	b.I32Const(')')
	b.Call(c.rt.putByte)
}

// putShow(v) prints v as Show does: a string as its text, anything else as
// putValue writes it.
func (c *wasmCompiler) putShowBody(b *wasmBody) {
	v := b.param(0)
	c.tagIs(b, v, tagString)
	b.returnIf(func() {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.Call(c.rt.putStr)
	})
	b.LocalGet(v)
	b.Call(c.rt.putValue)
}

// putQuoted(s) prints the string object s in quotes, escaping as quote does.
func (c *wasmCompiler) putQuotedBody(b *wasmBody) {
	s, i, n, ch := b.param(0), b.local(i32), b.local(i32), b.local(i32)
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
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
	}, func() {
		b.LocalGet(s)
		b.LocalGet(i)
		b.Op(wasm.I32Add)
		b.I32Load8U(8)
		b.LocalSet(ch)
		is('\n')
		b.ifElse(wasm.BlockVoid, func() {
			put('\\', 'n')
		}, func() {
			is('\t')
			b.ifElse(wasm.BlockVoid, func() {
				put('\\', 't')
			}, func() {
				is('"')
				is('\\')
				b.Op(wasm.I32Or)
				b.when(func() {
					put('\\')
				})
				b.LocalGet(ch)
				b.Call(c.rt.putByte)
			})
		})
		b.incr(i)
	})
	put('"')
}

// putDescribed(v) prints v's type and value, as described does.
func (c *wasmCompiler) putDescribedBody(b *wasmBody) {
	v := b.param(0)
	b.LocalGet(v)
	b.I64Const(addrNil)
	b.Op(wasm.I64Eq)
	b.returnIf(func() { c.text(b, "nil") })
	b.LocalGet(v)
	b.Call(c.rt.putType)
	b.I32Const(' ')
	b.Call(c.rt.putByte)
	b.LocalGet(v)
	b.Call(c.rt.putValue)
}

// failBegin flushes the program's output and starts an error message on
// stderr; failEnd ends it and exits with status 1.
func (c *wasmCompiler) failBeginBody(b *wasmBody) {
	b.Call(c.rt.flush)
	b.I32Const(2)
	b.GlobalSet(gFD)
	c.text(b, "glisp: ")
	b.GlobalGet(gLine)
	b.when(func() {
		c.text(b, "line ")
		b.GlobalGet(gLine)
		b.Op(wasm.I64ExtendI32U)
		b.Call(c.rt.putInt)
		c.text(b, ": ")
	})
}

func (c *wasmCompiler) failEndBody(b *wasmBody) {
	b.I32Const('\n')
	b.Call(c.rt.putByte)
	b.Call(c.rt.flush)
	b.I32Const(1)
	b.Call(c.rt.procExit)
}

// tagOf(v) is v's tag; tagInt for an integer.
func (c *wasmCompiler) tagOfBody(b *wasmBody) {
	v := b.param(0)
	b.LocalGet(v)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I32WrapI64)
	b.ifElse(wasm.BlockI32, func() {
		b.I32Const(tagInt)
	}, func() {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.I32Load(0)
	})
}

// pair(car, cdr) makes a list cell.
func (c *wasmCompiler) pairBody(b *wasmBody) {
	car, cdr, p := b.param(0), b.param(1), b.local(i32)
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
}

// cell(v) makes a cell holding v.
func (c *wasmCompiler) cellBody(b *wasmBody) {
	v, p := b.param(0), b.local(i32)
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
}

// equal(a, b) is the interpreter's equal: lists element by element, strings
// and symbols by their text, functions never.
func (c *wasmCompiler) equalBody(b *wasmBody) {
	a, bv, ta, tb, n, i := b.param(0), b.param(1), b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	b.LocalGet(a)
	b.Call(c.rt.tagOf)
	b.LocalSet(ta)
	b.LocalGet(bv)
	b.Call(c.rt.tagOf)
	b.LocalSet(tb)
	no := func() { b.I32Const(0) }

	b.LocalGet(ta)
	b.isList()
	b.when(func() {
		b.LocalGet(tb)
		b.isList()
		b.Op(wasm.I32Eqz)
		b.returnIf(no)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(a)
		b.I64Const(addrEmpty)
		b.Op(wasm.I64Eq)
		b.LocalGet(bv)
		b.I64Const(addrEmpty)
		b.Op(wasm.I64Eq)
		b.Op(wasm.I32Or)
		b.returnIf(func() {
			b.LocalGet(a)
			b.LocalGet(bv)
			b.Op(wasm.I64Eq)
		})
		b.field(a, 8)
		b.field(bv, 8)
		b.Call(c.rt.equal)
		b.Op(wasm.I32Eqz)
		b.returnIf(no)
		b.field(a, 16)
		b.LocalSet(a)
		b.field(bv, 16)
		b.LocalSet(bv)
		b.Br(0)
		b.Op(wasm.End)
	})

	b.LocalGet(ta)
	b.LocalGet(tb)
	b.Op(wasm.I32Ne)
	b.returnIf(no)
	b.LocalGet(ta)
	b.I32Const(tagInt)
	b.Op(wasm.I32Eq)
	b.returnIf(func() {
		b.LocalGet(a)
		b.LocalGet(bv)
		b.Op(wasm.I64Eq)
	})
	b.LocalGet(ta)
	b.I32Const(tagClosure)
	b.Op(wasm.I32Eq)
	b.returnIf(no)
	b.LocalGet(ta)
	b.I32Const(tagString)
	b.Op(wasm.I32Eq)
	b.LocalGet(ta)
	b.I32Const(tagSymbol)
	b.Op(wasm.I32Eq)
	b.Op(wasm.I32Or, wasm.I32Eqz)
	b.returnIf(func() { b.I32Const(1) }) // nil, true, false
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
	b.returnIf(no)
	byteAt := func(v uint32) {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
		b.LocalGet(i)
		b.Op(wasm.I32Add)
		b.I32Load8U(8)
	}
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
	}, func() {
		byteAt(a)
		byteAt(bv)
		b.Op(wasm.I32Ne)
		b.returnIf(no)
		b.incr(i)
	})
	b.I32Const(1)
}

// invoke(f, args, argc, site) calls the function f with argc arguments at
// args. site is the call, printed if f isn't a function.
func (c *wasmCompiler) invokeBody(b *wasmBody) {
	f, args, argc, site, r, entry, i := b.param(0), b.param(1), b.param(2), b.param(3), b.local(i64), b.local(i32), b.local(i32)
	c.checkStack(b)
	c.tagIs(b, f, tagClosure)
	b.Op(wasm.I32Eqz)
	c.failIf(b, "Cannot call ", func() {
		c.typeAndValue(b, f)
		c.text(b, " in ")
		b.LocalGet(site)
		b.Call(c.rt.putStr)
	})
	b.GlobalGet(gSP)
	b.LocalSet(entry)
	c.callValue(b, f, args, argc)
	b.LocalSet(r)
	// While the function called made a tail call, make that call: move its
	// arguments down to where this invoke's stack starts, so a chain of tail
	// calls never grows the stack, and call it.
	b.until(func() {
		b.LocalGet(r)
		b.I64Const(pending)
		b.Op(wasm.I64Ne)
	}, func() {
		b.GlobalGet(gPendingFn)
		b.LocalSet(f)
		b.GlobalGet(gPendingArgc)
		b.LocalSet(argc)
		b.I32Const(0)
		b.LocalSet(i)
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(argc)
			b.Op(wasm.I32GeU)
		}, func() {
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
			b.incr(i)
		})
		b.LocalGet(entry)
		b.LocalSet(args)
		b.LocalGet(entry)
		b.LocalGet(argc)
		b.I32Const(3)
		b.Op(wasm.I32Shl, wasm.I32Add)
		b.GlobalSet(gSP)
		c.checkStack(b)
		c.callValue(b, f, args, argc)
		b.LocalSet(r)
		b.LocalGet(entry)
		b.GlobalSet(gSP)
	})
	b.LocalGet(r)
}

// callValue calls the function value in local f, a closure, with argc
// arguments at args: through the table, at the closure's index.
func (c *wasmCompiler) callValue(b *wasmBody, f, args, argc uint32) {
	b.LocalGet(f)
	b.LocalGet(args)
	b.LocalGet(argc)
	b.LocalGet(f)
	b.tableIndex()
	b.CallIndirect(c.fnType)
}

// tableIndex turns the closure on the stack into the table index of its
// function.
func (b *wasmBody) tableIndex() {
	b.Op(wasm.I32WrapI64)
	b.I32Load(4)
}

// checkStack fails if the argument stack has grown past its end.
func (c *wasmCompiler) checkStack(b *wasmBody) {
	b.GlobalGet(gSP)
	b.GlobalGet(gStackEnd)
	b.Op(wasm.I32GtU)
	c.failIf(b, "stack overflow", nil)
}

// checkArgs fails, as the interpreter does, unless the builtin's arguments
// are what spec takes: how many, then the kind of each.
func (c *wasmCompiler) checkArgs(b *wasmBody, spec builtin) {
	argc := b.param(paramArgc)
	argcIs := func(n int, op byte) {
		b.LocalGet(argc)
		b.I32Const(int32(n))
		b.Op(op)
	}
	wrongCount := true
	switch {
	case spec.max < 0 && spec.min == 0:
		wrongCount = false
	case spec.max < 0:
		argcIs(spec.min, wasm.I32LtU)
	case spec.min == spec.max:
		argcIs(spec.min, wasm.I32Ne)
	default:
		argcIs(spec.min, wasm.I32LtU)
		argcIs(spec.max, wasm.I32GtU)
		b.Op(wasm.I32Or)
	}
	if wrongCount {
		c.failIf(b, spec.name+": "+spec.expects()+", got ", func() {
			b.LocalGet(argc)
			b.Op(wasm.I64ExtendI32U)
			b.Call(c.rt.putInt)
		})
	}
	var v, t uint32 // the argument checked, and its tag
	for _, k := range spec.kinds {
		if k != anyKind {
			v, t = b.local(i64), b.local(i32)
			break
		}
	}
	check := func(k kind, load func()) {
		load()
		b.LocalTee(v)
		b.Call(c.rt.tagOf)
		b.LocalSet(t)
		b.tagIn(t, k.tags())
		b.Op(wasm.I32Eqz)
		c.failIf(b, spec.name+": expected "+k.noun()+", got ", func() { c.typeAndValue(b, v) })
	}
	// Each argument up to the last kind given, then, for a builtin taking
	// any number, the rest in a loop: the last kind stands for them.
	last := len(spec.kinds) - 1
	fixed := spec.max
	if spec.max < 0 {
		fixed = last
	}
	for i := 0; i < fixed; i++ {
		if k := spec.kind(i); k != anyKind {
			if i >= spec.min {
				panic("wasm: " + spec.name + ": a kind for an optional argument")
			}
			check(k, func() { b.arg(uint32(i)) })
		}
	}
	if spec.max < 0 && last >= 0 && spec.kinds[last] != anyKind {
		j := b.local(i32)
		b.I32Const(int32(last))
		b.LocalSet(j)
		b.until(func() {
			b.LocalGet(j)
			b.LocalGet(argc)
			b.Op(wasm.I32GeU)
		}, func() {
			check(spec.kinds[last], func() { b.argAt(j) })
			b.incr(j)
		})
	}
}

// tagIn leaves whether the tag in local t is one of tags.
func (b *wasmBody) tagIn(t uint32, tags []int32) {
	for k, tag := range tags {
		b.LocalGet(t)
		b.I32Const(tag)
		b.Op(wasm.I32Eq)
		if k > 0 {
			b.Op(wasm.I32Or)
		}
	}
}

// decode turns the value on the stack, an integer, into the integer.
func (b *wasmBody) decode() {
	b.I64Const(1)
	b.Op(wasm.I64ShrS)
}

// builtinBody compiles the builtin name as a glisp function:
// (self, args, argc) -> value.
func (c *wasmCompiler) builtinBody(b *wasmBody, spec builtin) {
	c.checkArgs(b, spec)
	argc := b.param(paramArgc)
	name := spec.name
	switch name {
	case "car", "cdr":
		v := b.local(i64)
		offset := uint32(8)
		if name == "cdr" {
			offset = 16
		}
		b.arg(0)
		b.LocalSet(v)
		c.tagIs(b, v, tagPair)
		b.ifElse(wasm.BlockI64, func() {
			b.field(v, offset)
		}, func() {
			b.I64Const(addrNil)
		})
	case "atom":
		b.arg(0)
		b.Call(c.rt.tagOf)
		b.isList()
		b.Op(wasm.I32Eqz)
		b.boolean()
	case "cons":
		rest := b.local(i64)
		b.LocalGet(argc)
		b.I32Const(1)
		b.Op(wasm.I32Eq)
		b.returnIf(func() {
			b.arg(0)
			b.I64Const(addrEmpty)
			b.Call(c.rt.pair)
		})
		b.arg(1)
		b.LocalTee(rest)
		b.I64Const(addrNil)
		b.Op(wasm.I64Eq)
		b.returnIf(func() {
			b.arg(0)
			b.I64Const(addrEmpty)
			b.Call(c.rt.pair)
		})
		b.LocalGet(rest)
		b.Call(c.rt.tagOf)
		b.isList()
		b.returnIf(func() {
			b.arg(0)
			b.LocalGet(rest)
			b.Call(c.rt.pair)
		})
		b.arg(0)
		b.LocalGet(rest)
		b.I64Const(addrEmpty)
		b.Call(c.rt.pair)
		b.Call(c.rt.pair)
	case "plus", "-", "*":
		sum, i := b.local(i64), b.local(i32)
		number := func() {
			b.argAt(i)
			b.decode()
		}
		op := wasm.I64Add
		if name == "*" {
			op = wasm.I64Mul
			b.I64Const(1)
			b.LocalSet(sum)
		}
		if name == "-" {
			op = wasm.I64Sub
			number()
			b.LocalSet(sum)
			b.LocalGet(argc)
			b.I32Const(1)
			b.Op(wasm.I32Eq)
			b.returnIf(func() {
				b.I64Const(0)
				b.LocalGet(sum)
				b.Op(wasm.I64Sub)
				b.encode()
			})
			b.I32Const(1)
			b.LocalSet(i)
		}
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(argc)
			b.Op(wasm.I32GeU)
		}, func() {
			b.LocalGet(sum)
			number()
			b.Op(op)
			b.LocalSet(sum)
			b.incr(i)
		})
		b.LocalGet(sum)
		b.encode()
	case "/", "mod":
		a, d := b.local(i64), b.local(i64)
		for i, local := range []uint32{a, d} {
			b.arg(uint32(i))
			b.decode()
			b.LocalSet(local)
		}
		b.LocalGet(d)
		b.Op(wasm.I64Eqz)
		c.failIf(b, name+": division by zero", nil)
		b.LocalGet(a)
		b.LocalGet(d)
		if name == "/" {
			b.Op(wasm.I64DivS)
		} else {
			b.Op(wasm.I64RemS)
		}
		b.encode()
	case "<", ">", "<=", ">=":
		// Encoding keeps integers' order.
		b.arg(0)
		b.arg(1)
		b.Op(map[string]byte{"<": wasm.I64LtS, ">": wasm.I64GtS, "<=": wasm.I64LeS, ">=": wasm.I64GeS}[name])
		b.boolean()
	case "eq":
		b.arg(0)
		b.arg(1)
		b.Call(c.rt.equal)
		b.boolean()
	case "apply":
		b.arg(0)
		b.LocalGet(paramArgs)
		b.I32Const(8)
		b.Op(wasm.I32Add)
		b.LocalGet(argc)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.I32Const(0)
		b.Call(c.rt.invoke)
	case "p":
		i := b.local(i32)
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(argc)
			b.Op(wasm.I32GeU)
		}, func() {
			b.LocalGet(i)
			b.when(func() {
				b.I32Const(' ')
				b.Call(c.rt.putByte)
			})
			b.argAt(i)
			b.Call(c.rt.putShow)
			b.incr(i)
		})
		b.I32Const('\n')
		b.Call(c.rt.putByte)
		b.Call(c.rt.flush)
		b.I64Const(addrEmpty)
	default:
		if typ, isPredicate := predicateType(name); isPredicate {
			t := b.local(i32)
			b.arg(0)
			b.Call(c.rt.tagOf)
			b.LocalSet(t)
			b.tagIn(t, tagsOf(typ))
			b.boolean()
			return
		}
		c.stringBuiltin(b, name)
	}
}
