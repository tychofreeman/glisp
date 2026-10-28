package glisp

import "github.com/tychofreeman/glisp/wasm"

// Vectors in compiled code: an object tagged tagVector, its length at +4 and
// its values from +8 (tagKinds has the layout, so the collector knows it).

// vectorOps compile the vector builtins. Their arguments have been checked
// against the spec already.
var vectorOps = map[string]func(c *wasmCompiler, b *wasmBody){
	"vector":         (*wasmCompiler).compileVector,
	"vector_ref":     (*wasmCompiler).compileVectorRef,
	"vector_length":  (*wasmCompiler).compileVectorLength,
	"vector_set":     (*wasmCompiler).compileVectorSet,
	"vector_to_list": (*wasmCompiler).compileVectorToList,
}

// newVector leaves the address of a new vector whose length is in local n,
// its values not yet set.
func (c *wasmCompiler) newVector(b *wasmBody, tag int32, n uint32) {
	p := b.local(i32)
	b.LocalGet(n)
	b.I32Const(3)
	b.Op(wasm.I32Shl)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.I32Const(tag)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(n)
	b.I32Store(4)
	b.LocalGet(p)
}

// vectorArg leaves argument 0, a vector, in a new local as its address.
func vectorArg(b *wasmBody) uint32 {
	v := b.local(i32)
	b.arg(0)
	b.Op(wasm.I32WrapI64)
	b.LocalSet(v)
	return v
}

// element leaves the address of the value of the vector in local v whose
// index index leaves (an i32), less 8: load and store it at offset 8.
func element(b *wasmBody, v uint32, index func()) {
	b.LocalGet(v)
	index()
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
}

// vectorIndex returns a local holding argument 1 as an index into the vector
// in local v, failing as the interpreter does unless it is in range.
func (c *wasmCompiler) vectorIndex(b *wasmBody, name string, v uint32) uint32 {
	i := b.local(i64)
	length := func() {
		b.LocalGet(v)
		b.I32Load(4)
		b.Op(wasm.I64ExtendI32U)
	}
	b.arg(1)
	b.decode()
	b.LocalTee(i)
	b.I64Const(0)
	b.Op(wasm.I64LtS)
	b.LocalGet(i)
	length()
	b.Op(wasm.I64GeS, wasm.I32Or)
	c.failIf(b, name+": index ", func() {
		b.LocalGet(i)
		b.Call(c.rt.putInt)
		c.text(b, " out of range for a vector of length ")
		length()
		b.Call(c.rt.putInt)
	})
	return i
}

// (vector x ...): its arguments, copied from the argument stack.
func (c *wasmCompiler) compileVector(b *wasmBody) {
	c.objectOfArgs(b, tagVector)
}

// objectOfArgs leaves a new vector-like object holding the call's arguments,
// copied from the argument stack.
func (c *wasmCompiler) objectOfArgs(b *wasmBody, tag int32) {
	argc := b.param(paramArgc)
	p := b.local(i32)
	c.newVector(b, tag, argc)
	b.LocalTee(p)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(paramArgs)
	b.LocalGet(argc)
	b.I32Const(3)
	b.Op(wasm.I32Shl)
	b.MemoryCopy()
	b.LocalGet(p)
	b.Op(wasm.I64ExtendI32U)
}

func (c *wasmCompiler) compileVectorRef(b *wasmBody) {
	v := vectorArg(b)
	i := c.vectorIndex(b, "vector_ref", v)
	element(b, v, func() {
		b.LocalGet(i)
		b.Op(wasm.I32WrapI64)
	})
	b.I64Load(8)
}

func (c *wasmCompiler) compileVectorLength(b *wasmBody) {
	v := vectorArg(b)
	b.LocalGet(v)
	b.I32Load(4)
	b.Op(wasm.I64ExtendI32U)
	b.encode()
}

// (vector_set v i x): a copy of v with x at i.
func (c *wasmCompiler) compileVectorSet(b *wasmBody) {
	v := vectorArg(b)
	i := c.vectorIndex(b, "vector_set", v)
	n, w := b.local(i32), b.local(i32)
	b.LocalGet(v)
	b.I32Load(4)
	b.LocalSet(n)
	c.newVector(b, tagVector, n)
	b.LocalTee(w)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(v)
	b.I32Const(8)
	b.Op(wasm.I32Add)
	b.LocalGet(n)
	b.I32Const(3)
	b.Op(wasm.I32Shl)
	b.MemoryCopy()
	element(b, w, func() {
		b.LocalGet(i)
		b.Op(wasm.I32WrapI64)
	})
	b.arg(2)
	b.I64Store(8)
	b.LocalGet(w)
	b.Op(wasm.I64ExtendI32U)
}

// (vector_to_list v): from the last value to the first, each consed on.
func (c *wasmCompiler) compileVectorToList(b *wasmBody) {
	v := vectorArg(b)
	k, list := b.local(i32), b.local(i64)
	b.LocalGet(v)
	b.I32Load(4)
	b.LocalSet(k)
	b.I64Const(addrEmpty)
	b.LocalSet(list)
	b.until(func() {
		b.LocalGet(k)
		b.Op(wasm.I32Eqz)
	}, func() {
		b.LocalGet(k)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.LocalSet(k)
		element(b, v, func() { b.LocalGet(k) })
		b.I64Load(8)
		b.LocalGet(list)
		b.Call(c.rt.pair)
		b.LocalSet(list)
	})
	b.LocalGet(list)
}
