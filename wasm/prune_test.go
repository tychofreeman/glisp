package wasm

import (
	"bytes"
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
)

// Prune keeps what an export or a live table entry reaches, renumbers the
// calls, and leaves a dead table entry at a trap.
func TestPruneKeepsWhatIsReached(t *testing.T) {
	m := &Module{}
	unary := m.Type(FuncType{Params: []ValType{I64}, Results: []ValType{I64}})
	nullary := m.Type(FuncType{Results: []ValType{I64}})
	m.Imports = []Import{{Module: "env", Name: "same", Type: unary}}
	const plusOne, twice, dead, viaDead, inTable, main = 1, 2, 3, 4, 5, 6
	var p, tw, d, vd, in, mn Code
	p.LocalGet(0)
	p.Call(0)
	p.I64Const(1)
	p.Op(I64Add)
	tw.LocalGet(0)
	tw.Call(plusOne)
	tw.Call(plusOne)
	d.LocalGet(0)
	d.Call(twice)
	vd.LocalGet(0)
	vd.Call(dead)
	in.LocalGet(0)
	in.Call(twice)
	// main = table[0](40), which is inTable(40) = 42
	mn.I64Const(40)
	mn.I32Const(0)
	mn.CallIndirect(unary)
	m.Funcs = []Func{{Type: unary, Body: p}, {Type: unary, Body: tw}, {Type: unary, Body: d},
		{Type: unary, Body: vd}, {Type: unary, Body: in}, {Type: nullary, Body: mn}}
	m.Table = []uint32{inTable, viaDead}
	m.Exports = []Export{{Name: "main", Kind: ExportFunc, Index: main}}

	m.Prune(func(entry int) bool { return entry == 0 })

	if len(m.Funcs) != 5 {
		t.Errorf("kept %d functions, want plusOne, twice, inTable, main and a trap", len(m.Funcs))
	}
	if m.Table[0] != 3 || m.Table[1] != 5 || m.Exports[0].Index != 4 {
		t.Errorf("table %v and main at %d, want [3 5] and 4", m.Table, m.Exports[0].Index)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	if _, err := r.NewHostModuleBuilder("env").NewFunctionBuilder().
		WithFunc(func(x int64) int64 { return x }).Export("same").Instantiate(ctx); err != nil {
		t.Fatal(err)
	}
	mod, err := r.Instantiate(ctx, m.Encode())
	if err != nil {
		t.Fatal(err)
	}
	results, err := mod.ExportedFunction("main").Call(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := int64(results[0]); got != 42 {
		t.Errorf("main returned %d, want 42", got)
	}
}

// Every instruction Code writes is read back whole: renumbering no calls
// gives the same bytes.
func TestCallsReadsEveryInstruction(t *testing.T) {
	var c Code
	c.Block(BlockVoid)
	c.Loop(BlockI32)
	c.If(BlockI64)
	c.Op(Else, End, End, End, Nop, Drop, Select, Return, Unreachable, I32Add, F64Sqrt, F64ReinterpretI64)
	c.I32Const(-5000000)
	c.I64Const(1 << 60)
	c.LocalGet(300)
	c.LocalSet(1)
	c.LocalTee(2)
	c.GlobalGet(3)
	c.GlobalSet(200)
	c.Call(70000)
	c.CallIndirect(4)
	c.Br(1)
	c.BrIf(2)
	c.I32Load(100000)
	c.I64Load(8)
	c.I32Load8U(0)
	c.I32Store(4)
	c.I64Store(16)
	c.I32Store8(1)
	c.MemorySize()
	c.MemoryGrow()
	c.MemoryCopy()
	c.Call(5)
	var called []uint32
	same := renumberCalls(c, func(f uint32) uint32 {
		called = append(called, f)
		return f
	})
	if !bytes.Equal(same, c) {
		t.Errorf("renumbering no calls changed the code:\n% x\n% x", same, c)
	}
	if len(called) != 2 || called[0] != 70000 || called[1] != 5 {
		t.Errorf("found calls %v, want [70000 5]", called)
	}
}
