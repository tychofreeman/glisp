package glisp

import "github.com/tychofreeman/glisp/wasm"

// Hashing in compiled code: the encoding hash.go describes, fed to the same
// FNV-1a.

// hashIntoBody is hashInto(v, h, name): the hash h with v's encoding fed
// in. name, a string, names the builtin for the error a function gives.
func (c *wasmCompiler) hashIntoBody(b *wasmBody) {
	v, h, name := b.param(0), b.param(1), b.param(2)
	t, p, n, i := b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	put := func(value func()) {
		b.LocalGet(h)
		value()
		b.Op(wasm.I32Xor)
		b.I32Const(int32(fnvPrime))
		b.Op(wasm.I32Mul)
		b.LocalSet(h)
	}
	putByte := func(x byte) { put(func() { b.I32Const(int32(x)) }) }
	// putBytes feeds the low k bytes of the i64 value leaves, least first.
	putBytes := func(k int, value func()) {
		for j := 0; j < k; j++ {
			put(func() {
				value()
				b.I64Const(int64(8 * j))
				b.Op(wasm.I64ShrU, wasm.I32WrapI64)
				b.I32Const(255)
				b.Op(wasm.I32And)
			})
		}
	}
	// Each kind feeds its encoding, then returns h.
	is := func(tag int32, feed func()) {
		b.LocalGet(t)
		b.I32Const(tag)
		b.Op(wasm.I32Eq)
		b.returnIf(func() {
			feed()
			b.LocalGet(h)
		})
	}
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.LocalSet(t)
	is(tagInt, func() {
		putByte('i')
		putBytes(8, func() {
			b.LocalGet(v)
			b.decode()
		})
	})
	if c.made["float"] {
		is(tagFloat, func() {
			// The bits, with one zero and one NaN, as floatBits gives them.
			putByte('d')
			putBytes(8, func() {
				b.field(v, 8)
				b.asF64()
				b.f64Const(0)
				b.Op(wasm.F64Eq)
				b.ifElse(wasm.BlockI64, func() { b.I64Const(0) }, func() {
					b.field(v, 8)
					b.asF64()
					b.field(v, 8)
					b.asF64()
					b.Op(wasm.F64Ne)
					b.ifElse(wasm.BlockI64, func() { b.I64Const(0x7ff8000000000000) }, func() { b.field(v, 8) })
				})
			})
		})
	}
	is(tagNil, func() { putByte('n') })
	is(tagTrue, func() { putByte('t') })
	is(tagFalse, func() { putByte('f') })
	text := func(tag byte) func() {
		return func() {
			putByte(tag)
			b.LocalGet(v)
			b.Op(wasm.I32WrapI64)
			b.LocalTee(p)
			b.I32Load(4)
			b.LocalSet(n)
			putBytes(4, func() {
				b.LocalGet(n)
				b.Op(wasm.I64ExtendI32U)
			})
			b.until(func() {
				b.LocalGet(i)
				b.LocalGet(n)
				b.Op(wasm.I32GeU)
			}, func() {
				put(func() { b.byteAt(p, i) })
				b.incr(i)
			})
		}
	}
	is(tagString, text('s'))
	is(tagSymbol, text('y'))
	is(tagEmpty, func() {
		putByte('(')
		putByte(')')
	})
	is(tagPair, func() {
		putByte('(')
		b.until(func() {
			c.tagIs(b, v, tagPair)
			b.Op(wasm.I32Eqz)
		}, func() {
			b.field(v, 8)
			b.LocalGet(h)
			b.LocalGet(name)
			b.Call(c.rt.hashInto)
			b.LocalSet(h)
			b.field(v, 16)
			b.LocalSet(v)
		})
		putByte(')')
	})
	is(tagVector, func() {
		putByte('[')
		c.hashElements(b, v, p, n, i, h, name)
		putByte(']')
	})
	is(tagData, func() {
		putByte('#')
		c.hashElements(b, v, p, n, i, h, name)
		putByte(')')
	})
	c.fail(b, "", func() {
		b.LocalGet(name)
		b.Call(c.rt.putStr)
		c.text(b, ": can't hash ")
		c.typeAndValue(b, v)
	})
}

// hashElements feeds each value of the vector-like object v into the hash
// in h.
func (c *wasmCompiler) hashElements(b *wasmBody, v uint32, p uint32, n uint32, i uint32, h uint32, name uint32) {
	b.LocalGet(v)
	b.Op(wasm.I32WrapI64)
	b.LocalTee(p)
	b.I32Load(4)
	b.LocalSet(n)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(n)
		b.Op(wasm.I32GeU)
	}, func() {
		element(b, p, func() { b.LocalGet(i) })
		b.I64Load(8)
		b.LocalGet(h)
		b.LocalGet(name)
		b.Call(c.rt.hashInto)
		b.LocalSet(h)
		b.incr(i)
	})
}
