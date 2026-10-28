package glisp

import (
	"math"

	"github.com/tychofreeman/glisp/wasm"
)

// Floats in compiled code: an object tagged tagFloat with the float's 64
// bits at +8. Code keeps a float as those bits, an i64, and reinterprets
// them as an f64 for each operation, so no local is an f64.

// Printing a float exactly needs integers far wider than 64 bits. The
// scratch memory (at gScratch) holds five bignums, each bnLimbs 32-bit
// limbs, least significant first, then the digits printed.
const (
	bnLimbs     = 40 // 1280 bits: printing needs about 1140
	bnSize      = 4 * bnLimbs
	scratchSize = 5*bnSize + 24
)

func (c *wasmCompiler) declareFloats() {
	rt := &c.rt
	rt.boxFloat = c.declare(sig(vals(i64), i64))
	rt.floatBits = c.declare(sig(vals(i64), i64))
	rt.bnSet = c.declare(sig(vals(i32, i64)))
	rt.bnMul = c.declare(sig(vals(i32, i32)))
	rt.bnShl = c.declare(sig(vals(i32, i32)))
	rt.bnAdd = c.declare(sig(vals(i32, i32, i32)))
	rt.bnSub = c.declare(sig(vals(i32, i32)))
	rt.bnCmp = c.declare(sig(vals(i32, i32), i32))
	rt.putFloat = c.declare(sig(vals(i64)))
}

func (c *wasmCompiler) defineFloats() {
	rt := &c.rt
	c.define(rt.boxFloat, c.boxFloatBody)
	c.define(rt.floatBits, c.floatBitsBody)
	c.define(rt.bnSet, bnSetBody)
	c.define(rt.bnMul, bnMulBody)
	c.define(rt.bnShl, c.bnShlBody)
	c.define(rt.bnAdd, bnAddBody)
	c.define(rt.bnSub, bnSubBody)
	c.define(rt.bnCmp, bnCmpBody)
	c.define(rt.putFloat, c.putFloatBody)
}

// floatOps compile the float builtins. Their arguments have been checked
// against the spec already.
var floatOps = map[string]func(c *wasmCompiler, b *wasmBody){
	"float":    (*wasmCompiler).compileFloat,
	"floor":    compileRounding("floor", wasm.F64Floor),
	"ceiling":  compileRounding("ceiling", wasm.F64Ceil),
	"truncate": compileRounding("truncate", wasm.F64Trunc),
	"round":    compileRounding("round", wasm.F64Nearest),
	"sqrt":     (*wasmCompiler).compileSqrt,
}

// asF64 turns the bits on the stack into the f64 they are; asBits back.
func (b *wasmBody) asF64()  { b.Op(wasm.F64ReinterpretI64) }
func (b *wasmBody) asBits() { b.Op(wasm.I64ReinterpretF64) }

// f64Const leaves the f64 f.
func (b *wasmBody) f64Const(f float64) {
	b.I64Const(int64(math.Float64bits(f)))
	b.asF64()
}

// isFloat leaves whether the value on the stack is a float.
func (c *wasmCompiler) isFloat(b *wasmBody) {
	b.Call(c.rt.tagOf)
	b.I32Const(tagFloat)
	b.Op(wasm.I32Eq)
}

// argAsF64 leaves argument i, a number, as an f64.
func (c *wasmCompiler) argAsF64(b *wasmBody, i uint32) {
	b.argAt(i)
	b.Call(c.rt.floatBits)
	b.asF64()
}

// anyFloatArg leaves whether any of the call's arguments is a float.
func (c *wasmCompiler) anyFloatArg(b *wasmBody) {
	argc := b.param(paramArgc)
	i, found := b.local(i32), b.local(i32)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(argc)
		b.Op(wasm.I32GeU)
	}, func() {
		b.argAt(i)
		c.isFloat(b)
		b.LocalGet(found)
		b.Op(wasm.I32Or)
		b.LocalSet(found)
		b.incr(i)
	})
	b.LocalGet(found)
}

// floatArithmetic returns from the builtin name (plus, - or *) the float
// it gives if any argument is a float, every argument taken as a float.
func (c *wasmCompiler) floatArithmetic(b *wasmBody, name string) {
	argc := b.param(paramArgc)
	i, acc := b.local(i32), b.local(i64)
	op := map[string]byte{"plus": wasm.F64Add, "-": wasm.F64Sub, "*": wasm.F64Mul}[name]
	c.anyFloatArg(b)
	b.when(func() {
		switch name {
		case "plus":
			b.f64Const(0)
		case "*":
			b.f64Const(1)
		case "-":
			c.argAsF64(b, i)
		}
		b.asBits()
		b.LocalSet(acc)
		if name == "-" {
			b.LocalGet(argc)
			b.I32Const(1)
			b.Op(wasm.I32Eq)
			b.returnIf(func() {
				b.LocalGet(acc)
				b.asF64()
				b.Op(wasm.F64Neg)
				b.asBits()
				b.Call(c.rt.boxFloat)
			})
			b.incr(i)
		}
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(argc)
			b.Op(wasm.I32GeU)
		}, func() {
			b.LocalGet(acc)
			b.asF64()
			c.argAsF64(b, i)
			b.Op(op)
			b.asBits()
			b.LocalSet(acc)
			b.incr(i)
		})
		b.LocalGet(acc)
		b.Call(c.rt.boxFloat)
		b.Op(wasm.Return)
	})
}

// floatBinary returns from a builtin of two numbers what ops gives, from
// the two as f64s, if either is a float.
func (c *wasmCompiler) floatBinary(b *wasmBody, ops func()) {
	b.arg(0)
	c.isFloat(b)
	b.arg(1)
	c.isFloat(b)
	b.Op(wasm.I32Or)
	b.returnIf(func() {
		for i := range uint32(2) {
			b.arg(i)
			b.Call(c.rt.floatBits)
			b.asF64()
		}
		ops()
	})
}

// (float n): n's float.
func (c *wasmCompiler) compileFloat(b *wasmBody) {
	b.arg(0)
	b.Call(c.rt.floatBits)
	b.Call(c.rt.boxFloat)
}

func (c *wasmCompiler) compileSqrt(b *wasmBody) {
	b.arg(0)
	b.Call(c.rt.floatBits)
	b.asF64()
	b.Op(wasm.F64Sqrt)
	b.asBits()
	b.Call(c.rt.boxFloat)
}

// compileRounding compiles (name n): an integer is itself, and a float is op of it,
// which must be in the integers' range.
func compileRounding(name string, op byte) func(c *wasmCompiler, b *wasmBody) {
	return func(c *wasmCompiler, b *wasmBody) {
		r := b.local(i64)
		b.arg(0)
		b.I64Const(1)
		b.Op(wasm.I64And, wasm.I32WrapI64)
		b.returnIf(func() { b.arg(0) })
		b.arg(0)
		b.Call(c.rt.floatBits)
		b.asF64()
		b.Op(op)
		b.asBits()
		b.LocalTee(r)
		b.asF64()
		b.f64Const(-(1 << 62))
		b.Op(wasm.F64Ge)
		b.LocalGet(r)
		b.asF64()
		b.f64Const(1 << 62)
		b.Op(wasm.F64Lt, wasm.I32And, wasm.I32Eqz)
		c.failIf(b, name+": ", func() {
			b.arg(0)
			b.Call(c.rt.putValue)
			c.text(b, " is out of range for an integer")
		})
		b.LocalGet(r)
		b.asF64()
		b.Op(wasm.I64TruncF64S)
		b.encode()
	}
}

// boxFloat(bits) is a new float holding bits.
func (c *wasmCompiler) boxFloatBody(b *wasmBody) {
	bits, p := b.param(0), b.local(i32)
	b.I32Const(16)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.I32Const(tagFloat)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(bits)
	b.I64Store(8)
	b.LocalGet(p)
	b.Op(wasm.I64ExtendI32U)
}

// floatBits(n) is the bits of the number n as a float: an integer is
// converted.
func (c *wasmCompiler) floatBitsBody(b *wasmBody) {
	v := b.param(0)
	b.LocalGet(v)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I32WrapI64)
	b.ifElse(wasm.BlockI64, func() {
		b.LocalGet(v)
		b.decode()
		b.Op(wasm.F64ConvertI64S)
		b.asBits()
	}, func() {
		b.field(v, 8)
	})
}

// limb leaves the address of limb i (a local) of the bignum at local x.
func limb(b *wasmBody, x, i uint32) {
	b.LocalGet(x)
	b.LocalGet(i)
	b.I32Const(2)
	b.Op(wasm.I32Shl, wasm.I32Add)
}

// limbValue leaves limb i of the bignum at x, as an i64.
func limbValue(b *wasmBody, x, i uint32) {
	limb(b, x, i)
	b.I32Load(0)
	b.Op(wasm.I64ExtendI32U)
}

// eachLimb runs body for each limb index i, from 0 up.
func eachLimb(b *wasmBody, i uint32, body func()) {
	b.until(func() {
		b.LocalGet(i)
		b.I32Const(bnLimbs)
		b.Op(wasm.I32GeU)
	}, func() {
		body()
		b.incr(i)
	})
}

// bnSet(x, v): x = v.
func bnSetBody(b *wasmBody) {
	x, v, i := b.param(0), b.param(1), b.local(i32)
	eachLimb(b, i, func() {
		limb(b, x, i)
		b.I32Const(0)
		b.I32Store(0)
	})
	b.LocalGet(x)
	b.LocalGet(v)
	b.I64Store(0)
}

// bnMul(x, m): x = x * m, for a 32-bit m.
func bnMulBody(b *wasmBody) {
	x, m, i, carry := b.param(0), b.param(1), b.local(i32), b.local(i64)
	eachLimb(b, i, func() {
		limb(b, x, i)
		limbValue(b, x, i)
		b.LocalGet(m)
		b.Op(wasm.I64ExtendI32U, wasm.I64Mul)
		b.LocalGet(carry)
		b.Op(wasm.I64Add)
		b.LocalTee(carry)
		b.Op(wasm.I32WrapI64)
		b.I32Store(0)
		b.LocalGet(carry)
		b.I64Const(32)
		b.Op(wasm.I64ShrU)
		b.LocalSet(carry)
	})
}

// bnShl(x, n): x = x * 2^n.
func (c *wasmCompiler) bnShlBody(b *wasmBody) {
	x, n := b.param(0), b.param(1)
	b.until(func() {
		b.LocalGet(n)
		b.I32Const(16)
		b.Op(wasm.I32LtU)
	}, func() {
		b.LocalGet(x)
		b.I32Const(1 << 16)
		b.Call(c.rt.bnMul)
		b.LocalGet(n)
		b.I32Const(16)
		b.Op(wasm.I32Sub)
		b.LocalSet(n)
	})
	b.LocalGet(x)
	b.I32Const(1)
	b.LocalGet(n)
	b.Op(wasm.I32Shl)
	b.Call(c.rt.bnMul)
}

// bnAdd(dst, x, y): dst = x + y.
func bnAddBody(b *wasmBody) {
	dst, x, y, i, carry := b.param(0), b.param(1), b.param(2), b.local(i32), b.local(i64)
	eachLimb(b, i, func() {
		limb(b, dst, i)
		limbValue(b, x, i)
		limbValue(b, y, i)
		b.Op(wasm.I64Add)
		b.LocalGet(carry)
		b.Op(wasm.I64Add)
		b.LocalTee(carry)
		b.Op(wasm.I32WrapI64)
		b.I32Store(0)
		b.LocalGet(carry)
		b.I64Const(32)
		b.Op(wasm.I64ShrU)
		b.LocalSet(carry)
	})
}

// bnSub(x, y): x = x - y, for y no greater than x.
func bnSubBody(b *wasmBody) {
	x, y, i, borrow := b.param(0), b.param(1), b.local(i32), b.local(i64)
	eachLimb(b, i, func() {
		limb(b, x, i)
		limbValue(b, x, i)
		limbValue(b, y, i)
		b.Op(wasm.I64Sub)
		b.LocalGet(borrow)
		b.Op(wasm.I64Sub)
		b.LocalTee(borrow)
		b.Op(wasm.I32WrapI64)
		b.I32Store(0)
		b.LocalGet(borrow)
		b.I64Const(63)
		b.Op(wasm.I64ShrU)
		b.LocalSet(borrow)
	})
}

// bnCmp(x, y) is -1, 0 or 1 as x is less than, equal to or greater than y.
func bnCmpBody(b *wasmBody) {
	x, y, i := b.param(0), b.param(1), b.local(i32)
	b.I32Const(bnLimbs)
	b.LocalSet(i)
	b.until(func() {
		b.LocalGet(i)
		b.Op(wasm.I32Eqz)
	}, func() {
		b.LocalGet(i)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.LocalSet(i)
		limbValue(b, x, i)
		limbValue(b, y, i)
		b.Op(wasm.I64Ne)
		b.returnIf(func() {
			b.I32Const(-1)
			b.I32Const(1)
			limbValue(b, x, i)
			limbValue(b, y, i)
			b.Op(wasm.I64LtU, wasm.Select)
		})
	})
	b.I32Const(0)
}

// putFloat(bits) prints the float as formatFloat does. Its digits are the
// shortest that read back as the float, by Burger and Dybvig's free-format
// algorithm ("Printing Floating-Point Numbers Quickly and Accurately",
// 1996) on bignums: v lies strictly between its neighbours' midpoints
// (r - m-)/s and (r + m+)/s, and digits are generated from r/s until the
// number they make is within that interval, reached on even mantissas.
func (c *wasmCompiler) putFloatBody(b *wasmBody) {
	bits := b.param(0)
	exp, f, e, even, unequal := b.local(i32), b.local(i64), b.local(i32), b.local(i32), b.local(i32)
	k, n, d, cmp, low, high, done := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	putByte := func(ch byte) {
		b.I32Const(int32(ch))
		b.Call(c.rt.putByte)
	}
	at := func(offset int32) func() {
		return func() {
			b.GlobalGet(gScratch)
			b.I32Const(offset)
			b.Op(wasm.I32Add)
		}
	}
	R, S, MP, MM, T, D := at(0), at(bnSize), at(2*bnSize), at(3*bnSize), at(4*bnSize), at(5*bnSize)
	set := func(x func(), v int64) {
		x()
		b.I64Const(v)
		b.Call(c.rt.bnSet)
	}
	shl := func(x func(), by ...uint32) { // by the sum of the i32 locals in by
		x()
		b.I32Const(0)
		for _, l := range by {
			b.LocalGet(l)
			b.Op(wasm.I32Add)
		}
		b.Call(c.rt.bnShl)
	}
	times10 := func(xs ...func()) {
		for _, x := range xs {
			x()
			b.I32Const(10)
			b.Call(c.rt.bnMul)
		}
	}
	compare := func(x, y func()) {
		x()
		y()
		b.Call(c.rt.bnCmp)
	}
	// highEnough leaves whether T reaches S: T >= S on an even mantissa,
	// whose interval includes its ends, and T > S otherwise.
	highEnough := func() {
		compare(T, S)
		b.LocalGet(even)
		b.Op(wasm.I32Add)
		b.I32Const(0)
		b.Op(wasm.I32GtS)
	}
	sumRM := func() {
		T()
		R()
		MP()
		b.Call(c.rt.bnAdd)
	}

	b.LocalGet(bits)
	b.asF64()
	b.LocalGet(bits)
	b.asF64()
	b.Op(wasm.F64Ne)
	b.returnIf(func() { c.text(b, "NaN") })
	b.LocalGet(bits)
	b.I64Const(0)
	b.Op(wasm.I64LtS)
	b.when(func() {
		putByte('-')
		b.LocalGet(bits)
		b.I64Const(math.MaxInt64)
		b.Op(wasm.I64And)
		b.LocalSet(bits)
	})
	b.LocalGet(bits)
	b.I64Const(52)
	b.Op(wasm.I64ShrU, wasm.I32WrapI64)
	b.LocalTee(exp)
	b.I32Const(0x7ff)
	b.Op(wasm.I32Eq)
	b.returnIf(func() { c.text(b, "Infinity") })
	b.LocalGet(bits)
	b.Op(wasm.I64Eqz)
	b.returnIf(func() { c.text(b, "0.0") })

	// v = f * 2^e, with f a 53-bit integer but for subnormals.
	b.LocalGet(bits)
	b.I64Const(1<<52 - 1)
	b.Op(wasm.I64And)
	b.LocalSet(f)
	b.LocalGet(exp)
	b.Op(wasm.I32Eqz)
	b.ifElse(wasm.BlockVoid, func() {
		b.I32Const(-1074)
		b.LocalSet(e)
	}, func() {
		b.LocalGet(f)
		b.I64Const(1 << 52)
		b.Op(wasm.I64Or)
		b.LocalSet(f)
		b.LocalGet(exp)
		b.I32Const(1075)
		b.Op(wasm.I32Sub)
		b.LocalSet(e)
	})
	b.LocalGet(f)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I64Eqz)
	b.LocalSet(even)
	// The gap below v is half the gap above when f is a power of two (and v
	// isn't the least normal float).
	b.LocalGet(f)
	b.I64Const(1 << 52)
	b.Op(wasm.I64Eq)
	b.LocalGet(e)
	b.I32Const(-1074)
	b.Op(wasm.I32GtS, wasm.I32And)
	b.LocalSet(unequal)

	// v = r/s, and the midpoints to its neighbours are m+/s above and m-/s
	// below, all scaled by 2 (by 4 when unequal).
	b.LocalGet(f)
	set(R, 0)
	R()
	b.LocalGet(f)
	b.Call(c.rt.bnSet)
	set(S, 1)
	set(MP, 1)
	set(MM, 1)
	one := b.local(i32)
	b.I32Const(1)
	b.LocalSet(one)
	b.LocalGet(e)
	b.I32Const(0)
	b.Op(wasm.I32GeS)
	b.ifElse(wasm.BlockVoid, func() {
		shl(R, e, one, unequal)
		shl(S, one, unequal)
		shl(MP, e, unequal)
		shl(MM, e)
	}, func() {
		minusE := b.local(i32)
		b.I32Const(0)
		b.LocalGet(e)
		b.Op(wasm.I32Sub)
		b.LocalSet(minusE)
		shl(R, one, unequal)
		shl(S, one, minusE, unequal)
		shl(MP, unequal)
	})
	b.Op(wasm.Drop) // the f left above

	// Scale by 10^k so that the first digit generated is not 0:
	// (r + m+)/s is below 1, and at least 1/10.
	b.until(func() {
		sumRM()
		highEnough()
		b.Op(wasm.I32Eqz)
	}, func() {
		times10(S)
		b.incr(k)
	})
	b.until(func() {
		sumRM()
		times10(T)
		highEnough()
	}, func() {
		times10(R, MP, MM)
		b.LocalGet(k)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.LocalSet(k)
	})

	// Generate digits until the number they make is within the interval.
	b.until(func() { b.LocalGet(done) }, func() {
		times10(R, MP, MM)
		b.I32Const(0)
		b.LocalSet(d)
		b.until(func() {
			compare(R, S)
			b.I32Const(0)
			b.Op(wasm.I32LtS)
		}, func() {
			R()
			S()
			b.Call(c.rt.bnSub)
			b.incr(d)
		})
		// low: r < m- (or <= on an even mantissa); high: r + m+ > s (or >=).
		compare(R, MM)
		b.LocalGet(even)
		b.Op(wasm.I32Sub)
		b.I32Const(0)
		b.Op(wasm.I32LtS)
		b.LocalSet(low)
		sumRM()
		highEnough()
		b.LocalSet(high)
		b.LocalGet(low)
		b.LocalGet(high)
		b.Op(wasm.I32Or)
		b.LocalSet(done)
		b.LocalGet(low)
		b.LocalGet(high)
		b.Op(wasm.I32And)
		b.ifElse(wasm.BlockVoid, func() {
			// Both d and d+1 are in the interval: the nearer, and the even
			// one when v is halfway.
			T()
			R()
			R()
			b.Call(c.rt.bnAdd)
			compare(T, S)
			b.LocalTee(cmp)
			b.I32Const(0)
			b.Op(wasm.I32GtS)
			b.LocalGet(cmp)
			b.Op(wasm.I32Eqz)
			b.LocalGet(d)
			b.I32Const(1)
			b.Op(wasm.I32And, wasm.I32And, wasm.I32Or)
			b.LocalGet(d)
			b.Op(wasm.I32Add)
			b.LocalSet(d)
		}, func() {
			b.LocalGet(d)
			b.LocalGet(high)
			b.Op(wasm.I32Add)
			b.LocalSet(d)
		})
		D()
		b.LocalGet(n)
		b.Op(wasm.I32Add)
		b.LocalGet(d)
		b.I32Const('0')
		b.Op(wasm.I32Add)
		b.I32Store8(0)
		b.incr(n)
	})

	// v is 0.digits times 10^k: print it as formatFloat does.
	putDigits := func(from, count func()) {
		D()
		from()
		b.Op(wasm.I32Add)
		count()
		b.Call(c.rt.putBytes)
	}
	constant := func(v int32) func() { return func() { b.I32Const(v) } }
	local := func(l uint32) func() { return func() { b.LocalGet(l) } }
	b.LocalGet(k)
	b.I32Const(0)
	b.Op(wasm.I32LtS)
	b.LocalGet(k)
	b.I32Const(7)
	b.Op(wasm.I32GtS, wasm.I32Or)
	b.ifElse(wasm.BlockVoid, func() {
		putDigits(constant(0), constant(1))
		putByte('.')
		b.LocalGet(n)
		b.I32Const(1)
		b.Op(wasm.I32Eq)
		b.ifElse(wasm.BlockVoid, func() { putByte('0') }, func() {
			putDigits(constant(1), func() {
				b.LocalGet(n)
				b.I32Const(1)
				b.Op(wasm.I32Sub)
			})
		})
		putByte('e')
		b.LocalGet(k)
		b.I32Const(1)
		b.Op(wasm.I32Sub, wasm.I64ExtendI32S)
		b.Call(c.rt.putInt)
	}, func() {
		// The integer part: k digits, padded with zeros, or 0.
		b.LocalGet(k)
		b.Op(wasm.I32Eqz)
		b.ifElse(wasm.BlockVoid, func() { putByte('0') }, func() {
			b.LocalGet(n)
			b.LocalGet(k)
			b.Op(wasm.I32GeU)
			b.ifElse(wasm.BlockVoid, func() {
				putDigits(constant(0), local(k))
			}, func() {
				putDigits(constant(0), local(n))
				b.until(func() {
					b.LocalGet(n)
					b.LocalGet(k)
					b.Op(wasm.I32GeU)
				}, func() {
					putByte('0')
					b.incr(n)
				})
			})
		})
		putByte('.')
		b.LocalGet(n)
		b.LocalGet(k)
		b.Op(wasm.I32GtU)
		b.ifElse(wasm.BlockVoid, func() {
			putDigits(local(k), func() {
				b.LocalGet(n)
				b.LocalGet(k)
				b.Op(wasm.I32Sub)
			})
		}, func() { putByte('0') })
	})
}
