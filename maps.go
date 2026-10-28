package glisp

import (
	"fmt"
	"math/bits"
	"slices"
)

// Maps: immutable hash tries in Steindorff and Vinju's CHAMP form. Each node
// branches 32 ways on 5 bits of a key's hash (hash.go), keeping the entries
// that end there inline and the subtries apart, each found by counting the
// bits below its own in a bitmap. An update copies the path from the root to
// the node it changes and shares the rest. A removal folds a subtrie left with
// one entry back into its parent, so a map's shape depends only on its keys:
// maps that eq finds equal look alike, and print alike. The order is each
// node's entries in bit order, then its subtries'; compiled code keeps the
// same.

// Map is a glisp map. The zero Map is the empty map.
type Map struct {
	root  *champNode
	count int
}

// A champNode is a node of a map's trie: the entries for the bits of dataMap
// and the subtries for the bits of nodeMap, each in bit order. Below the last
// level (all 32 bits of the hash used) a node holds the entries whose hashes
// collide, in data alone.
type champNode struct {
	dataMap, nodeMap uint32
	data             []mapEntry
	nodes            []*champNode
	owner            *owner // the builder that may change it in place, if any
}

// An owner is a builder of a map, such as hash_map: the nodes it owns it
// made itself, and no map but the one it builds can reach them, so it may
// change them in place. It is not empty, so that each has its own address.
type owner struct{ _ byte }

type mapEntry struct {
	key, value interface{}
	hash       uint32
}

const champBits = 5

// champBit is the bit of hash's slot at the level of shift.
func champBit(hash uint32, shift uint) uint32 {
	return 1 << (hash >> shift & (1<<champBits - 1))
}

// below is how many bits of bitmap are below bit: the slot's index.
func below(bitmap, bit uint32) int {
	return bits.OnesCount32(bitmap & (bit - 1))
}

func (n *champNode) get(key interface{}, hash uint32, shift uint) (interface{}, bool) {
	if shift >= 32 {
		for _, e := range n.data {
			if equal(e.key, key) {
				return e.value, true
			}
		}
		return nil, false
	}
	bit := champBit(hash, shift)
	switch {
	case n.dataMap&bit != 0:
		if e := n.data[below(n.dataMap, bit)]; equal(e.key, key) {
			return e.value, true
		}
	case n.nodeMap&bit != 0:
		return n.nodes[below(n.nodeMap, bit)].get(key, hash, shift+champBits)
	}
	return nil, false
}

// put returns n with e in it, and whether e's key is new.
func (n *champNode) put(e mapEntry, shift uint, o *owner) (*champNode, bool) {
	own := o != nil && n.owner == o
	if shift >= 32 {
		for i, d := range n.data {
			if equal(d.key, e.key) {
				return n.with(o, 0, 0, replaced(own, n.data, i, e), nil), false
			}
		}
		return n.with(o, 0, 0, inserted(own, n.data, len(n.data), e), nil), true
	}
	bit := champBit(e.hash, shift)
	switch {
	case n.dataMap&bit != 0:
		i := below(n.dataMap, bit)
		d := n.data[i]
		if equal(d.key, e.key) {
			return n.with(o, n.dataMap, n.nodeMap, replaced(own, n.data, i, e), n.nodes), false
		}
		// Two keys in one slot: they move down into a subtrie.
		nodeMap := n.nodeMap | bit
		sub := pairNode(d, e, shift+champBits, o)
		return n.with(o, n.dataMap&^bit, nodeMap, removed(own, n.data, i), inserted(own, n.nodes, below(nodeMap, bit), sub)), true
	case n.nodeMap&bit != 0:
		i := below(n.nodeMap, bit)
		sub, added := n.nodes[i].put(e, shift+champBits, o)
		return n.with(o, n.dataMap, n.nodeMap, n.data, replaced(own, n.nodes, i, sub)), added
	}
	dataMap := n.dataMap | bit
	return n.with(o, dataMap, n.nodeMap, inserted(own, n.data, below(dataMap, bit), e), n.nodes), true
}

// pairNode is the subtrie, at the level of shift, of two entries whose hashes
// agree above it.
func pairNode(a, b mapEntry, shift uint, o *owner) *champNode {
	if shift >= 32 {
		return &champNode{data: []mapEntry{a, b}, owner: o}
	}
	ba, bb := champBit(a.hash, shift), champBit(b.hash, shift)
	switch {
	case ba == bb:
		return &champNode{nodeMap: ba, nodes: []*champNode{pairNode(a, b, shift+champBits, o)}, owner: o}
	case ba < bb:
		return &champNode{dataMap: ba | bb, data: []mapEntry{a, b}, owner: o}
	}
	return &champNode{dataMap: ba | bb, data: []mapEntry{b, a}, owner: o}
}

// with is n with the fields given: n itself, changed, if o owns it, and
// otherwise a new node that o owns. A new node owned by a builder gets its
// own copies of the slices, which it may then change in place.
func (n *champNode) with(o *owner, dataMap, nodeMap uint32, data []mapEntry, nodes []*champNode) *champNode {
	if o != nil && n.owner == o {
		n.dataMap, n.nodeMap, n.data, n.nodes = dataMap, nodeMap, data, nodes
		return n
	}
	if o != nil {
		data, nodes = slices.Clone(data), slices.Clone(nodes)
	}
	return &champNode{dataMap, nodeMap, data, nodes, o}
}

// remove returns n without key, and whether it had it.
func (n *champNode) remove(key interface{}, hash uint32, shift uint) (*champNode, bool) {
	if shift >= 32 {
		for i, d := range n.data {
			if equal(d.key, key) {
				return &champNode{data: removed(false, n.data, i)}, true
			}
		}
		return n, false
	}
	bit := champBit(hash, shift)
	switch {
	case n.dataMap&bit != 0:
		i := below(n.dataMap, bit)
		if !equal(n.data[i].key, key) {
			return n, false
		}
		return &champNode{n.dataMap &^ bit, n.nodeMap, removed(false, n.data, i), n.nodes, nil}, true
	case n.nodeMap&bit != 0:
		i := below(n.nodeMap, bit)
		sub, gone := n.nodes[i].remove(key, hash, shift+champBits)
		if !gone {
			return n, false
		}
		if sub.nodeMap == 0 && len(sub.data) == 1 {
			// One entry left below: it comes back up into this node.
			dataMap := n.dataMap | bit
			return &champNode{dataMap, n.nodeMap &^ bit, inserted(false, n.data, below(dataMap, bit), sub.data[0]), removed(false, n.nodes, i), nil}, true
		}
		return &champNode{n.dataMap, n.nodeMap, n.data, replaced(false, n.nodes, i, sub), nil}, true
	}
	return n, false
}

// each calls f with each entry, in the map's order.
func (n *champNode) each(f func(mapEntry)) {
	for _, e := range n.data {
		f(e)
	}
	for _, sub := range n.nodes {
		sub.each(f)
	}
}

// inserted, replaced and removed are s with x inserted at i, x at i, or i's
// element removed: s itself, changed, if own, and otherwise a copy.
func inserted[T any](own bool, s []T, i int, x T) []T {
	if own {
		return slices.Insert(s, i, x)
	}
	c := make([]T, 0, len(s)+1)
	return append(append(append(c, s[:i]...), x), s[i:]...)
}

func replaced[T any](own bool, s []T, i int, x T) []T {
	if own {
		s[i] = x
		return s
	}
	c := append([]T(nil), s...)
	c[i] = x
	return c
}

func removed[T any](own bool, s []T, i int) []T {
	if own {
		return slices.Delete(s, i, i+1)
	}
	c := make([]T, 0, len(s)-1)
	return append(append(c, s[:i]...), s[i+1:]...)
}

// get is the value of key in m, if m has it. fn names the builtin, for the
// error a key that can't be hashed gives.
func (m Map) get(fn string, key interface{}) (interface{}, bool) {
	hash := hashValue(fn, key)
	if m.root == nil {
		return nil, false
	}
	return m.root.get(key, hash, 0)
}

// put is m with key's value set.
func (m Map) put(fn string, key, value interface{}, o *owner) Map {
	root := m.root
	if root == nil {
		root = &champNode{}
	}
	root, added := root.put(mapEntry{key, value, hashValue(fn, key)}, 0, o)
	if added {
		return Map{root, m.count + 1}
	}
	return Map{root, m.count}
}

// remove is m without key.
func (m Map) remove(fn string, key interface{}) Map {
	hash := hashValue(fn, key)
	if m.root == nil {
		return m
	}
	root, gone := m.root.remove(key, hash, 0)
	if !gone {
		return m
	}
	return Map{root, m.count - 1}
}

// each calls f with each key and value, in the map's order.
func (m Map) each(f func(key, value interface{})) {
	if m.root != nil {
		m.root.each(func(e mapEntry) { f(e.key, e.value) })
	}
}

// equalMaps reports whether a and b have the same keys, with equal values.
func equalMaps(a, b Map) bool {
	if a.count != b.count {
		return false
	}
	same := true
	a.each(func(key, value interface{}) {
		if v, found := b.get("eq", key); !found || !equal(v, value) {
			same = false
		}
	})
	return same
}

// makeMap is (hash_map k v ...).
func makeMap(_ *Scope, args List) interface{} {
	if len(args)%2 != 0 {
		panic(fmt.Sprintf("hash_map: expects keys and values in pairs, got %s", arguments(len(args))))
	}
	m, o := Map{}, &owner{}
	for i := 0; i < len(args); i += 2 {
		m = m.put("hash_map", args[i], args[i+1], o)
	}
	return m
}

// mapGet is (map_get m k), k's value or nil, and (map_get m k default).
func mapGet(_ *Scope, args List) interface{} {
	if value, found := args[0].(Map).get("map_get", args[1]); found {
		return value
	}
	if len(args) == 3 {
		return args[2]
	}
	return nil
}

// mapPut is (map_put m k v): m with k's value v.
func mapPut(_ *Scope, args List) interface{} {
	return args[0].(Map).put("map_put", args[1], args[2], nil)
}

// mapRemove is (map_remove m k): m without k.
func mapRemove(_ *Scope, args List) interface{} {
	return args[0].(Map).remove("map_remove", args[1])
}

// mapHas is (map_has m k): whether m has k.
func mapHas(_ *Scope, args List) interface{} {
	_, found := args[0].(Map).get("map_has", args[1])
	return found
}

// mapCount is (map_count m): how many keys m has.
func mapCount(_ *Scope, args List) interface{} {
	return int64(args[0].(Map).count)
}

// mapKeys is (map_keys m): m's keys, in its order.
func mapKeys(_ *Scope, args List) interface{} {
	keys := List{}
	args[0].(Map).each(func(key, _ interface{}) { keys = append(keys, key) })
	return keys
}

// mapToList is (map_to_list m): m's entries as (k v) lists, in its order.
func mapToList(_ *Scope, args List) interface{} {
	entries := List{}
	args[0].(Map).each(func(key, value interface{}) { entries = append(entries, List{key, value}) })
	return entries
}
