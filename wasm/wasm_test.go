package wasm

import (
	"bytes"
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func TestLEB128(t *testing.T) {
	cases := []struct {
		got, want []byte
	}{
		{uleb(nil, 0), []byte{0x00}},
		{uleb(nil, 624485), []byte{0xe5, 0x8e, 0x26}},
		{sleb(nil, -1), []byte{0x7f}},
		{sleb(nil, 63), []byte{0x3f}},
		{sleb(nil, 64), []byte{0xc0, 0x00}},
		{sleb(nil, -123456), []byte{0xc0, 0xbb, 0x78}},
	}
	for i, c := range cases {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("case %d: got % x, want % x", i, c.got, c.want)
		}
	}
}

func TestTypeReusesEqualSignatures(t *testing.T) {
	m := &Module{}
	a := m.Type(FuncType{Params: []ValType{I64}, Results: []ValType{I64}})
	b := m.Type(FuncType{Params: []ValType{I32}})
	c := m.Type(FuncType{Params: []ValType{I64}, Results: []ValType{I64}})
	if a != 0 || b != 1 || c != 0 || len(m.Types) != 2 {
		t.Errorf("got indices %d %d %d and %d types", a, b, c, len(m.Types))
	}
}

// TestEncodedModuleRuns builds a module that uses every section and runs it.
func TestEncodedModuleRuns(t *testing.T) {
	m := &Module{MemoryPages: 1, DataAt: 16, Data: []byte{5, 7}}
	unary := m.Type(FuncType{Params: []ValType{I64}, Results: []ValType{I64}})
	nullary := m.Type(FuncType{Results: []ValType{I64}})
	m.Imports = []Import{{Module: "env", Name: "double", Type: unary}}
	const double, sumTo, viaTable, main = 0, 1, 2, 3
	m.Globals = []Global{{Type: I64, Mutable: true, Init: 100}}

	// sumTo(n) = n + (n-1) + ... + 1, with a loop and a local.
	var sum Code
	sum.Loop(BlockVoid)
	sum.LocalGet(0)
	sum.Op(I64Eqz)
	sum.If(BlockVoid)
	sum.LocalGet(1)
	sum.Op(Return, End)
	sum.LocalGet(1)
	sum.LocalGet(0)
	sum.Op(I64Add)
	sum.LocalSet(1)
	sum.LocalGet(0)
	sum.I64Const(1)
	sum.Op(I64Sub)
	sum.LocalSet(0)
	sum.Br(0)
	sum.Op(End, Unreachable)

	// viaTable(n) calls table entry 0 (sumTo) with n.
	var table Code
	table.LocalGet(0)
	table.I32Const(0)
	table.CallIndirect(unary)

	// main = double(viaTable(4)) + data[16] * data[17] + global - 100
	//      = 20 + 35 + 0
	var body Code
	body.I64Const(4)
	body.Call(viaTable)
	body.Call(double)
	body.I32Const(0)
	body.I32Load8U(16)
	body.I32Const(0)
	body.I32Load8U(17)
	body.Op(I32Mul, I64ExtendI32U, I64Add)
	body.GlobalGet(0)
	body.I64Const(-100)
	body.Op(I64Add, I64Add)

	m.Funcs = []Func{
		{Type: unary, Locals: []ValType{I64}, Body: sum},
		{Type: unary, Body: table},
		{Type: nullary, Body: body},
	}
	m.Table = []uint32{sumTo}
	m.Exports = []Export{{Name: "main", Kind: ExportFunc, Index: main}, {Name: "memory", Kind: ExportMemory}}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	_, err := r.NewHostModuleBuilder("env").NewFunctionBuilder().
		WithFunc(func(_ context.Context, _ api.Module, x int64) int64 { return 2 * x }).
		Export("double").Instantiate(ctx)
	if err != nil {
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
	if got := int64(results[0]); got != 55 {
		t.Errorf("main returned %d, want 55", got)
	}
}
