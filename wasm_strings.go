package glisp

import "github.com/tychofreeman/glisp/wasm"

// The string builtins, compiled by stringBuiltin. Strings are byte arrays
// of UTF-8; indexes and lengths count characters, the bytes that don't
// continue one (b&0xC0 != 0x80).
var stringBuiltinNames = []string{"str", "string_length", "substring", "chars", "string<", "symbol", "parse_number"}

// declareStrings declares the string helpers; defineStrings defines them.
func (c *wasmCompiler) declareStrings() {
	rt := &c.rt
	rt.strArg = c.declare(sig(vals(i64, i32), i32))
	rt.runeCount = c.declare(sig(vals(i32), i32))
	rt.runeOffset = c.declare(sig(vals(i32, i32), i32))
	rt.newString = c.declare(sig(vals(i32, i32), i32))
}

func (c *wasmCompiler) defineStrings() {
	c.define(c.rt.strArg, nil, c.strArgBody())
	c.define(c.rt.runeCount, vals(i32, i32, i32), c.runeCountBody())
	c.define(c.rt.runeOffset, vals(i32, i32, i32), c.runeOffsetBody())
	c.define(c.rt.newString, vals(i32), c.newStringBody())
}

// startsRune leaves whether the byte on the stack starts a character.
func startsRune(b *wasm.Code) {
	b.I32Const(0xc0)
	b.Op(wasm.I32And)
	b.I32Const(0x80)
	b.Op(wasm.I32Ne)
}

// byteAt leaves the byte at index local i of the string at local s.
func byteAt(b *wasm.Code, s, i uint32) {
	b.LocalGet(s)
	b.LocalGet(i)
	b.Op(wasm.I32Add)
	b.I32Load8U(8)
}

// copyBytes copies n bytes (local) from offset src (local) of the string at
// local from to offset 8 of the string at local to.
func copyBytes(b *wasm.Code, to, from, src, n uint32) {
	b.LocalGet(to)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(from)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(src)
	b.Op(wasm.I32Add)
	b.LocalGet(n)
	b.MemoryCopy()
}

// strArg(v, fn) is the address of the string v, or an error naming fn.
func (c *wasmCompiler) strArgBody() wasm.Code {
	const v, fn = 0, 1
	var b wasm.Code
	c.tagIs(&b, v, tagString)
	returnIf(&b, func() {
		b.LocalGet(v)
		b.Op(wasm.I32WrapI64)
	})
	b.Call(c.rt.failBegin)
	b.LocalGet(fn)
	b.Call(c.rt.putStr)
	c.text(&b, ": expected a string, got ")
	c.typeAndValue(&b, v)
	b.Call(c.rt.failEnd)
	b.Op(wasm.Unreachable)
	return b
}

// runeCount(s) is how many characters the string at s has.
func (c *wasmCompiler) runeCountBody() wasm.Code {
	const s, n, i, count = 0, 1, 2, 3
	var b wasm.Code
	b.LocalGet(s)
	b.I32Load(4)
	b.LocalSet(n)
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(n)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	byteAt(&b, s, i)
	startsRune(&b)
	b.If(wasm.BlockVoid)
	incr(&b, count)
	b.Op(wasm.End)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.LocalGet(count)
	return b
}

// runeOffset(s, k) is the byte offset of character k of the string at s,
// or its length if k is its number of characters.
func (c *wasmCompiler) runeOffsetBody() wasm.Code {
	const s, k, n, i, count = 0, 1, 2, 3, 4
	var b wasm.Code
	b.LocalGet(s)
	b.I32Load(4)
	b.LocalSet(n)
	b.Block(wasm.BlockVoid)
	b.Loop(wasm.BlockVoid)
	b.LocalGet(i)
	b.LocalGet(n)
	b.Op(wasm.I32GeU)
	b.BrIf(1)
	byteAt(&b, s, i)
	startsRune(&b)
	b.If(wasm.BlockVoid)
	b.LocalGet(count)
	b.LocalGet(k)
	b.Op(wasm.I32Eq)
	returnIf(&b, func() { b.LocalGet(i) })
	incr(&b, count)
	b.Op(wasm.End)
	incr(&b, i)
	b.Br(0)
	b.Op(wasm.End, wasm.End)
	b.LocalGet(n)
	return b
}

// newString(n, tag) makes a string (or, with tagSymbol, a symbol) of n
// bytes, to be filled in.
func (c *wasmCompiler) newStringBody() wasm.Code {
	const n, tag, p = 0, 1, 2
	var b wasm.Code
	b.I32Const(8)
	b.LocalGet(n)
	b.Op(wasm.I32Add)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.LocalGet(tag)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(n)
	b.I32Store(4)
	b.LocalGet(p)
	return b
}

// stringBuiltin compiles the string builtin name as a glisp function:
// (self, args, argc) -> value.
func (c *wasmCompiler) stringBuiltin(name string) ([]wasm.ValType, wasm.Code) {
	const args, argc = 1, 2
	var b wasm.Code
	fn := int32(c.str(name))
	expectArgc := func(n int32) {
		b.LocalGet(argc)
		b.I32Const(n)
		b.Op(wasm.I32Ne)
		b.If(wasm.BlockVoid)
		arguments := " arguments, got "
		if n == 1 {
			arguments = " argument, got "
		}
		c.fail(&b, name+": expects "+string(rune('0'+n))+arguments, func() {
			b.LocalGet(argc)
			b.Op(wasm.I64ExtendI32U)
			b.Call(c.rt.putInt)
		})
		b.Op(wasm.End)
	}
	stringArg := func(i, into uint32) {
		arg(&b, i)
		b.I32Const(fn)
		b.Call(c.rt.strArg)
		b.LocalSet(into)
	}
	switch name {
	case "str":
		// p's printing, captured into a new string on the heap: nothing
		// else allocates while it prints.
		const i, start = 3, 4
		b.GlobalGet(gHP)
		b.LocalTee(start)
		b.I32Const(8)
		b.Op(wasm.I32Add)
		b.GlobalSet(gCapEnd)
		b.I32Const(1)
		b.GlobalSet(gCapture)
		b.Block(wasm.BlockVoid)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(i)
		b.LocalGet(argc)
		b.Op(wasm.I32GeU)
		b.BrIf(1)
		argAt(&b, i)
		b.Call(c.rt.putShow)
		incr(&b, i)
		b.Br(0)
		b.Op(wasm.End, wasm.End)
		b.I32Const(0)
		b.GlobalSet(gCapture)
		b.LocalGet(start)
		b.I32Const(tagString)
		b.I32Store(0)
		b.LocalGet(start)
		b.GlobalGet(gCapEnd)
		b.LocalGet(start)
		b.Op(wasm.I32Sub)
		b.I32Const(8)
		b.Op(wasm.I32Sub)
		b.I32Store(4)
		b.GlobalGet(gCapEnd)
		b.I32Const(7)
		b.Op(wasm.I32Add)
		b.I32Const(-8)
		b.Op(wasm.I32And)
		b.GlobalSet(gHP)
		b.LocalGet(start)
		b.Op(wasm.I64ExtendI32U)
		return vals(i32, i32), b
	case "string_length":
		const s = 3
		expectArgc(1)
		stringArg(0, s)
		b.LocalGet(s)
		b.Call(c.rt.runeCount)
		b.Op(wasm.I64ExtendI32U)
		encode(&b)
		return vals(i32), b
	case "substring":
		const s, start, end, length, from, n, r = 3, 4, 5, 6, 7, 8, 9
		expectArgc(3)
		stringArg(0, s)
		for i, local := range []uint32{start, end} {
			arg(&b, uint32(i+1))
			b.I32Const(fn)
			b.Call(c.rt.num)
			b.LocalSet(local)
		}
		b.LocalGet(s)
		b.Call(c.rt.runeCount)
		b.Op(wasm.I64ExtendI32U)
		b.LocalSet(length)
		b.LocalGet(start)
		b.I64Const(0)
		b.Op(wasm.I64LtS)
		b.LocalGet(start)
		b.LocalGet(end)
		b.Op(wasm.I64GtS, wasm.I32Or)
		b.LocalGet(end)
		b.LocalGet(length)
		b.Op(wasm.I64GtS, wasm.I32Or)
		b.If(wasm.BlockVoid)
		c.fail(&b, "substring: indexes ", func() {
			b.LocalGet(start)
			b.Call(c.rt.putInt)
			c.text(&b, "..")
			b.LocalGet(end)
			b.Call(c.rt.putInt)
			c.text(&b, " out of range for a string of length ")
			b.LocalGet(length)
			b.Call(c.rt.putInt)
		})
		b.Op(wasm.End)
		b.LocalGet(s)
		b.LocalGet(start)
		b.Op(wasm.I32WrapI64)
		b.Call(c.rt.runeOffset)
		b.LocalSet(from)
		b.LocalGet(s)
		b.LocalGet(end)
		b.Op(wasm.I32WrapI64)
		b.Call(c.rt.runeOffset)
		b.LocalGet(from)
		b.Op(wasm.I32Sub)
		b.LocalTee(n)
		b.I32Const(tagString)
		b.Call(c.rt.newString)
		b.LocalSet(r)
		copyBytes(&b, r, s, from, n)
		b.LocalGet(r)
		b.Op(wasm.I64ExtendI32U)
		return vals(i32, i64, i64, i64, i32, i32, i32), b
	case "chars":
		// From the last character to the first, consing each onto the list.
		const s, i, end, piece, n, list = 3, 4, 5, 6, 7, 8
		expectArgc(1)
		stringArg(0, s)
		b.LocalGet(s)
		b.I32Load(4)
		b.LocalTee(i)
		b.LocalSet(end)
		b.I64Const(addrEmpty)
		b.LocalSet(list)
		b.Block(wasm.BlockVoid)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(i)
		b.Op(wasm.I32Eqz)
		b.BrIf(1)
		b.LocalGet(i)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.LocalSet(i)
		byteAt(&b, s, i)
		startsRune(&b)
		b.If(wasm.BlockVoid)
		b.LocalGet(end)
		b.LocalGet(i)
		b.Op(wasm.I32Sub)
		b.LocalTee(n)
		b.I32Const(tagString)
		b.Call(c.rt.newString)
		b.LocalSet(piece)
		copyBytes(&b, piece, s, i, n)
		b.LocalGet(piece)
		b.Op(wasm.I64ExtendI32U)
		b.LocalGet(list)
		b.Call(c.rt.pair)
		b.LocalSet(list)
		b.LocalGet(i)
		b.LocalSet(end)
		b.Op(wasm.End)
		b.Br(0)
		b.Op(wasm.End, wasm.End)
		b.LocalGet(list)
		return vals(i32, i32, i32, i32, i32, i64), b
	case "string<":
		const a, z, la, lz, i, ca, cz = 3, 4, 5, 6, 7, 8, 9
		expectArgc(2)
		stringArg(0, a)
		stringArg(1, z)
		b.LocalGet(a)
		b.I32Load(4)
		b.LocalSet(la)
		b.LocalGet(z)
		b.I32Load(4)
		b.LocalSet(lz)
		b.Block(wasm.BlockVoid)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(i)
		b.LocalGet(la)
		b.Op(wasm.I32GeU)
		b.LocalGet(i)
		b.LocalGet(lz)
		b.Op(wasm.I32GeU, wasm.I32Or)
		b.BrIf(1)
		byteAt(&b, a, i)
		b.LocalSet(ca)
		byteAt(&b, z, i)
		b.LocalSet(cz)
		b.LocalGet(ca)
		b.LocalGet(cz)
		b.Op(wasm.I32Ne)
		returnIf(&b, func() {
			b.LocalGet(ca)
			b.LocalGet(cz)
			b.Op(wasm.I32LtU)
			boolean(&b)
		})
		incr(&b, i)
		b.Br(0)
		b.Op(wasm.End, wasm.End)
		b.LocalGet(la)
		b.LocalGet(lz)
		b.Op(wasm.I32LtU)
		boolean(&b)
		return vals(i32, i32, i32, i32, i32, i32, i32), b
	case "symbol":
		const s, n, r, zero = 3, 4, 5, 6
		expectArgc(1)
		stringArg(0, s)
		b.LocalGet(s)
		b.I32Load(4)
		b.LocalTee(n)
		b.I32Const(tagSymbol)
		b.Call(c.rt.newString)
		b.LocalSet(r)
		copyBytes(&b, r, s, zero, n)
		b.LocalGet(r)
		b.Op(wasm.I64ExtendI32U)
		return vals(i32, i32, i32, i32), b
	case "parse_number":
		const s, n, i, ch, acc, negative = 3, 4, 5, 6, 7, 8
		expectArgc(1)
		stringArg(0, s)
		b.LocalGet(s)
		b.I32Load(4)
		b.LocalSet(n)
		b.LocalGet(n)
		b.If(wasm.BlockVoid)
		byteAt(&b, s, i)
		b.I32Const('-')
		b.Op(wasm.I32Eq)
		b.If(wasm.BlockVoid)
		b.I32Const(1)
		b.LocalSet(negative)
		b.I32Const(1)
		b.LocalSet(i)
		b.Op(wasm.End, wasm.End)
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32Eq)
		returnIf(&b, func() { b.I64Const(addrNil) })
		b.Block(wasm.BlockVoid)
		b.Loop(wasm.BlockVoid)
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
		b.BrIf(1)
		byteAt(&b, s, i)
		b.LocalTee(ch)
		b.I32Const('0')
		b.Op(wasm.I32LtU)
		b.LocalGet(ch)
		b.I32Const('9')
		b.Op(wasm.I32GtU, wasm.I32Or)
		returnIf(&b, func() { b.I64Const(addrNil) })
		b.LocalGet(acc)
		b.I64Const(10)
		b.Op(wasm.I64Mul)
		b.LocalGet(ch)
		b.I32Const('0')
		b.Op(wasm.I32Sub, wasm.I64ExtendI32U, wasm.I64Add)
		b.LocalSet(acc)
		incr(&b, i)
		b.Br(0)
		b.Op(wasm.End, wasm.End)
		b.LocalGet(negative)
		b.If(wasm.BlockVoid)
		b.I64Const(0)
		b.LocalGet(acc)
		b.Op(wasm.I64Sub)
		b.LocalSet(acc)
		b.Op(wasm.End)
		b.LocalGet(acc)
		encode(&b)
		return vals(i32, i32, i32, i32, i64, i32), b
	}
	panic("wasm: no string builtin " + name)
}
