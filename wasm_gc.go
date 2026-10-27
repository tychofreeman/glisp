package glisp

import (
	"sort"

	"github.com/tychofreeman/glisp/wasm"
)

// The garbage collector for compiled programs: a copying (Cheney)
// collector that then slides what survived back to the start of the heap,
// so the heap stays at one place in memory.
//
// It runs only at safe points: just before a call, once the heap has grown
// gcThreshold bytes past what survived the last collection. At a safe point
// every value the program holds is in memory, where the collector can find
// and update it: the argument stack (lambdas' frames, and the functions and
// arguments of calls being made) and the globals of top-level names.
// Nothing else holds a heap value across a call: the runtime's functions
// don't call back into the program, and allocating never collects.

// gcThreshold is how far the heap may grow past what survived the last
// collection before the next one. Tests lower it to collect often.
var gcThreshold = 8 << 20

// tagForward marks an object the collector has copied; its new address is
// at +4.
const tagForward = 10

// declareGC declares the collector's functions; gc itself is defined by
// defineGC, once the program's globals are known.
func (c *wasmCompiler) declareGC() {
	rt := &c.rt
	rt.objSize = c.declare(sig(vals(i32), i32))
	rt.gcCopy = c.declare(sig(vals(i64), i64))
	rt.gcReloc = c.declare(sig(vals(i64), i64))
	rt.gc = c.declare(sig(nil))
	c.define(rt.objSize, c.objSizeBody)
	c.define(rt.gcCopy, c.gcCopyBody)
	c.define(rt.gcReloc, c.gcRelocBody)
}

// safePoint collects if the heap has grown enough.
func (c *wasmCompiler) safePoint(b *wasmBody) {
	b.GlobalGet(gHP)
	b.GlobalGet(gGCNext)
	b.Op(wasm.I32GtU)
	b.when(func() {
		b.Call(c.rt.gc)
	})
}

// objSize(p) is the size in bytes of the object at p.
func (c *wasmCompiler) objSizeBody(b *wasmBody) {
	p, tag := b.param(0), b.local(i32)
	b.LocalGet(p)
	b.I32Load(0)
	b.LocalSet(tag)
	for _, size := range []struct{ tag, size int32 }{{tagPair, 24}, {tagCell, 16}} {
		b.LocalGet(tag)
		b.I32Const(size.tag)
		b.Op(wasm.I32Eq)
		b.returnIf(func() { b.I32Const(size.size) })
	}
	b.LocalGet(tag)
	b.I32Const(tagClosure)
	b.Op(wasm.I32Eq)
	b.returnIf(func() {
		b.LocalGet(p)
		b.I32Load(8)
		b.I32Const(3)
		b.Op(wasm.I32Shl)
		b.I32Const(16)
		b.Op(wasm.I32Add)
	})
	// A string or symbol.
	b.LocalGet(p)
	b.I32Load(4)
	b.I32Const(15)
	b.Op(wasm.I32Add)
	b.I32Const(-8)
	b.Op(wasm.I32And)
}

// gcCopy(v) is v, copied to to-space if it is an object in from-space: the
// copy's address, which a forward left in the old object gives to any
// later reference.
func (c *wasmCompiler) gcCopyBody(b *wasmBody) {
	v, p, n, dst := b.param(0), b.local(i32), b.local(i32), b.local(i32)
	b.LocalGet(v)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I32WrapI64)
	b.returnIf(func() { b.LocalGet(v) })
	b.LocalGet(v)
	b.Op(wasm.I32WrapI64)
	b.LocalTee(p)
	b.GlobalGet(gHeapStart)
	b.Op(wasm.I32LtU)
	b.LocalGet(p)
	b.GlobalGet(gFromEnd)
	b.Op(wasm.I32GeU, wasm.I32Or)
	b.returnIf(func() { b.LocalGet(v) })
	b.LocalGet(p)
	b.I32Load(0)
	b.I32Const(tagForward)
	b.Op(wasm.I32Eq)
	b.returnIf(func() {
		b.LocalGet(p)
		b.I32Load(4)
		b.Op(wasm.I64ExtendI32U)
	})
	b.LocalGet(p)
	b.Call(c.rt.objSize)
	b.LocalSet(n)
	b.GlobalGet(gToNext)
	b.LocalTee(dst)
	b.LocalGet(p)
	b.LocalGet(n)
	b.MemoryCopy()
	b.LocalGet(dst)
	b.LocalGet(n)
	b.Op(wasm.I32Add)
	b.GlobalSet(gToNext)
	b.LocalGet(p)
	b.I32Const(tagForward)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(dst)
	b.I32Store(4)
	b.LocalGet(dst)
	b.Op(wasm.I64ExtendI32U)
}

// gcReloc(v) is v, moved down by gDelta if it points into to-space: where
// it will be once to-space slides to the start of the heap.
func (c *wasmCompiler) gcRelocBody(b *wasmBody) {
	v, p := b.param(0), b.local(i32)
	b.LocalGet(v)
	b.I64Const(1)
	b.Op(wasm.I64And, wasm.I32WrapI64)
	b.returnIf(func() { b.LocalGet(v) })
	b.LocalGet(v)
	b.Op(wasm.I32WrapI64)
	b.LocalTee(p)
	b.GlobalGet(gToStart)
	b.Op(wasm.I32GeU)
	b.LocalGet(p)
	b.GlobalGet(gToNext)
	b.Op(wasm.I32LtU, wasm.I32And)
	b.returnIf(func() {
		b.LocalGet(p)
		b.GlobalGet(gDelta)
		b.Op(wasm.I32Sub, wasm.I64ExtendI32U)
	})
	b.LocalGet(v)
}

// defineGC defines gc, which updates the program's globals.
func (c *wasmCompiler) defineGC() {
	c.define(c.rt.gc, c.gcBody)
}

// gcBody is gc: it collects, then updates the program's globals.
func (c *wasmCompiler) gcBody(b *wasmBody) {
	p, scan, k, end := b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	var globals []uint32
	for _, g := range c.globals {
		globals = append(globals, g)
	}
	sort.Slice(globals, func(i, j int) bool { return globals[i] < globals[j] })
	// Each root, and each value field of each object in to-space from
	// gToStart up to gToNext (which grows while copying), goes through fn.
	roots := func(fn uint32) {
		b.GlobalGet(gStackBase)
		b.LocalSet(p)
		b.until(func() {
			b.LocalGet(p)
			b.GlobalGet(gSP)
			b.Op(wasm.I32GeU)
		}, func() {
			b.LocalGet(p)
			b.LocalGet(p)
			b.I64Load(0)
			b.Call(fn)
			b.I64Store(0)
			b.LocalGet(p)
			b.I32Const(8)
			b.Op(wasm.I32Add)
			b.LocalSet(p)
		})
		for _, g := range globals {
			b.GlobalGet(g)
			b.Call(fn)
			b.GlobalSet(g)
		}
	}
	field := func(fn uint32, offset uint32) {
		b.LocalGet(scan)
		b.LocalGet(scan)
		b.I64Load(offset)
		b.Call(fn)
		b.I64Store(offset)
	}
	tagIs := func(tag int32) {
		b.LocalGet(scan)
		b.I32Load(0)
		b.I32Const(tag)
		b.Op(wasm.I32Eq)
	}
	objects := func(fn uint32) {
		b.GlobalGet(gToStart)
		b.LocalSet(scan)
		b.until(func() {
			b.LocalGet(scan)
			b.GlobalGet(gToNext)
			b.Op(wasm.I32GeU)
		}, func() {
			tagIs(tagPair)
			b.when(func() {
				field(fn, 8)
				field(fn, 16)
			})
			tagIs(tagCell)
			b.when(func() {
				field(fn, 8)
			})
			tagIs(tagClosure)
			b.when(func() {
				b.I32Const(0)
				b.LocalSet(k)
				b.until(func() {
					b.LocalGet(k)
					b.LocalGet(scan)
					b.I32Load(8)
					b.Op(wasm.I32GeU)
				}, func() {
					b.LocalGet(scan)
					b.LocalGet(k)
					b.I32Const(3)
					b.Op(wasm.I32Shl, wasm.I32Add)
					b.LocalTee(p)
					b.LocalGet(p)
					b.I64Load(16)
					b.Call(fn)
					b.I64Store(16)
					b.incr(k)
				})
			})
			b.LocalGet(scan)
			b.LocalGet(scan)
			b.Call(c.rt.objSize)
			b.Op(wasm.I32Add)
			b.LocalSet(scan)
		})
	}

	// To-space starts after the heap, with room for all of it to survive.
	b.GlobalGet(gHP)
	b.GlobalSet(gFromEnd)
	b.GlobalGet(gHP)
	b.I32Const(7)
	b.Op(wasm.I32Add)
	b.I32Const(-8)
	b.Op(wasm.I32And)
	b.GlobalSet(gToStart)
	b.GlobalGet(gToStart)
	b.GlobalSet(gToNext)
	b.GlobalGet(gToStart)
	b.GlobalGet(gFromEnd)
	b.GlobalGet(gHeapStart)
	b.Op(wasm.I32Sub, wasm.I32Add)
	b.LocalSet(end)
	b.LocalGet(end)
	b.MemorySize()
	b.I32Const(16)
	b.Op(wasm.I32Shl, wasm.I32GtU)
	b.when(func() {
		b.LocalGet(end)
		b.MemorySize()
		b.I32Const(16)
		b.Op(wasm.I32Shl, wasm.I32Sub)
		b.I32Const(65535)
		b.Op(wasm.I32Add)
		b.I32Const(16)
		b.Op(wasm.I32ShrU)
		b.LocalSet(p)
		// Grow as alloc does: by the larger of what's needed and the memory's
		// size, or by what's needed if that much isn't available.
		b.MemorySize()
		b.LocalGet(p)
		b.MemorySize()
		b.LocalGet(p)
		b.Op(wasm.I32GtU, wasm.Select)
		b.MemoryGrow()
		b.I32Const(-1)
		b.Op(wasm.I32Eq)
		b.when(func() {
			b.LocalGet(p)
			b.MemoryGrow()
			b.I32Const(-1)
			b.Op(wasm.I32Eq)
			b.when(func() {
				c.fail(b, "out of memory", nil)
			})
		})
	})

	// Copy what the roots reach, then what the copies reach.
	roots(c.rt.gcCopy)
	objects(c.rt.gcCopy)

	// Point everything at where to-space is about to be, and slide it there.
	b.GlobalGet(gToStart)
	b.GlobalGet(gHeapStart)
	b.Op(wasm.I32Sub)
	b.GlobalSet(gDelta)
	roots(c.rt.gcReloc)
	objects(c.rt.gcReloc)
	b.GlobalGet(gHeapStart)
	b.GlobalGet(gToStart)
	b.GlobalGet(gToNext)
	b.GlobalGet(gToStart)
	b.Op(wasm.I32Sub)
	b.LocalTee(end)
	b.MemoryCopy()
	b.GlobalGet(gHeapStart)
	b.LocalGet(end)
	b.Op(wasm.I32Add)
	b.GlobalSet(gHP)

	// Collect again once the heap grows by the larger of what survived and
	// the threshold.
	b.GlobalGet(gHP)
	b.LocalGet(end)
	b.I32Const(int32(gcThreshold))
	b.LocalGet(end)
	b.I32Const(int32(gcThreshold))
	b.Op(wasm.I32GtU, wasm.Select, wasm.I32Add)
	b.GlobalSet(gGCNext)
}
