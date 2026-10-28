package glisp

import "github.com/tychofreeman/glisp/wasm"

// Maps in compiled code: the CHAMP tries of maps.go, laid out in memory.
// A map is tagMap: its count at +4 and its root node at +8. A node is
// tagMapNode: its data bitmap at +4, its node bitmap at +8, its number of
// slots at +12, and its slots from +16: a key and a value for each bit of
// the data bitmap, then a subtrie for each bit of the node bitmap, in bit
// order. Below the last level a node's bitmaps are 0 and its slots hold the
// colliding keys and values. Both are rows of tagKinds, which is all the
// collector needs. The order of entries, and the shape after a removal, are
// the interpreter's, so maps print alike.

// mapOps compile the map builtins. Their arguments have been checked against
// the spec already.
var mapOps = map[string]func(c *wasmCompiler, b *wasmBody){
	"map":         (*wasmCompiler).compileMap,
	"map_get":     (*wasmCompiler).compileMapGet,
	"map_put":     (*wasmCompiler).compileMapPut,
	"map_remove":  (*wasmCompiler).compileMapRemove,
	"map_has":     (*wasmCompiler).compileMapHas,
	"map_count":   (*wasmCompiler).compileMapCount,
	"map_keys":    func(c *wasmCompiler, b *wasmBody) { c.compileMapList(b, 0) },
	"map_to_list": func(c *wasmCompiler, b *wasmBody) { c.compileMapList(b, 1) },
}

func (c *wasmCompiler) declareMaps() {
	rt := &c.rt
	rt.newNode = c.declare(sig(vals(i32, i32, i32), i32))
	rt.nodeSplice = c.declare(sig(vals(i32, i32, i32, i32, i32, i32, i32), i32))
	rt.pairNode = c.declare(sig(vals(i64, i64, i32, i64, i64, i32, i32), i32))
	rt.nodeGet = c.declare(sig(vals(i32, i64, i32, i32), i64))
	rt.nodePut = c.declare(sig(vals(i32, i64, i64, i32, i32, i32), i32))
	rt.nodeRemove = c.declare(sig(vals(i32, i64, i32, i32), i32))
	rt.mapPrint = c.declare(sig(vals(i32, i32), i32))
	rt.mapPush = c.declare(sig(vals(i32, i32)))
	rt.mapAllIn = c.declare(sig(vals(i32, i32), i32))
	rt.newMap = c.declare(sig(vals(i32, i32), i64))
	rt.mapPut = c.declare(sig(vals(i64, i64, i64, i32), i64))
	rt.mapGet = c.declare(sig(vals(i64, i64, i32), i64))
	rt.mapRemove = c.declare(sig(vals(i64, i64, i32), i64))
}

func (c *wasmCompiler) defineMaps() {
	rt := &c.rt
	c.define(rt.newNode, c.newNodeBody)
	c.define(rt.nodeSplice, c.nodeSpliceBody)
	c.define(rt.pairNode, c.pairNodeBody)
	c.define(rt.nodeGet, c.nodeGetBody)
	c.define(rt.nodePut, c.nodePutBody)
	c.define(rt.nodeRemove, c.nodeRemoveBody)
	c.define(rt.mapPrint, c.mapPrintBody)
	c.define(rt.mapPush, c.mapPushBody)
	c.define(rt.mapAllIn, c.mapAllInBody)
	c.define(rt.newMap, c.newMapBody)
	c.define(rt.mapPut, c.mapPutBody)
	c.define(rt.mapGet, c.mapGetBody)
	c.define(rt.mapRemove, c.mapRemoveBody)
}

// nodeField leaves the i32 at offset in the node at local p: 4 its data
// bitmap, 8 its node bitmap, 12 its number of slots.
func nodeField(b *wasmBody, p uint32, offset uint32) {
	b.LocalGet(p)
	b.I32Load(offset)
}

// slotAt leaves the address of slot index of the node at local p, less 16:
// load and store the slot at offset 16.
func slotAt(b *wasmBody, p uint32, index func()) {
	b.LocalGet(p)
	index()
	b.I32Const(3)
	b.Op(wasm.I32Shl, wasm.I32Add)
}

// loadSlot leaves slot index of the node at local p.
func loadSlot(b *wasmBody, p uint32, index func()) {
	slotAt(b, p, index)
	b.I64Load(16)
}

// storeSlot stores what value leaves in slot index of the node at local p.
func storeSlot(b *wasmBody, p uint32, index, value func()) {
	slotAt(b, p, index)
	value()
	b.I64Store(16)
}

// bitOf leaves the bit of the slot of the hash in local h at the level of
// the shift in local s.
func bitOf(b *wasmBody, h, s uint32) {
	b.I32Const(1)
	b.LocalGet(h)
	b.LocalGet(s)
	b.Op(wasm.I32ShrU)
	b.I32Const(31)
	b.Op(wasm.I32And, wasm.I32Shl)
}

// popBelow leaves how many bits of the bitmap in local bitmap are below the
// bit in local bit.
func popBelow(b *wasmBody, bitmap, bit uint32) {
	b.LocalGet(bitmap)
	b.LocalGet(bit)
	b.I32Const(1)
	b.Op(wasm.I32Sub, wasm.I32And, wasm.I32Popcnt)
}

// at leaves local x, plus k.
func at(b *wasmBody, x uint32, k int32) func() {
	return func() {
		b.LocalGet(x)
		if k != 0 {
			b.I32Const(k)
			b.Op(wasm.I32Add)
		}
	}
}

// constant leaves k.
func constant(b *wasmBody, k int32) func() { return func() { b.I32Const(k) } }

// setChanged records whether a put added a key, or a remove removed one.
func setChanged(b *wasmBody, changed int32) {
	b.I32Const(changed)
	b.GlobalSet(gMapChanged)
}

// newNode(dataMap, nodeMap, n) is a new node with n slots, not yet set.
func (c *wasmCompiler) newNodeBody(b *wasmBody) {
	dm, nm, n, p := b.param(0), b.param(1), b.param(2), b.local(i32)
	b.LocalGet(n)
	b.I32Const(3)
	b.Op(wasm.I32Shl)
	b.I32Const(16)
	b.Op(wasm.I32Add)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.I32Const(tagMapNode)
	b.I32Store(0)
	for _, f := range []struct{ local, offset uint32 }{{dm, 4}, {nm, 8}, {n, 12}} {
		b.LocalGet(p)
		b.LocalGet(f.local)
		b.I32Store(f.offset)
	}
	b.LocalGet(p)
}

// nodeSplice(old, dataMap, nodeMap, removeAt, removeCount, insertAt,
// insertCount) is a copy of old with the bitmaps given, without its
// removeCount slots from removeAt, and with insertCount slots, not yet set,
// at insertAt of the new node.
func (c *wasmCompiler) nodeSpliceBody(b *wasmBody) {
	old, dm, nm, ra, rc, ia, ic := b.param(0), b.param(1), b.param(2), b.param(3), b.param(4), b.param(5), b.param(6)
	n, p := b.local(i32), b.local(i32)
	nodeField(b, old, 12)
	b.LocalSet(n)
	b.LocalGet(dm)
	b.LocalGet(nm)
	b.LocalGet(n)
	b.LocalGet(rc)
	b.Op(wasm.I32Sub)
	b.LocalGet(ic)
	b.Op(wasm.I32Add)
	b.Call(c.rt.newNode)
	b.LocalSet(p)
	address := func(node uint32, index func()) {
		slotAt(b, node, index)
		b.I32Const(16)
		b.Op(wasm.I32Add)
	}
	bytes := func(count func()) {
		count()
		b.I32Const(3)
		b.Op(wasm.I32Shl)
	}
	// The slots before the removed ones, then those after them.
	address(p, constant(b, 0))
	address(old, constant(b, 0))
	bytes(func() { b.LocalGet(ra) })
	b.MemoryCopy()
	address(p, func() { b.LocalGet(ra) })
	address(old, func() {
		b.LocalGet(ra)
		b.LocalGet(rc)
		b.Op(wasm.I32Add)
	})
	bytes(func() {
		b.LocalGet(n)
		b.LocalGet(ra)
		b.Op(wasm.I32Sub)
		b.LocalGet(rc)
		b.Op(wasm.I32Sub)
	})
	b.MemoryCopy()
	// Then open the gap.
	address(p, func() {
		b.LocalGet(ia)
		b.LocalGet(ic)
		b.Op(wasm.I32Add)
	})
	address(p, func() { b.LocalGet(ia) })
	bytes(func() {
		b.LocalGet(n)
		b.LocalGet(rc)
		b.Op(wasm.I32Sub)
		b.LocalGet(ia)
		b.Op(wasm.I32Sub)
	})
	b.MemoryCopy()
	b.LocalGet(p)
}

// pairNode(k1, v1, h1, k2, v2, h2, shift) is the subtrie, at the level of
// shift, of two entries whose hashes agree above it; the first is the one
// that was there.
func (c *wasmCompiler) pairNodeBody(b *wasmBody) {
	k1, v1, h1, k2, v2, h2, s := b.param(0), b.param(1), b.param(2), b.param(3), b.param(4), b.param(5), b.param(6)
	b1, b2, p := b.local(i32), b.local(i32), b.local(i32)
	fill := func(values ...uint32) {
		for i, v := range values {
			storeSlot(b, p, constant(b, int32(i)), func() { b.LocalGet(v) })
		}
	}
	b.LocalGet(s)
	b.I32Const(32)
	b.Op(wasm.I32GeU)
	b.returnIf(func() {
		b.I32Const(0)
		b.I32Const(0)
		b.I32Const(4)
		b.Call(c.rt.newNode)
		b.LocalSet(p)
		fill(k1, v1, k2, v2)
		b.LocalGet(p)
	})
	bitOf(b, h1, s)
	b.LocalSet(b1)
	bitOf(b, h2, s)
	b.LocalSet(b2)
	b.LocalGet(b1)
	b.LocalGet(b2)
	b.Op(wasm.I32Eq)
	b.returnIf(func() {
		b.I32Const(0)
		b.LocalGet(b1)
		b.I32Const(1)
		b.Call(c.rt.newNode)
		b.LocalSet(p)
		storeSlot(b, p, constant(b, 0), func() {
			for _, x := range []uint32{k1, v1, h1, k2, v2, h2} {
				b.LocalGet(x)
			}
			at(b, s, champBits)()
			b.Call(c.rt.pairNode)
			b.Op(wasm.I64ExtendI32U)
		})
		b.LocalGet(p)
	})
	b.LocalGet(b1)
	b.LocalGet(b2)
	b.Op(wasm.I32Or)
	b.I32Const(0)
	b.I32Const(4)
	b.Call(c.rt.newNode)
	b.LocalSet(p)
	b.LocalGet(b1)
	b.LocalGet(b2)
	b.Op(wasm.I32LtU)
	b.ifElse(wasm.BlockVoid, func() { fill(k1, v1, k2, v2) }, func() { fill(k2, v2, k1, v1) })
	b.LocalGet(p)
}

// nodeGet(node, key, hash, shift) is key's value in the trie at node, or 0
// if it has none.
func (c *wasmCompiler) nodeGetBody(b *wasmBody) {
	p, key, h, s := b.param(0), b.param(1), b.param(2), b.param(3)
	bit, dm, nm, i, n := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	keyAt := func(index func()) {
		loadSlot(b, p, index)
		b.LocalGet(key)
		b.Call(c.rt.equal)
	}
	b.until(func() { b.I32Const(0) }, func() {
		b.LocalGet(s)
		b.I32Const(32)
		b.Op(wasm.I32GeU)
		b.returnIf(func() {
			// Colliding keys.
			nodeField(b, p, 12)
			b.LocalSet(n)
			b.until(func() {
				b.LocalGet(i)
				b.LocalGet(n)
				b.Op(wasm.I32GeU)
			}, func() {
				keyAt(at(b, i, 0))
				b.returnIf(func() { loadSlot(b, p, at(b, i, 1)) })
				b.LocalGet(i)
				b.I32Const(2)
				b.Op(wasm.I32Add)
				b.LocalSet(i)
			})
			b.I64Const(0)
		})
		bitOf(b, h, s)
		b.LocalSet(bit)
		nodeField(b, p, 4)
		b.LocalSet(dm)
		nodeField(b, p, 8)
		b.LocalSet(nm)
		b.LocalGet(dm)
		b.LocalGet(bit)
		b.Op(wasm.I32And)
		b.returnIf(func() {
			popBelow(b, dm, bit)
			b.I32Const(1)
			b.Op(wasm.I32Shl)
			b.LocalSet(i)
			keyAt(at(b, i, 0))
			b.ifElse(wasm.BlockI64, func() { loadSlot(b, p, at(b, i, 1)) }, func() { b.I64Const(0) })
		})
		b.LocalGet(nm)
		b.LocalGet(bit)
		b.Op(wasm.I32And, wasm.I32Eqz)
		b.returnIf(func() { b.I64Const(0) })
		// Down into the subtrie.
		c.childIndex(b, dm, nm, bit)
		b.LocalSet(i)
		loadSlot(b, p, at(b, i, 0))
		b.Op(wasm.I32WrapI64)
		b.LocalSet(p)
		at(b, s, champBits)()
		b.LocalSet(s)
	})
	b.I64Const(0)
}

// childIndex leaves the slot of the subtrie for the bit in local bit, given
// the bitmaps in dm and nm: after the data's slots, the subtries below it.
func (c *wasmCompiler) childIndex(b *wasmBody, dm, nm, bit uint32) {
	b.LocalGet(dm)
	b.Op(wasm.I32Popcnt)
	b.I32Const(1)
	b.Op(wasm.I32Shl)
	popBelow(b, nm, bit)
	b.Op(wasm.I32Add)
}

// nodePut(node, key, value, hash, shift, name) is the trie at node with key's
// value set; gMapChanged says whether key is new. name, a string, names the
// builtin for the error a key that can't be hashed gives.
func (c *wasmCompiler) nodePutBody(b *wasmBody) {
	p, key, value, h, s, name := b.param(0), b.param(1), b.param(2), b.param(3), b.param(4), b.param(5)
	bit, dm, nm, i, n, q := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	splice := func(dataMap, nodeMap, removeAt, removeCount, insertAt, insertCount func()) {
		b.LocalGet(p)
		for _, arg := range []func(){dataMap, nodeMap, removeAt, removeCount, insertAt, insertCount} {
			arg()
		}
		b.Call(c.rt.nodeSplice)
		b.LocalSet(q)
	}
	loc := func(x uint32) func() { return at(b, x, 0) }
	keyAt := func(index func()) {
		loadSlot(b, p, index)
		b.LocalGet(key)
		b.Call(c.rt.equal)
	}
	// replaceValue returns p with the value at slot i+1 replaced.
	replaceValue := func(dataMap, nodeMap func()) {
		splice(dataMap, nodeMap, at(b, i, 1), constant(b, 1), at(b, i, 1), constant(b, 1))
		storeSlot(b, q, at(b, i, 1), loc(value))
		setChanged(b, 0)
		b.LocalGet(q)
	}
	b.LocalGet(s)
	b.I32Const(32)
	b.Op(wasm.I32GeU)
	b.returnIf(func() {
		// Colliding keys: replace the value, or add the entry at the end.
		nodeField(b, p, 12)
		b.LocalSet(n)
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(n)
			b.Op(wasm.I32GeU)
		}, func() {
			keyAt(loc(i))
			b.returnIf(func() { replaceValue(constant(b, 0), constant(b, 0)) })
			b.LocalGet(i)
			b.I32Const(2)
			b.Op(wasm.I32Add)
			b.LocalSet(i)
		})
		splice(constant(b, 0), constant(b, 0), loc(n), constant(b, 0), loc(n), constant(b, 2))
		storeSlot(b, q, loc(n), loc(key))
		storeSlot(b, q, at(b, n, 1), loc(value))
		setChanged(b, 1)
		b.LocalGet(q)
	})
	bitOf(b, h, s)
	b.LocalSet(bit)
	nodeField(b, p, 4)
	b.LocalSet(dm)
	nodeField(b, p, 8)
	b.LocalSet(nm)
	b.LocalGet(dm)
	b.LocalGet(bit)
	b.Op(wasm.I32And)
	b.returnIf(func() {
		popBelow(b, dm, bit)
		b.I32Const(1)
		b.Op(wasm.I32Shl)
		b.LocalSet(i)
		keyAt(loc(i))
		b.returnIf(func() { replaceValue(loc(dm), loc(nm)) })
		// Two keys in one slot: they move down into a subtrie.
		b.LocalGet(dm)
		b.LocalGet(bit)
		b.I32Const(-1)
		b.Op(wasm.I32Xor, wasm.I32And)
		b.LocalSet(dm)
		b.LocalGet(nm)
		b.LocalGet(bit)
		b.Op(wasm.I32Or)
		b.LocalSet(nm)
		c.childIndex(b, dm, nm, bit)
		b.LocalSet(n)
		splice(loc(dm), loc(nm), loc(i), constant(b, 2), loc(n), constant(b, 1))
		storeSlot(b, q, loc(n), func() {
			loadSlot(b, p, loc(i))
			loadSlot(b, p, at(b, i, 1))
			loadSlot(b, p, loc(i))
			offset := fnvOffset // as an i32, its bits
			b.I32Const(int32(offset))
			b.LocalGet(name)
			b.Call(c.rt.hashInto)
			b.LocalGet(key)
			b.LocalGet(value)
			b.LocalGet(h)
			at(b, s, champBits)()
			b.Call(c.rt.pairNode)
			b.Op(wasm.I64ExtendI32U)
		})
		setChanged(b, 1)
		b.LocalGet(q)
	})
	b.LocalGet(nm)
	b.LocalGet(bit)
	b.Op(wasm.I32And)
	b.returnIf(func() {
		c.childIndex(b, dm, nm, bit)
		b.LocalSet(i)
		loadSlot(b, p, loc(i))
		b.Op(wasm.I32WrapI64)
		b.LocalGet(key)
		b.LocalGet(value)
		b.LocalGet(h)
		at(b, s, champBits)()
		b.LocalGet(name)
		b.Call(c.rt.nodePut)
		b.LocalSet(n)
		splice(loc(dm), loc(nm), loc(i), constant(b, 1), loc(i), constant(b, 1))
		storeSlot(b, q, loc(i), func() {
			b.LocalGet(n)
			b.Op(wasm.I64ExtendI32U)
		})
		b.LocalGet(q)
	})
	// A new key in an empty slot.
	b.LocalGet(dm)
	b.LocalGet(bit)
	b.Op(wasm.I32Or)
	b.LocalSet(dm)
	popBelow(b, dm, bit)
	b.I32Const(1)
	b.Op(wasm.I32Shl)
	b.LocalSet(i)
	splice(loc(dm), loc(nm), constant(b, 0), constant(b, 0), loc(i), constant(b, 2))
	storeSlot(b, q, loc(i), loc(key))
	storeSlot(b, q, at(b, i, 1), loc(value))
	setChanged(b, 1)
	b.LocalGet(q)
}

// nodeRemove(node, key, hash, shift) is the trie at node without key;
// gMapChanged says whether it had it. A subtrie left with one entry comes
// back up into its parent, as in maps.go.
func (c *wasmCompiler) nodeRemoveBody(b *wasmBody) {
	p, key, h, s := b.param(0), b.param(1), b.param(2), b.param(3)
	bit, dm, nm, i, n, q, t := b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32), b.local(i32)
	loc := func(x uint32) func() { return at(b, x, 0) }
	splice := func(into uint32, from uint32, dataMap, nodeMap, removeAt, removeCount, insertAt, insertCount func()) {
		b.LocalGet(from)
		for _, arg := range []func(){dataMap, nodeMap, removeAt, removeCount, insertAt, insertCount} {
			arg()
		}
		b.Call(c.rt.nodeSplice)
		b.LocalSet(into)
	}
	keyAt := func(index func()) {
		loadSlot(b, p, index)
		b.LocalGet(key)
		b.Call(c.rt.equal)
	}
	unchanged := func() {
		setChanged(b, 0)
		b.LocalGet(p)
	}
	b.LocalGet(s)
	b.I32Const(32)
	b.Op(wasm.I32GeU)
	b.returnIf(func() {
		nodeField(b, p, 12)
		b.LocalSet(n)
		b.until(func() {
			b.LocalGet(i)
			b.LocalGet(n)
			b.Op(wasm.I32GeU)
		}, func() {
			keyAt(loc(i))
			b.returnIf(func() {
				splice(q, p, constant(b, 0), constant(b, 0), loc(i), constant(b, 2), constant(b, 0), constant(b, 0))
				setChanged(b, 1)
				b.LocalGet(q)
			})
			b.LocalGet(i)
			b.I32Const(2)
			b.Op(wasm.I32Add)
			b.LocalSet(i)
		})
		unchanged()
	})
	bitOf(b, h, s)
	b.LocalSet(bit)
	nodeField(b, p, 4)
	b.LocalSet(dm)
	nodeField(b, p, 8)
	b.LocalSet(nm)
	b.LocalGet(dm)
	b.LocalGet(bit)
	b.Op(wasm.I32And)
	b.returnIf(func() {
		popBelow(b, dm, bit)
		b.I32Const(1)
		b.Op(wasm.I32Shl)
		b.LocalSet(i)
		keyAt(loc(i))
		b.Op(wasm.I32Eqz)
		b.returnIf(unchanged)
		splice(q, p, func() {
			b.LocalGet(dm)
			b.LocalGet(bit)
			b.I32Const(-1)
			b.Op(wasm.I32Xor, wasm.I32And)
		}, loc(nm), loc(i), constant(b, 2), constant(b, 0), constant(b, 0))
		setChanged(b, 1)
		b.LocalGet(q)
	})
	b.LocalGet(nm)
	b.LocalGet(bit)
	b.Op(wasm.I32And, wasm.I32Eqz)
	b.returnIf(unchanged)
	c.childIndex(b, dm, nm, bit)
	b.LocalSet(i)
	loadSlot(b, p, loc(i))
	b.Op(wasm.I32WrapI64)
	b.LocalGet(key)
	b.LocalGet(h)
	at(b, s, champBits)()
	b.Call(c.rt.nodeRemove)
	b.LocalSet(t)
	b.GlobalGet(gMapChanged)
	b.Op(wasm.I32Eqz)
	b.returnIf(func() { b.LocalGet(p) })
	// One entry left below: it comes back up into this node.
	nodeField(b, t, 8)
	b.Op(wasm.I32Eqz)
	nodeField(b, t, 12)
	b.I32Const(2)
	b.Op(wasm.I32Eq, wasm.I32And)
	b.returnIf(func() {
		b.LocalGet(dm)
		b.LocalGet(bit)
		b.Op(wasm.I32Or)
		b.LocalSet(dm)
		b.LocalGet(nm)
		b.LocalGet(bit)
		b.I32Const(-1)
		b.Op(wasm.I32Xor, wasm.I32And)
		b.LocalSet(nm)
		popBelow(b, dm, bit)
		b.I32Const(1)
		b.Op(wasm.I32Shl)
		b.LocalSet(n)
		splice(q, p, loc(dm), loc(nm), loc(i), constant(b, 1), loc(n), constant(b, 2))
		storeSlot(b, q, loc(n), func() { loadSlot(b, t, constant(b, 0)) })
		storeSlot(b, q, at(b, n, 1), func() { loadSlot(b, t, constant(b, 1)) })
		b.LocalGet(q)
	})
	splice(q, p, loc(dm), loc(nm), loc(i), constant(b, 1), loc(i), constant(b, 1))
	storeSlot(b, q, loc(i), func() {
		b.LocalGet(t)
		b.Op(wasm.I64ExtendI32U)
	})
	b.LocalGet(q)
}

// dataSlots leaves how many slots of the node at local p hold keys and
// values: all but its subtries'.
func dataSlots(b *wasmBody, p uint32) {
	nodeField(b, p, 12)
	nodeField(b, p, 8)
	b.Op(wasm.I32Popcnt, wasm.I32Sub)
}

// eachSlot runs body for each slot index i from local from up to local to,
// stepping by step.
func eachSlot(b *wasmBody, i uint32, from func(), to uint32, step int32, body func()) {
	from()
	b.LocalSet(i)
	b.until(func() {
		b.LocalGet(i)
		b.LocalGet(to)
		b.Op(wasm.I32GeU)
	}, func() {
		body()
		b.LocalGet(i)
		b.I32Const(step)
		b.Op(wasm.I32Add)
		b.LocalSet(i)
	})
}

// mapPrint(node, first) prints the entries of the trie at node as Write
// does, "k v" separated by ", "; first says none has been printed yet, and
// the result whether that is still so.
func (c *wasmCompiler) mapPrintBody(b *wasmBody) {
	p, first, d, n, i := b.param(0), b.param(1), b.local(i32), b.local(i32), b.local(i32)
	dataSlots(b, p)
	b.LocalSet(d)
	nodeField(b, p, 12)
	b.LocalSet(n)
	eachSlot(b, i, constant(b, 0), d, 2, func() {
		b.LocalGet(first)
		b.Op(wasm.I32Eqz)
		b.when(func() { c.text(b, ", ") })
		loadSlot(b, p, at(b, i, 0))
		b.Call(c.rt.putValue)
		b.I32Const(' ')
		b.Call(c.rt.putByte)
		loadSlot(b, p, at(b, i, 1))
		b.Call(c.rt.putValue)
		b.I32Const(0)
		b.LocalSet(first)
	})
	eachSlot(b, i, at(b, d, 0), n, 1, func() {
		loadSlot(b, p, at(b, i, 0))
		b.Op(wasm.I32WrapI64)
		b.LocalGet(first)
		b.Call(c.rt.mapPrint)
		b.LocalSet(first)
	})
	b.LocalGet(first)
}

// mapPush(node, values) pushes the keys of the trie at node onto the
// argument stack, in order, each followed by its value if values is 1.
func (c *wasmCompiler) mapPushBody(b *wasmBody) {
	p, values, d, n, i := b.param(0), b.param(1), b.local(i32), b.local(i32), b.local(i32)
	dataSlots(b, p)
	b.LocalSet(d)
	nodeField(b, p, 12)
	b.LocalSet(n)
	eachSlot(b, i, constant(b, 0), d, 2, func() {
		c.checkStack(b)
		b.push(func() { loadSlot(b, p, at(b, i, 0)) })
		b.LocalGet(values)
		b.when(func() { b.push(func() { loadSlot(b, p, at(b, i, 1)) }) })
	})
	eachSlot(b, i, at(b, d, 0), n, 1, func() {
		loadSlot(b, p, at(b, i, 0))
		b.Op(wasm.I32WrapI64)
		b.LocalGet(values)
		b.Call(c.rt.mapPush)
	})
}

// mapAllIn(node, other) says whether each entry of the trie at node is in
// the trie at other with an equal value.
func (c *wasmCompiler) mapAllInBody(b *wasmBody) {
	p, other := b.param(0), b.param(1)
	d, n, i, v := b.local(i32), b.local(i32), b.local(i32), b.local(i64)
	no := func() { b.I32Const(0) }
	dataSlots(b, p)
	b.LocalSet(d)
	nodeField(b, p, 12)
	b.LocalSet(n)
	eachSlot(b, i, constant(b, 0), d, 2, func() {
		b.LocalGet(other)
		loadSlot(b, p, at(b, i, 0))
		loadSlot(b, p, at(b, i, 0))
		offset := fnvOffset // as an i32, its bits
		b.I32Const(int32(offset))
		b.I32Const(int32(c.str("eq")))
		b.Call(c.rt.hashInto)
		b.I32Const(0)
		b.Call(c.rt.nodeGet)
		b.LocalTee(v)
		b.Op(wasm.I64Eqz)
		b.returnIf(no)
		b.LocalGet(v)
		loadSlot(b, p, at(b, i, 1))
		b.Call(c.rt.equal)
		b.Op(wasm.I32Eqz)
		b.returnIf(no)
	})
	eachSlot(b, i, at(b, d, 0), n, 1, func() {
		loadSlot(b, p, at(b, i, 0))
		b.Op(wasm.I32WrapI64)
		b.LocalGet(other)
		b.Call(c.rt.mapAllIn)
		b.Op(wasm.I32Eqz)
		b.returnIf(no)
	})
	b.I32Const(1)
}

// newMap(count, root) is a map.
func (c *wasmCompiler) newMapBody(b *wasmBody) {
	count, root, p := b.param(0), b.param(1), b.local(i32)
	b.I32Const(16)
	b.Call(c.rt.alloc)
	b.LocalTee(p)
	b.I32Const(tagMap)
	b.I32Store(0)
	b.LocalGet(p)
	b.LocalGet(count)
	b.I32Store(4)
	b.LocalGet(p)
	b.LocalGet(root)
	b.Op(wasm.I64ExtendI32U)
	b.I64Store(8)
	b.LocalGet(p)
	b.Op(wasm.I64ExtendI32U)
}

// mapRoot leaves the root node of the map in local m.
func mapRoot(b *wasmBody, m uint32) {
	b.field(m, 8)
	b.Op(wasm.I32WrapI64)
}

// mapCountOf leaves the count of the map in local m.
func mapCountOf(b *wasmBody, m uint32) {
	b.LocalGet(m)
	b.Op(wasm.I32WrapI64)
	b.I32Load(4)
}

// hashKey leaves the hash of the value in local key, for the builtin whose
// name is in local name.
func (c *wasmCompiler) hashKey(b *wasmBody, key, name uint32) {
	b.LocalGet(key)
	offset := fnvOffset // as an i32, its bits
	b.I32Const(int32(offset))
	b.LocalGet(name)
	b.Call(c.rt.hashInto)
}

// mapPut(m, key, value, name) is m with key's value set.
func (c *wasmCompiler) mapPutBody(b *wasmBody) {
	m, key, value, name := b.param(0), b.param(1), b.param(2), b.param(3)
	root := b.local(i32)
	mapRoot(b, m)
	b.LocalGet(key)
	b.LocalGet(value)
	c.hashKey(b, key, name)
	b.I32Const(0)
	b.LocalGet(name)
	b.Call(c.rt.nodePut)
	b.LocalSet(root)
	mapCountOf(b, m)
	b.GlobalGet(gMapChanged)
	b.Op(wasm.I32Add)
	b.LocalGet(root)
	b.Call(c.rt.newMap)
}

// mapGet(m, key, name) is key's value in m, or 0 if it has none.
func (c *wasmCompiler) mapGetBody(b *wasmBody) {
	m, key, name := b.param(0), b.param(1), b.param(2)
	mapRoot(b, m)
	b.LocalGet(key)
	c.hashKey(b, key, name)
	b.I32Const(0)
	b.Call(c.rt.nodeGet)
}

// mapRemove(m, key, name) is m without key.
func (c *wasmCompiler) mapRemoveBody(b *wasmBody) {
	m, key, name := b.param(0), b.param(1), b.param(2)
	root := b.local(i32)
	mapRoot(b, m)
	b.LocalGet(key)
	c.hashKey(b, key, name)
	b.I32Const(0)
	b.Call(c.rt.nodeRemove)
	b.LocalSet(root)
	b.GlobalGet(gMapChanged)
	b.Op(wasm.I32Eqz)
	b.returnIf(func() { b.LocalGet(m) })
	mapCountOf(b, m)
	b.I32Const(1)
	b.Op(wasm.I32Sub)
	b.LocalGet(root)
	b.Call(c.rt.newMap)
}

// name leaves the address of the builtin's name, a string.
func (c *wasmCompiler) nameOf(b *wasmBody, name string) {
	b.I32Const(int32(c.str(name)))
}

// (map k v ...): the empty map, with each pair put in turn.
func (c *wasmCompiler) compileMap(b *wasmBody) {
	argc := b.param(paramArgc)
	m, i := b.local(i64), b.local(i32)
	b.LocalGet(argc)
	b.I32Const(1)
	b.Op(wasm.I32And)
	c.failIf(b, "map: expects keys and values in pairs, got ", func() {
		b.LocalGet(argc)
		b.I32Const(1)
		b.Op(wasm.I32Eq)
		b.ifElse(wasm.BlockVoid, func() { c.text(b, "1 argument") }, func() {
			b.LocalGet(argc)
			b.Op(wasm.I64ExtendI32U)
			b.Call(c.rt.putInt)
			c.text(b, " arguments")
		})
	})
	b.I32Const(0)
	b.I32Const(0)
	b.I32Const(0)
	b.I32Const(0)
	b.Call(c.rt.newNode)
	b.Call(c.rt.newMap)
	b.LocalSet(m)
	eachSlot(b, i, constant(b, 0), argc, 2, func() {
		b.LocalGet(m)
		b.argAt(i)
		b.LocalGet(i)
		b.I32Const(1)
		b.Op(wasm.I32Add)
		b.LocalSet(i)
		b.argAt(i)
		b.LocalGet(i)
		b.I32Const(1)
		b.Op(wasm.I32Sub)
		b.LocalSet(i)
		c.nameOf(b, "map")
		b.Call(c.rt.mapPut)
		b.LocalSet(m)
	})
	b.LocalGet(m)
}

func (c *wasmCompiler) compileMapGet(b *wasmBody) {
	argc := b.param(paramArgc)
	v := b.local(i64)
	b.arg(0)
	b.arg(1)
	c.nameOf(b, "map_get")
	b.Call(c.rt.mapGet)
	b.LocalTee(v)
	b.Op(wasm.I64Eqz)
	b.ifElse(wasm.BlockI64, func() {
		b.LocalGet(argc)
		b.I32Const(3)
		b.Op(wasm.I32Eq)
		b.ifElse(wasm.BlockI64, func() { b.arg(2) }, func() { b.I64Const(addrNil) })
	}, func() { b.LocalGet(v) })
}

func (c *wasmCompiler) compileMapPut(b *wasmBody) {
	b.arg(0)
	b.arg(1)
	b.arg(2)
	c.nameOf(b, "map_put")
	b.Call(c.rt.mapPut)
}

func (c *wasmCompiler) compileMapRemove(b *wasmBody) {
	b.arg(0)
	b.arg(1)
	c.nameOf(b, "map_remove")
	b.Call(c.rt.mapRemove)
}

func (c *wasmCompiler) compileMapHas(b *wasmBody) {
	b.arg(0)
	b.arg(1)
	c.nameOf(b, "map_has")
	b.Call(c.rt.mapGet)
	b.Op(wasm.I64Eqz, wasm.I32Eqz)
	b.boolean()
}

func (c *wasmCompiler) compileMapCount(b *wasmBody) {
	m := b.local(i64)
	b.arg(0)
	b.LocalSet(m)
	mapCountOf(b, m)
	b.Op(wasm.I64ExtendI32U)
	b.encode()
}

// compileMapList is map_keys (values 0) or map_to_list (values 1): the keys,
// or (k v) lists, pushed onto the argument stack in order, then built into a
// list from the last.
func (c *wasmCompiler) compileMapList(b *wasmBody, values int32) {
	m, start, list := b.local(i64), b.local(i32), b.local(i64)
	pop := func() {
		b.GlobalGet(gSP)
		b.I32Const(8)
		b.Op(wasm.I32Sub)
		b.GlobalSet(gSP)
		b.GlobalGet(gSP)
		b.I64Load(0)
	}
	b.arg(0)
	b.LocalSet(m)
	b.GlobalGet(gSP)
	b.LocalSet(start)
	mapRoot(b, m)
	b.I32Const(values)
	b.Call(c.rt.mapPush)
	b.I64Const(addrEmpty)
	b.LocalSet(list)
	b.until(func() {
		b.GlobalGet(gSP)
		b.LocalGet(start)
		b.Op(wasm.I32Eq)
	}, func() {
		if values == 1 {
			// (k v): the value is on top, then the key.
			v, k := b.local(i64), b.local(i64)
			pop()
			b.LocalSet(v)
			pop()
			b.LocalSet(k)
			b.LocalGet(k)
			b.LocalGet(v)
			b.I64Const(addrEmpty)
			b.Call(c.rt.pair)
			b.Call(c.rt.pair)
		} else {
			pop()
		}
		b.LocalGet(list)
		b.Call(c.rt.pair)
		b.LocalSet(list)
	})
	b.LocalGet(list)
}
