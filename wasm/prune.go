package wasm

import "fmt"

// Prune removes the functions the module can't reach: those that no export,
// and no table entry live reports true for, calls, directly or through other
// functions. A table entry that isn't live is left at a function that traps,
// so that the others keep their indices. Calls, the table and the exports
// are renumbered; imports stay.
func (m *Module) Prune(live func(entry int) bool) {
	imports := uint32(len(m.Imports))
	reached := make([]bool, int(imports)+len(m.Funcs))
	var reach func(f uint32)
	reach = func(f uint32) {
		if reached[f] {
			return
		}
		reached[f] = true
		if f >= imports {
			renumberCalls(m.Funcs[f-imports].Body, func(g uint32) uint32 {
				reach(g)
				return g
			})
		}
	}
	for f := uint32(0); f < imports; f++ {
		reached[f] = true
	}
	for _, e := range m.Exports {
		if e.Kind == ExportFunc {
			reach(e.Index)
		}
	}
	trap := -1
	for i, f := range m.Table {
		if live(i) {
			reach(f)
		} else if trap < 0 {
			trap = int(f)
		}
	}
	index := make([]uint32, len(reached))
	var kept []Func
	for f, r := range reached {
		switch {
		case f < int(imports):
			index[f] = uint32(f)
		case r:
			index[f] = imports + uint32(len(kept))
			kept = append(kept, m.Funcs[f-int(imports)])
		}
	}
	for i := range kept {
		kept[i].Body = renumberCalls(kept[i].Body, func(g uint32) uint32 { return index[g] })
	}
	trapIndex := imports + uint32(len(kept))
	if trap >= 0 {
		kept = append(kept, Func{Type: m.Funcs[trap-int(imports)].Type, Body: Code{Unreachable}})
	}
	for i, f := range m.Table {
		if live(i) {
			m.Table[i] = index[f]
		} else {
			m.Table[i] = trapIndex
		}
	}
	for i, e := range m.Exports {
		if e.Kind == ExportFunc {
			m.Exports[i].Index = index[e.Index]
		}
	}
	m.Funcs = kept
}

// renumberCalls is code with each call's function f replaced by to(f).
// It reads the instructions Code writes, and panics at any other.
func renumberCalls(code Code, to func(f uint32) uint32) Code {
	out := make(Code, 0, len(code))
	for i := 0; i < len(code); {
		op := code[i]
		start := i
		i++
		switch {
		case op == 0x10: // call
			f, n := readUleb(code[i:])
			i += n
			out = uleb(append(out, op), uint64(to(uint32(f))))
			continue
		case op == 0x02 || op == 0x03 || op == 0x04: // block, loop, if: a block type
			i++
		case op == 0x0c || op == 0x0d || op >= 0x20 && op <= 0x24: // a label, local or global
			i += lebLen(code[i:])
		case op == 0x11: // call_indirect: a type, then the table
			i += lebLen(code[i:]) + 1
		case op == 0x41 || op == 0x42: // constants
			i += lebLen(code[i:])
		case op >= 0x28 && op <= 0x3e: // loads and stores: alignment, offset
			i += lebLen(code[i:])
			i += lebLen(code[i:])
		case op == 0x3f || op == 0x40: // memory.size, memory.grow
			i++
		case op == 0xfc && code[i] == 0x0a: // memory.copy
			i += 3
		case op <= 0x01 || op == 0x05 || op == 0x0b || op == 0x0f || op == 0x1a || op == 0x1b || op >= 0x45 && op <= 0xbf:
		default:
			panic(fmt.Sprintf("wasm: can't read instruction 0x%02x", op))
		}
		out = append(out, code[start:i]...)
	}
	return out
}

// readUleb reads an unsigned LEB128 number, and how many bytes it took.
func readUleb(b []byte) (uint64, int) {
	var v uint64
	for i, x := range b {
		v |= uint64(x&0x7f) << (7 * i)
		if x&0x80 == 0 {
			return v, i + 1
		}
	}
	panic("wasm: LEB128 number runs past the code")
}

// lebLen is how many bytes the LEB128 number at the start of b takes.
func lebLen(b []byte) int {
	_, n := readUleb(b)
	return n
}
