// Package wasm encodes WebAssembly modules in the binary format, with no
// dependencies: enough of the format (version 1, the MVP instruction set)
// for a compiler to emit a module that any wasm runtime can load.
//
// A Module lists its types, imports, functions, table, memory, globals,
// exports and data; Encode writes them as the format's sections. Function
// bodies are written with Code, one method per instruction.
package wasm

// ValType is the type of a value: I32 or I64.
type ValType byte

// Value types.
const (
	I32 ValType = 0x7f
	I64 ValType = 0x7e
)

// BlockType is the result type of a block, loop or if.
type BlockType byte

// Block types: no result, or one value.
const (
	BlockVoid BlockType = 0x40
	BlockI32  BlockType = 0x7f
	BlockI64  BlockType = 0x7e
)

// FuncType is a function signature.
type FuncType struct{ Params, Results []ValType }

// Import is a function the host provides.
type Import struct {
	Module, Name string
	Type         uint32
}

// Global is a global variable of type Type, starting at Init.
type Global struct {
	Type    ValType
	Mutable bool
	Init    int64
}

// Kinds of export.
const (
	ExportFunc   byte = 0x00
	ExportMemory byte = 0x02
)

// Export makes a function or the memory visible to the host by Name.
type Export struct {
	Name  string
	Kind  byte
	Index uint32
}

// Func is a function defined in the module: its type, the locals it declares
// after its parameters, and its body, without the final end.
type Func struct {
	Type   uint32
	Locals []ValType
	Body   Code
}

// Module is a WebAssembly module. Function indices count Imports first, then
// Funcs. Table holds the function indices call_indirect reaches, at table
// indices 0, 1, ...; Data is copied into memory at DataAt.
type Module struct {
	Types       []FuncType
	Imports     []Import
	Funcs       []Func
	Table       []uint32
	MemoryPages uint32
	Globals     []Global
	Exports     []Export
	DataAt      uint32
	Data        []byte
}

// Type returns the index of ft in m.Types, adding it if it isn't there.
func (m *Module) Type(ft FuncType) uint32 {
	for i, t := range m.Types {
		if sameTypes(t.Params, ft.Params) && sameTypes(t.Results, ft.Results) {
			return uint32(i)
		}
	}
	m.Types = append(m.Types, ft)
	return uint32(len(m.Types) - 1)
}

func sameTypes(a, b []ValType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Encode returns the module in the binary format.
func (m *Module) Encode() []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	section := func(id byte, count int, body []byte) {
		if count == 0 {
			return
		}
		contents := append(uleb(nil, uint64(count)), body...)
		out = append(out, id)
		out = uleb(out, uint64(len(contents)))
		out = append(out, contents...)
	}

	var types []byte
	for _, t := range m.Types {
		types = append(types, 0x60)
		types = valTypes(types, t.Params)
		types = valTypes(types, t.Results)
	}
	section(1, len(m.Types), types)

	var imports []byte
	for _, imp := range m.Imports {
		imports = name(imports, imp.Module)
		imports = name(imports, imp.Name)
		imports = append(imports, 0x00)
		imports = uleb(imports, uint64(imp.Type))
	}
	section(2, len(m.Imports), imports)

	var funcs []byte
	for _, f := range m.Funcs {
		funcs = uleb(funcs, uint64(f.Type))
	}
	section(3, len(m.Funcs), funcs)

	if len(m.Table) > 0 {
		table := []byte{0x70, 0x00}
		section(4, 1, uleb(table, uint64(len(m.Table))))
	}

	section(5, 1, uleb([]byte{0x00}, uint64(m.MemoryPages)))

	var globals []byte
	for _, g := range m.Globals {
		globals = append(globals, byte(g.Type))
		if g.Mutable {
			globals = append(globals, 0x01)
		} else {
			globals = append(globals, 0x00)
		}
		if g.Type == I32 {
			globals = append(globals, 0x41)
		} else {
			globals = append(globals, 0x42)
		}
		globals = append(sleb(globals, g.Init), 0x0b)
	}
	section(6, len(m.Globals), globals)

	var exports []byte
	for _, e := range m.Exports {
		exports = name(exports, e.Name)
		exports = append(exports, e.Kind)
		exports = uleb(exports, uint64(e.Index))
	}
	section(7, len(m.Exports), exports)

	if len(m.Table) > 0 {
		elems := []byte{0x00, 0x41, 0x00, 0x0b}
		elems = uleb(elems, uint64(len(m.Table)))
		for _, f := range m.Table {
			elems = uleb(elems, uint64(f))
		}
		section(9, 1, elems)
	}

	var code []byte
	for _, f := range m.Funcs {
		var body []byte
		body = localDecls(body, f.Locals)
		body = append(body, f.Body...)
		body = append(body, 0x0b)
		code = uleb(code, uint64(len(body)))
		code = append(code, body...)
	}
	section(10, len(m.Funcs), code)

	if len(m.Data) > 0 {
		data := []byte{0x00, 0x41}
		data = append(sleb(data, int64(m.DataAt)), 0x0b)
		data = uleb(data, uint64(len(m.Data)))
		section(11, 1, append(data, m.Data...))
	}
	return out
}

// localDecls declares locals as runs of the same type.
func localDecls(out []byte, locals []ValType) []byte {
	var runs [][2]int
	for _, t := range locals {
		if n := len(runs); n > 0 && runs[n-1][1] == int(t) {
			runs[n-1][0]++
		} else {
			runs = append(runs, [2]int{1, int(t)})
		}
	}
	out = uleb(out, uint64(len(runs)))
	for _, run := range runs {
		out = uleb(out, uint64(run[0]))
		out = append(out, byte(run[1]))
	}
	return out
}

func valTypes(out []byte, types []ValType) []byte {
	out = uleb(out, uint64(len(types)))
	for _, t := range types {
		out = append(out, byte(t))
	}
	return out
}

func name(out []byte, s string) []byte {
	return append(uleb(out, uint64(len(s))), s...)
}

// uleb appends v in unsigned LEB128.
func uleb(out []byte, v uint64) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

// sleb appends v in signed LEB128.
func sleb(out []byte, v int64) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}
