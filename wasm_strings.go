package glisp

import "github.com/tychofreeman/glisp/wasm"

// The string builtins, compiled by stringBuiltin. Strings are byte arrays
// of UTF-8; indexes and lengths count characters, the bytes that don't
// continue one (b&0xC0 != 0x80).

// declareStrings declares the string helpers; defineStrings defines them.
func (c *wasmCompiler) declareStrings() {
	rt := &c.rt
	rt.runeCount = c.declare(sig(vals(i32), i32))
	rt.runeOffset = c.declare(sig(vals(i32, i32), i32))
	rt.newString = c.declare(sig(vals(i32, i32), i32))
}

func (c *wasmCompiler) defineStrings() {
	c.define(c.rt.runeCount, c.runeCountBody)
	c.define(c.rt.runeOffset, c.runeOffsetBody)
	c.define(c.rt.newString, c.newStringBody)
}

// startsRune leaves whether the byte on the stack starts a character.
func (b *wasmBody) startsRune() {
	b.I32Const(0xc0)
	b.Op(wasm.I32And)
	b.I32Const(0x80)
	b.Op(wasm.I32Ne)
}

// byteAt leaves the byte at index local i of the string at local s.
func (b *wasmBody) byteAt(s, i uint32) {
	b.LocalGet(s)
	b.LocalGet(i)
	b.Op(wasm.I32Add)
	b.I32Load8U(8)
}

// copyBytes copies n bytes (local) from offset src (local) of the string at
// local from to offset 8 of the string at local to.
func (b *wasmBody) copyBytes(to, from, src, n uint32) {
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

// runeCount(s) is how many characters the string at s has.
func (c *wasmCompiler) runeCountBody(b *wasmBody) {
	s, n, i, count := b.param(0), b.local(i32), b.local(i32), b.local(i32)
	b.LocalGet(s)
	b.I32Load(4)
	b.LocalSet(n)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
	}, func() {
		b.byteAt(s, i)
		b.startsRune()
		b.when(func() {
			b.incr(count)
		})
		b.incr(i)
	})
	b.LocalGet(count)
}

// runeOffset(s, k) is the byte offset of character k of the string at s,
// or its length if k is its number of characters.
func (c *wasmCompiler) runeOffsetBody(b *wasmBody) {
	s, k, n, i, count := b.param(0), b.param(1), b.local(i32), b.local(i32), b.local(i32)
	b.LocalGet(s)
	b.I32Load(4)
	b.LocalSet(n)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
	}, func() {
		b.byteAt(s, i)
		b.startsRune()
		b.when(func() {
			b.LocalGet(count)
			b.LocalGet(k)
			b.Op(wasm.I32Eq)
			b.returnIf(func() { b.LocalGet(i) })
			b.incr(count)
		})
		b.incr(i)
	})
	b.LocalGet(n)
}

// newString(n, tag) makes a string (or, with tagSymbol, a symbol) of n
// bytes, to be filled in.
func (c *wasmCompiler) newStringBody(b *wasmBody) {
	n, tag, p := b.param(0), b.param(1), b.local(i32)
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
}

// stringBuiltin compiles the string builtin name as a glisp function:
// (self, args, argc) -> value.
func (c *wasmCompiler) stringBuiltin(b *wasmBody, name string) {
	argc := b.param(paramArgc)
	stringArg := func(i, into uint32) {
		b.arg(i)
		b.Op(wasm.I32WrapI64)
		b.LocalSet(into)
	}
	switch name {
	case "str":
		// p's printing, captured into a new string on the heap: nothing
		// else allocates while it prints.
		i, start := b.local(i32), b.local(i32)
		b.GlobalGet(gHP)
		b.LocalTee(start)
		b.I32Const(8)
		b.Op(wasm.I32Add)
		b.GlobalSet(gCapEnd)
		b.I32Const(1)
		b.GlobalSet(gCapture)
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(argc)
			b.Op(wasm.I32GeU)
		}, func() {
			b.argAt(i)
			b.Call(c.rt.putShow)
			b.incr(i)
		})
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
	case "string_length":
		s := b.local(i32)
		stringArg(0, s)
		b.LocalGet(s)
		b.Call(c.rt.runeCount)
		b.Op(wasm.I64ExtendI32U)
		b.encode()
	case "substring":
		s, start, end, length, from, n, r := b.local(i32), b.local(i64), b.local(i64), b.local(i64), b.local(i32), b.local(i32), b.local(i32)
		stringArg(0, s)
		for i, local := range []uint32{start, end} {
			b.arg(uint32(i + 1))
			b.decode()
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
		c.failIf(b, "substring: indexes ", func() {
			b.LocalGet(start)
			b.Call(c.rt.putInt)
			c.text(b, "..")
			b.LocalGet(end)
			b.Call(c.rt.putInt)
			c.text(b, " out of range for a string of length ")
			b.LocalGet(length)
			b.Call(c.rt.putInt)
		})
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
		b.copyBytes(r, s, from, n)
		b.LocalGet(r)
		b.Op(wasm.I64ExtendI32U)
	case "chars":
		// From the last character to the first, consing each onto the list.
		s, i, end, piece, n, list := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i64)
		stringArg(0, s)
		b.LocalGet(s)
		b.I32Load(4)
		b.LocalTee(i)
		b.LocalSet(end)
		b.I64Const(addrEmpty)
		b.LocalSet(list)
		b.until(func() {
			b.LocalGet(i)
			b.Op(wasm.I32Eqz)
		}, func() {
			b.LocalGet(i)
			b.I32Const(1)
			b.Op(wasm.I32Sub)
			b.LocalSet(i)
			b.byteAt(s, i)
			b.startsRune()
			b.when(func() {
				b.LocalGet(end)
				b.LocalGet(i)
				b.Op(wasm.I32Sub)
				b.LocalTee(n)
				b.I32Const(tagString)
				b.Call(c.rt.newString)
				b.LocalSet(piece)
				b.copyBytes(piece, s, i, n)
				b.LocalGet(piece)
				b.Op(wasm.I64ExtendI32U)
				b.LocalGet(list)
				b.Call(c.rt.pair)
				b.LocalSet(list)
				b.LocalGet(i)
				b.LocalSet(end)
			})
		})
		b.LocalGet(list)
	case "string<":
		a, z, la, lz, i, ca, cz := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32)
		stringArg(0, a)
		stringArg(1, z)
		b.LocalGet(a)
		b.I32Load(4)
		b.LocalSet(la)
		b.LocalGet(z)
		b.I32Load(4)
		b.LocalSet(lz)
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(la)
			b.Op(wasm.I32GeU)
			b.LocalGet(i)
			b.LocalGet(lz)
			b.Op(wasm.I32GeU, wasm.I32Or)
		}, func() {
			b.byteAt(a, i)
			b.LocalSet(ca)
			b.byteAt(z, i)
			b.LocalSet(cz)
			b.LocalGet(ca)
			b.LocalGet(cz)
			b.Op(wasm.I32Ne)
			b.returnIf(func() {
				b.LocalGet(ca)
				b.LocalGet(cz)
				b.Op(wasm.I32LtU)
				b.boolean()
			})
			b.incr(i)
		})
		b.LocalGet(la)
		b.LocalGet(lz)
		b.Op(wasm.I32LtU)
		b.boolean()
	case "symbol":
		s, n, r, zero := b.local(i32), b.local(i32), b.local(i32), b.local(i32)
		stringArg(0, s)
		b.LocalGet(s)
		b.I32Load(4)
		b.LocalTee(n)
		b.I32Const(tagSymbol)
		b.Call(c.rt.newString)
		b.LocalSet(r)
		b.copyBytes(r, s, zero, n)
		b.LocalGet(r)
		b.Op(wasm.I64ExtendI32U)
	case "parse_number":
		s, n, i, ch, acc, negative := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i64), b.local(i32)
		stringArg(0, s)
		b.LocalGet(s)
		b.I32Load(4)
		b.LocalSet(n)
		b.LocalGet(n)
		b.when(func() {
			b.byteAt(s, i)
			b.I32Const('-')
			b.Op(wasm.I32Eq)
			b.when(func() {
				b.I32Const(1)
				b.LocalSet(negative)
				b.I32Const(1)
				b.LocalSet(i)
			})
		})
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32Eq)
		b.returnIf(func() { b.I64Const(addrNil) })
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(n)
			b.Op(wasm.I32GeU)
		}, func() {
			b.byteAt(s, i)
			b.LocalTee(ch)
			b.I32Const('0')
			b.Op(wasm.I32LtU)
			b.LocalGet(ch)
			b.I32Const('9')
			b.Op(wasm.I32GtU, wasm.I32Or)
			b.returnIf(func() {
				if !c.floats { // parse_number isn't called
					b.I64Const(addrNil)
					return
				}
				b.LocalGet(s)
				b.Call(c.rt.parseFloat)
			})
			b.LocalGet(acc)
			b.I64Const(10)
			b.Op(wasm.I64Mul)
			b.LocalGet(ch)
			b.I32Const('0')
			b.Op(wasm.I32Sub, wasm.I64ExtendI32U, wasm.I64Add)
			b.LocalSet(acc)
			b.incr(i)
		})
		b.LocalGet(negative)
		b.when(func() {
			b.I64Const(0)
			b.LocalGet(acc)
			b.Op(wasm.I64Sub)
			b.LocalSet(acc)
		})
		b.LocalGet(acc)
		b.encode()
	default:
		panic("wasm: no builtin " + name)
	}
}
