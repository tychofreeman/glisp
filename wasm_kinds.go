package glisp

import "github.com/tychofreeman/glisp/wasm"

// The kinds of value in compiled code, by tag: the type of each, as
// typeName names it, and how the objects on the heap lie in memory. The
// type names, the type predicates, argument checks, objSize and the
// collector all come from this table.

// A layout is how an object lies in memory: a fixed part of size bytes,
// with values at the offsets in values, then, if count isn't 0, as many
// elements of elem bytes as the i32 at offset count says, which are values
// if elemValues.
type layout struct {
	size       uint32
	values     []uint32
	count      uint32
	elem       uint32
	elemValues bool
}

// A tagKind is what one tag means.
type tagKind struct {
	tag    int32
	typ    string  // "" for a cell, which no program sees
	layout *layout // nil for what isn't on the heap
}

var tagKinds = []tagKind{
	{tagInt, "number", nil},
	{tagNil, "nil", nil},
	{tagTrue, "bool", nil},
	{tagFalse, "bool", nil},
	{tagEmpty, "list", nil},
	{tagPair, "list", &layout{size: 24, values: []uint32{8, 16}}},
	{tagString, "string", &layout{size: 8, count: 4, elem: 1}},
	{tagSymbol, "symbol", &layout{size: 8, count: 4, elem: 1}},
	{tagClosure, "function", &layout{size: 16, count: 8, elem: 8, elemValues: true}},
	{tagCell, "", &layout{size: 16, values: []uint32{8}}},
	{tagVector, "vector", &layout{size: 8, count: 4, elem: 8, elemValues: true}},
}

// tagsOf are the tags of the types typs.
func tagsOf(typs ...string) []int32 {
	var tags []int32
	for _, k := range tagKinds {
		for _, typ := range typs {
			if k.typ == typ {
				tags = append(tags, k.tag)
			}
		}
	}
	return tags
}

// tags are the tags that values of kind k have.
func (k kind) tags() []int32 {
	return tagsOf(k.types()...)
}

// objSize(p) is the size in bytes of the object at p.
func (c *wasmCompiler) objSizeBody(b *wasmBody) {
	p, tag := b.param(0), b.local(i32)
	b.LocalGet(p)
	b.I32Load(0)
	b.LocalSet(tag)
	for _, k := range tagKinds {
		if k.layout == nil {
			continue
		}
		b.LocalGet(tag)
		b.I32Const(k.tag)
		b.Op(wasm.I32Eq)
		b.returnIf(func() {
			l := k.layout
			if l.count == 0 {
				b.I32Const(int32(l.size))
				return
			}
			b.LocalGet(p)
			b.I32Load(l.count)
			b.I32Const(int32(l.elem))
			b.Op(wasm.I32Mul)
			b.I32Const(int32(l.size + 7))
			b.Op(wasm.I32Add)
			b.I32Const(-8)
			b.Op(wasm.I32And)
		})
	}
	b.Op(wasm.Unreachable)
}

// eachValue passes each value field of the object at local p, by its
// address, to fn: what the collector updates. k is an i32 local to count
// with, and at another, for the address.
func (b *wasmBody) eachValue(p, k, at uint32, fn func(at uint32)) {
	for _, kind := range tagKinds {
		l := kind.layout
		if l == nil || len(l.values) == 0 && !l.elemValues {
			continue
		}
		b.LocalGet(p)
		b.I32Load(0)
		b.I32Const(kind.tag)
		b.Op(wasm.I32Eq)
		b.when(func() {
			for _, offset := range l.values {
				b.LocalGet(p)
				b.I32Const(int32(offset))
				b.Op(wasm.I32Add)
				b.LocalSet(at)
				fn(at)
			}
			if !l.elemValues {
				return
			}
			b.I32Const(0)
			b.LocalSet(k)
			b.until(func() {
				b.LocalGet(k)
				b.LocalGet(p)
				b.I32Load(l.count)
				b.Op(wasm.I32GeU)
			}, func() {
				b.LocalGet(p)
				b.LocalGet(k)
				b.I32Const(3)
				b.Op(wasm.I32Shl, wasm.I32Add)
				b.I32Const(int32(l.size))
				b.Op(wasm.I32Add)
				b.LocalSet(at)
				fn(at)
				b.incr(k)
			})
		})
	}
}

// putType(v) prints the type of v, as typeName names it.
func (c *wasmCompiler) putTypeBody(b *wasmBody) {
	v, t := b.param(0), b.local(i32)
	b.LocalGet(v)
	b.Call(c.rt.tagOf)
	b.LocalSet(t)
	for _, k := range tagKinds {
		if k.typ == "" {
			continue
		}
		b.LocalGet(t)
		b.I32Const(k.tag)
		b.Op(wasm.I32Eq)
		b.returnIf(func() { c.text(b, k.typ) })
	}
}
