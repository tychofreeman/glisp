package glisp

import "github.com/tychofreeman/glisp/wasm"

// Data values in compiled code: an object tagged tagData, laid out as a
// vector whose first value is the constructor's name and the rest its
// fields.

// dataOps compile the data builtins. Their arguments have been checked
// against the spec already.
var dataOps = map[string]func(c *wasmCompiler, b *wasmBody){
	"data":        (*wasmCompiler).compileData,
	"data_name":   (*wasmCompiler).compileDataName,
	"data_ref":    (*wasmCompiler).compileDataRef,
	"data_length": (*wasmCompiler).compileDataLength,
}

// (data name x ...): its arguments, the name first.
func (c *wasmCompiler) compileData(b *wasmBody) {
	c.objectOfArgs(b, tagData)
}

func (c *wasmCompiler) compileDataName(b *wasmBody) {
	d := vectorArg(b)
	b.LocalGet(d)
	b.I64Load(8)
}

// fieldCount leaves how many fields the data value in local d has, an i32.
func fieldCount(b *wasmBody, d uint32) {
	b.LocalGet(d)
	b.I32Load(4)
	b.I32Const(1)
	b.Op(wasm.I32Sub)
}

func (c *wasmCompiler) compileDataLength(b *wasmBody) {
	d := vectorArg(b)
	fieldCount(b, d)
	b.Op(wasm.I64ExtendI32U)
	b.encode()
}

// (data_ref d i): field i, after the name.
func (c *wasmCompiler) compileDataRef(b *wasmBody) {
	d := vectorArg(b)
	i := b.local(i64)
	b.arg(1)
	b.decode()
	b.LocalTee(i)
	b.I64Const(0)
	b.Op(wasm.I64LtS)
	b.LocalGet(i)
	fieldCount(b, d)
	b.Op(wasm.I64ExtendI32U)
	b.Op(wasm.I64GeS, wasm.I32Or)
	c.failIf(b, "data_ref: index ", func() {
		b.LocalGet(i)
		b.Call(c.rt.putInt)
		c.text(b, " out of range for ")
		b.arg(0)
		b.Call(c.rt.putValue)
	})
	element(b, d, func() {
		b.LocalGet(i)
		b.Op(wasm.I32WrapI64)
	})
	b.I64Load(16)
}
