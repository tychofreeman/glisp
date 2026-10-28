package glisp

import (
	"fmt"
	"math/rand"
	"testing"

	. "github.com/tychofreeman/go-matchers"
)

func TestMaps(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`(hash_map)`, `{}`},
		{`(hash_map 1 2)`, `{1 2}`},
		{`(map_get (hash_map 1 2) 1)`, `2`},
		{`(map_get (hash_map 1 2) 3)`, `nil`},
		{`(map_get (hash_map) 1 9)`, `9`},
		{`(map_get (hash_map 1 2 1 5) 1)`, `5`},
		{`(map_count (hash_map 1 2 3 4 1 5))`, `2`},
		{`(map_has (hash_map "a" 1) "a")`, `true`},
		{`(map_has (hash_map "a" 1) "b")`, `false`},
		{`(map_count (map_remove (hash_map 1 2 3 4) 1))`, `1`},
		{`(map_count (map_remove (hash_map 1 2) 7))`, `1`},
		{`(map_get (hash_map (quote (a b)) 1 (vector 1) 2) (quote (a b)))`, `1`},
		{`(map_get (hash_map (quote (a b)) 1 (vector 1) 2) (vector 1))`, `2`},
		{`(map_keys (hash_map (quote (a b)) 1))`, `((a b))`},
		{`(map_to_list (hash_map "k" (quote v)))`, `(("k" v))`},
		{`(let (m (hash_map 1 2)) (cons (map_put m 1 3) (cons m nil)))`, `({1 3} {1 2})`},
		{`(eq (hash_map 1 2 3 4) (hash_map 3 4 1 2))`, `true`},
		{`(eq (hash_map 1 2) (hash_map 1 3))`, `false`},
		{`(eq (hash_map 1 2) (hash_map 1 2 3 4))`, `false`},
		{`(eq (hash_map) (vector))`, `false`},
		{`(map? (hash_map))`, `true`},
		{`(map? (vector))`, `false`},
	} {
		AssertThat(t, Show(Process(c.src)), Equals(c.want))
	}
	assertPanicsWith(t, `(hash_map 1)`, `hash_map: expects keys and values in pairs, got 1 argument`)
	assertPanicsWith(t, `(map_put (hash_map) car 1)`, `map_put: can't hash function <function>`)
	assertPanicsWith(t, `(map_get 5 1)`, `map_get: expected a map, got number 5`)
	assertPanicsWith(t, `(map_get (hash_map) 1 2 3)`, `map_get: expects 2 or 3 arguments, got 4`)
}

// Random puts and removes, checked against a Go map after every step.
func TestMapsAgreeWithGoMaps(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	m, want := Map{}, map[int64]int64{}
	for step := 0; step < 20000; step++ {
		k := int64(r.Intn(500))
		if r.Intn(3) == 0 {
			m = m.remove("test", k)
			delete(want, k)
		} else {
			v := int64(r.Intn(1000))
			m = m.put("test", k, v)
			want[k] = v
		}
		if m.count != len(want) {
			t.Fatalf("step %d: count %d, want %d", step, m.count, len(want))
		}
	}
	for k, v := range want {
		if got, found := m.get("test", k); !found || got != v {
			t.Fatalf("key %d: got %v (%v), want %d", k, got, found, v)
		}
	}
	seen := 0
	m.each(func(_, _ interface{}) { seen++ })
	if seen != len(want) {
		t.Errorf("each saw %d entries, want %d", seen, len(want))
	}
}

// A map's shape depends only on its keys, however it was built.
func TestMapsHaveOneShape(t *testing.T) {
	all, odd := Map{}, Map{}
	for k := int64(0); k < 1000; k++ {
		all = all.put("test", k, k)
		if k%2 == 1 {
			odd = odd.put("test", k, k)
		}
	}
	for k := int64(0); k < 1000; k += 2 {
		all = all.remove("test", k)
	}
	if shape(all.root) != shape(odd.root) {
		t.Error("removing the even keys from 0..999 left a different trie than putting the odd ones")
	}
	if Write(all) != Write(odd) {
		t.Error("equal maps print differently")
	}
}

// shape is n's trie written out: each node's bitmaps and keys, then its
// subtries.
func shape(n *champNode) string {
	s := fmt.Sprintf("(%x %x", n.dataMap, n.nodeMap)
	for _, e := range n.data {
		s += " " + Write(e.key)
	}
	for _, sub := range n.nodes {
		s += " " + shape(sub)
	}
	return s + ")"
}

// Keys whose whole hashes collide share a node below the last level.
func TestMapsKeepCollidingKeys(t *testing.T) {
	root := &champNode{}
	for k := int64(0); k < 5; k++ {
		root, _ = root.put(mapEntry{k, k * 10, 42}, 0)
	}
	for k := int64(0); k < 5; k++ {
		if v, found := root.get(k, 42, 0); !found || v != k*10 {
			t.Fatalf("key %d: got %v (%v)", k, v, found)
		}
	}
	for k := int64(0); k < 4; k++ {
		root, _ = root.remove(k, 42, 0)
	}
	// One entry left: it rises back to the root.
	if root.dataMap == 0 || len(root.data) != 1 || len(root.nodes) != 0 {
		t.Errorf("the last colliding key stayed down: %+v", root)
	}
}
