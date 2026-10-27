package glisp

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/tychofreeman/glisp/wasm"
)

// CompileWasm compiles the glisp program in input, with the stdlib, to a
// WebAssembly module for WASI (wasi_snapshot_preview1). Running the module
// prints what the program prints with p, then "=> " and the value of its
// last form, as cmd/glisp does; an error is printed as "glisp: ..." on
// stderr, with exit status 1.
//
// Macros are expanded while compiling, so a program's macros, and the
// functions they call (top-level defs of lambdas), run in the interpreter.
func CompileWasm(input string) (module []byte, err error) {
	defer recoverError(&err)
	return compileWasm(TokenizeString(input)), nil
}

// CompileWasmFile is CompileWasm for the program in the file fname.
func CompileWasmFile(fname string) ([]byte, error) {
	program, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return CompileWasm(string(program))
}

// Compiled values are i64s. An odd value is an integer n stored as 2n+1; any
// other value is the address of an object in linear memory, whose first i32
// is its tag. The value 0 marks a variable that has no value yet.
const (
	tagInt = iota // integers have no object; tagOf reports this tag
	tagNil
	tagTrue
	tagFalse
	tagEmpty   // the empty list
	tagPair    // a list cell: car at +8, cdr at +16
	tagString  // length at +4, bytes from +8
	tagSymbol  // like a string
	tagClosure // table index at +4, count at +8, captured cells from +16
	tagCell    // a captured variable: its value at +8
)

// The memory layout. Static objects (the four constants, strings, quoted
// data, closures that capture nothing) start at address 8, followed by the
// output buffer, the argument stack and the heap.
const (
	addrNil     = 8
	addrTrue    = 16
	addrFalse   = 24
	addrEmpty   = 32
	addrIovec   = 40 // an iovec for fd_write
	addrWritten = 48 // where fd_write puts the count written
	addrDigits  = 56 // 24 bytes for printing an integer
	staticStart = 80

	outCap     = 4096
	stackSize  = 8 << 20
	stackGuard = 1 << 16
)

// Global variables of the runtime; the program's top-level names follow.
const (
	gSP       = iota // argument stack pointer
	gHP              // next free heap address
	gOutLen          // bytes waiting in the output buffer
	gFD              // where the output buffer goes: 1, or 2 for an error
	gOutBuf          // address of the output buffer
	gStackEnd        // the argument stack is full past here
	gNames
)

// builtinNames are the builtin functions, compiled as runtime functions.
var builtinNames = []string{"car", "cdr", "atom", "cons", "plus", "-", "*", "/", "mod", "<", ">", "<=", ">=",
	"number?", "string?", "symbol?", "list?", "function?", "boolean?", "nil?", "eq", "apply", "p"}

type wasmCompiler struct {
	m        *wasm.Module
	scope    *Scope // the expander's: tells macros from values
	data     []byte // memory from address 0, as the module starts
	strings  map[string]uint32
	symbols  map[string]uint32
	globals  map[string]uint32
	closures map[uint32]uint32 // static closure of each table index
	fnType   uint32            // every glisp function: (self, args, argc) -> value
	rt       wasmRuntime
}

func compileWasm(tokens List) []byte {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}}
	expander := NewExpander(scope)
	expand := func(form interface{}) Node {
		node := expander.Expand(form)
		if def, isDef := node.(Def); isDef {
			if _, isLambda := def.Value.(Lambda); isLambda {
				Eval(node, scope) // so later macros can call it
			}
		}
		return Resolve(node)
	}
	var stdlibNodes, programNodes []Node
	for _, form := range stdlibForms {
		stdlibNodes = append(stdlibNodes, expand(form))
	}
	for _, token := range tokens {
		programNodes = append(programNodes, expand(Read(token)))
	}

	c := newWasmCompiler(scope)
	main := &wasmFunc{c: c}
	main.scratch = main.local(wasm.I64)
	main.tmp32 = main.local(wasm.I32)
	for _, node := range stdlibNodes {
		main.expr(node)
		main.code.Op(wasm.Drop)
	}
	main.body(programNodes)
	mainIndex := c.declare(wasm.FuncType{Results: []wasm.ValType{wasm.I64}})
	c.define(mainIndex, main.locals, main.code)

	var start wasm.Code
	start.Call(mainIndex)
	start.LocalSet(0)
	c.text(&start, "=> ")
	start.LocalGet(0)
	start.Call(c.rt.putShow)
	start.I32Const('\n')
	start.Call(c.rt.putByte)
	start.Call(c.rt.flush)
	startIndex := c.declare(wasm.FuncType{})
	c.define(startIndex, []wasm.ValType{wasm.I64}, start)
	c.m.Exports = []wasm.Export{
		{Name: "_start", Kind: wasm.ExportFunc, Index: startIndex},
		{Name: "memory", Kind: wasm.ExportMemory},
	}
	return c.finish()
}

func newWasmCompiler(scope *Scope) *wasmCompiler {
	c := &wasmCompiler{
		m:        &wasm.Module{},
		scope:    scope,
		data:     make([]byte, staticStart),
		strings:  map[string]uint32{},
		symbols:  map[string]uint32{},
		globals:  map[string]uint32{},
		closures: map[uint32]uint32{},
	}
	c.put32(addrNil, tagNil)
	c.put32(addrTrue, tagTrue)
	c.put32(addrFalse, tagFalse)
	c.put32(addrEmpty, tagEmpty)
	for range gNames {
		c.m.Globals = append(c.m.Globals, wasm.Global{Type: wasm.I32, Mutable: true})
	}
	c.fnType = c.m.Type(wasm.FuncType{Params: []wasm.ValType{wasm.I64, wasm.I32, wasm.I32}, Results: []wasm.ValType{wasm.I64}})
	c.runtime()
	return c
}

// finish lays out memory after the static data and encodes the module.
func (c *wasmCompiler) finish() []byte {
	outBuf := align8(len(c.data))
	stack := outBuf + outCap
	heap := stack + stackSize
	c.m.Globals[gSP].Init = int64(stack)
	c.m.Globals[gHP].Init = int64(heap)
	c.m.Globals[gFD].Init = 1
	c.m.Globals[gOutBuf].Init = int64(outBuf)
	c.m.Globals[gStackEnd].Init = int64(stack + stackSize - stackGuard)
	c.m.MemoryPages = uint32(heap/65536 + 1)
	c.m.DataAt = 8
	c.m.Data = c.data[8:]
	return c.m.Encode()
}

func align8(n int) int { return (n + 7) &^ 7 }

// declare adds a function of type ft and returns its index; define gives it
// a body.
func (c *wasmCompiler) declare(ft wasm.FuncType) uint32 {
	c.m.Funcs = append(c.m.Funcs, wasm.Func{Type: c.m.Type(ft)})
	return uint32(len(c.m.Imports) + len(c.m.Funcs) - 1)
}

func (c *wasmCompiler) define(f uint32, locals []wasm.ValType, body wasm.Code) {
	i := f - uint32(len(c.m.Imports))
	c.m.Funcs[i].Locals = locals
	c.m.Funcs[i].Body = body
}

// declareFn declares a glisp function and puts it in the table, returning
// its function and table indices.
func (c *wasmCompiler) declareFn() (uint32, uint32) {
	f := c.declare(c.m.Types[c.fnType])
	c.m.Table = append(c.m.Table, f)
	return f, uint32(len(c.m.Table) - 1)
}

// static reserves size bytes of static memory, 8-aligned.
func (c *wasmCompiler) static(size int) uint32 {
	addr := align8(len(c.data))
	c.data = append(c.data, make([]byte, addr+size-len(c.data))...)
	return uint32(addr)
}

func (c *wasmCompiler) put32(addr uint32, v uint32) {
	binary.LittleEndian.PutUint32(c.data[addr:], v)
}

func (c *wasmCompiler) put64(addr uint32, v int64) {
	binary.LittleEndian.PutUint64(c.data[addr:], uint64(v))
}

// str is the address of a static string object holding s.
func (c *wasmCompiler) str(s string) uint32 { return c.text8(c.strings, tagString, s) }

func (c *wasmCompiler) sym(s string) uint32 { return c.text8(c.symbols, tagSymbol, s) }

func (c *wasmCompiler) text8(interned map[string]uint32, tag uint32, s string) uint32 {
	if addr, found := interned[s]; found {
		return addr
	}
	addr := c.static(8 + len(s))
	c.put32(addr, tag)
	c.put32(addr+4, uint32(len(s)))
	copy(c.data[addr+8:], s)
	interned[s] = addr
	return addr
}

// closure is the static closure, capturing nothing, of table index t.
func (c *wasmCompiler) closure(t uint32) uint32 {
	if addr, found := c.closures[t]; found {
		return addr
	}
	addr := c.static(16)
	c.put32(addr, tagClosure)
	c.put32(addr+4, t)
	c.closures[t] = addr
	return addr
}

func fixnum(n int64) int64 { return n<<1 | 1 }

// constant is the compiled value of a Const.
func (c *wasmCompiler) constant(v interface{}) int64 {
	switch v := v.(type) {
	case int64:
		return fixnum(v)
	case string:
		return int64(c.str(v))
	case bool:
		if v {
			return addrTrue
		}
		return addrFalse
	case nil:
		return addrNil
	}
	panic(fmt.Sprintf("wasm: can't compile the constant %T %v", v, v))
}

// datum is the compiled value of quoted data, built in static memory.
func (c *wasmCompiler) datum(d interface{}) int64 {
	switch d := d.(type) {
	case Symbol:
		return int64(c.sym(d.Str()))
	case List:
		list := int64(addrEmpty)
		for i := len(d) - 1; i >= 0; i-- {
			car := c.datum(d[i])
			addr := c.static(24)
			c.put32(addr, tagPair)
			c.put64(addr+8, car)
			c.put64(addr+16, list)
			list = int64(addr)
		}
		return list
	}
	return c.constant(d)
}

// global is the index of the global holding the top-level name. A builtin's
// starts with the builtin; any other starts with no value.
func (c *wasmCompiler) global(name string) uint32 {
	if g, found := c.globals[name]; found {
		return g
	}
	var init int64
	if f, isBuiltin := c.rt.builtins[name]; isBuiltin {
		init = int64(c.closure(f))
	} else if v, isValue := builtins[name]; isValue {
		init = c.constant(v)
	}
	c.m.Globals = append(c.m.Globals, wasm.Global{Type: wasm.I64, Mutable: true, Init: init})
	c.globals[name] = uint32(len(c.m.Globals) - 1)
	return c.globals[name]
}

// text appends code that prints s to the output buffer.
func (c *wasmCompiler) text(code *wasm.Code, s string) {
	code.I32Const(int32(c.str(s)))
	code.Call(c.rt.putStr)
}

// fail appends code that reports the error "glisp: " + msg + whatever more
// prints, and exits.
func (c *wasmCompiler) fail(code *wasm.Code, msg string, more func()) {
	code.Call(c.rt.failBegin)
	c.text(code, msg)
	if more != nil {
		more()
	}
	code.Call(c.rt.failEnd)
	code.Op(wasm.Unreachable)
}

// wasmFunc compiles the body of main or of one lambda.
type wasmFunc struct {
	c         *wasmCompiler
	code      wasm.Code
	nparams   uint32
	locals    []wasm.ValType
	lambda    *Lambda // nil in main
	slots     []string
	slotLocal []uint32
	boxed     []bool // slots that lambdas inside this one capture, kept in cells
	scratch   uint32 // an i64 local
	tmp32     uint32 // an i32 local
	blocks    int    // blocks open inside a lambda's loop, for a tail call's br
}

func (f *wasmFunc) local(t wasm.ValType) uint32 {
	f.locals = append(f.locals, t)
	return f.nparams + uint32(len(f.locals)-1)
}

// body compiles nodes, leaving the last one's value, or nil if there are none.
func (f *wasmFunc) body(nodes []Node) {
	if len(nodes) == 0 {
		f.code.I64Const(addrNil)
	}
	for i, node := range nodes {
		if i < len(nodes)-1 {
			f.expr(node)
			f.code.Op(wasm.Drop)
		} else if f.lambda != nil {
			f.tailExpr(node)
		} else {
			f.expr(node)
		}
	}
}

func (f *wasmFunc) expr(node Node) {
	switch n := node.(type) {
	case Const:
		f.code.I64Const(f.c.constant(n.Value))
	case Quote:
		f.code.I64Const(f.c.datum(n.Datum))
	case Ref:
		f.ref(n)
	case If:
		f.expr(n.Cond)
		f.code.I64Const(addrTrue)
		f.code.Op(wasm.I64Eq)
		f.code.If(wasm.BlockI64)
		f.expr(n.Then)
		f.code.Op(wasm.Else)
		f.expr(n.Else)
		f.code.Op(wasm.End)
	case Lambda:
		f.lambdaValue(n)
	case Def:
		f.def(n)
	case Apply:
		f.apply(n)
	default:
		panic(fmt.Sprintf("wasm: unknown node %T %v", node, node))
	}
}

func (f *wasmFunc) slot(name string) int {
	for i, s := range f.slots {
		if s == name {
			return i
		}
	}
	return -1
}

func (f *wasmFunc) captured(name string) uint32 {
	for k, captured := range f.lambda.Captured {
		if captured == name {
			return uint32(k)
		}
	}
	panic("wasm: " + name + " is not captured")
}

func (f *wasmFunc) ref(n Ref) {
	switch {
	case n.Loc == nil:
		if value, found := f.c.scope.lookup(symbol(n.Name)); found {
			if _, isMacro := value.(Macro); isMacro {
				f.c.fail(&f.code, n.Name+" is a macro, not a value", nil)
				return
			}
		}
		f.code.GlobalGet(f.c.global(n.Name))
		f.checkBound(n.Name)
	case n.Loc.Depth == 0:
		f.code.LocalGet(f.slotLocal[n.Loc.Index])
		if f.boxed[n.Loc.Index] {
			f.code.Op(wasm.I32WrapI64)
			f.code.I64Load(8)
		}
		if n.Loc.Index >= len(f.lambda.Params) {
			f.checkBound(n.Name)
		}
	default:
		f.cellOf(n.Name)
		f.code.Op(wasm.I32WrapI64)
		f.code.I64Load(8)
		f.checkBound(n.Name)
	}
}

// checkBound fails if the value on the stack is 0: a name not yet defined.
func (f *wasmFunc) checkBound(name string) {
	f.code.LocalTee(f.scratch)
	f.code.Op(wasm.I64Eqz)
	f.code.If(wasm.BlockVoid)
	f.c.fail(&f.code, "Cannot resolve symbol "+name, nil)
	f.code.Op(wasm.End)
	f.code.LocalGet(f.scratch)
}

// cellOf leaves the cell of name, a slot of this lambda that inner lambdas
// capture or a variable this lambda captures.
func (f *wasmFunc) cellOf(name string) {
	if i := f.slot(name); i >= 0 {
		f.code.LocalGet(f.slotLocal[i])
		return
	}
	f.code.LocalGet(0)
	f.code.Op(wasm.I32WrapI64)
	f.code.I64Load(16 + 8*f.captured(name))
}

func (f *wasmFunc) def(n Def) {
	if f.lambda == nil {
		f.expr(n.Value)
		f.code.GlobalSet(f.c.global(n.Name))
	} else if i := f.slot(n.Name); f.boxed[i] {
		f.code.LocalGet(f.slotLocal[i])
		f.code.Op(wasm.I32WrapI64)
		f.expr(n.Value)
		f.code.I64Store(8)
	} else {
		f.expr(n.Value)
		f.code.LocalSet(f.slotLocal[i])
	}
	f.code.I64Const(addrEmpty)
}

// apply pushes the arguments on the argument stack and calls invoke.
func (f *wasmFunc) apply(n Apply) {
	fn := f.local(wasm.I64)
	f.expr(n.Fn)
	f.code.LocalSet(fn)
	for _, arg := range n.Args {
		f.code.GlobalGet(gSP)
		f.expr(arg)
		f.code.I64Store(0)
		f.code.GlobalGet(gSP)
		f.code.I32Const(8)
		f.code.Op(wasm.I32Add)
		f.code.GlobalSet(gSP)
	}
	f.invoke(fn, n)
}

// invoke calls the function in local fn with n's arguments, already on the
// argument stack, and pops them.
func (f *wasmFunc) invoke(fn uint32, n Apply) {
	size := int32(8 * len(n.Args))
	f.code.LocalGet(fn)
	f.code.GlobalGet(gSP)
	f.code.I32Const(size)
	f.code.Op(wasm.I32Sub)
	f.code.I32Const(int32(len(n.Args)))
	f.code.I32Const(int32(f.c.str(unresolved(n).String())))
	f.code.Call(f.c.rt.invoke)
	if size > 0 {
		f.code.GlobalGet(gSP)
		f.code.I32Const(size)
		f.code.Op(wasm.I32Sub)
		f.code.GlobalSet(gSP)
	}
}

// tailExpr compiles node, whose value the lambda returns. In tail position,
// a call to the lambda itself is a jump back to the start of its loop.
func (f *wasmFunc) tailExpr(node Node) {
	switch n := node.(type) {
	case If:
		f.expr(n.Cond)
		f.code.I64Const(addrTrue)
		f.code.Op(wasm.I64Eq)
		f.code.If(wasm.BlockI64)
		f.blocks++
		f.tailExpr(n.Then)
		f.code.Op(wasm.Else)
		f.tailExpr(n.Else)
		f.blocks--
		f.code.Op(wasm.End)
	case Apply:
		if len(n.Args) == len(f.lambda.Params) {
			f.tailApply(n)
		} else {
			f.apply(n)
		}
	default:
		f.expr(node)
	}
}

// tailApply compiles a call in tail position. If the function called is
// this closure itself (local 0), its arguments become the parameters and it
// jumps back to the top of the loop; otherwise it is an ordinary call.
func (f *wasmFunc) tailApply(n Apply) {
	fn := f.local(wasm.I64)
	f.expr(n.Fn)
	f.code.LocalSet(fn)
	args := make([]uint32, len(n.Args))
	for i, arg := range n.Args {
		args[i] = f.local(wasm.I64)
		f.expr(arg)
		f.code.LocalSet(args[i])
	}
	f.code.LocalGet(fn)
	f.code.LocalGet(0)
	f.code.Op(wasm.I64Eq)
	f.code.If(wasm.BlockI64)
	for i, arg := range args {
		f.code.LocalGet(arg)
		f.code.LocalSet(f.slotLocal[i])
	}
	f.code.Br(uint32(f.blocks + 1))
	f.code.Op(wasm.Else)
	for _, arg := range args {
		f.code.GlobalGet(gSP)
		f.code.LocalGet(arg)
		f.code.I64Store(0)
		f.code.GlobalGet(gSP)
		f.code.I32Const(8)
		f.code.Op(wasm.I32Add)
		f.code.GlobalSet(gSP)
	}
	f.invoke(fn, n)
	f.code.Op(wasm.End)
}

// lambdaValue compiles n's body as a function and leaves a closure of it.
func (f *wasmFunc) lambdaValue(n Lambda) {
	table := f.c.compileLambda(n)
	if len(n.Captured) == 0 {
		f.code.I64Const(int64(f.c.closure(table)))
		return
	}
	f.code.I32Const(int32(16 + 8*len(n.Captured)))
	f.code.Call(f.c.rt.alloc)
	f.code.LocalSet(f.tmp32)
	for offset, v := range []int32{tagClosure, int32(table), int32(len(n.Captured))} {
		f.code.LocalGet(f.tmp32)
		f.code.I32Const(v)
		f.code.I32Store(uint32(4 * offset))
	}
	for k, name := range n.Captured {
		f.code.LocalGet(f.tmp32)
		f.cellOf(name)
		f.code.I64Store(uint32(16 + 8*k))
	}
	f.code.LocalGet(f.tmp32)
	f.code.Op(wasm.I64ExtendI32U)
}

// compileLambda compiles n as a glisp function and returns its table index.
// Its slots are those Resolve gave it: parameters, then local defs.
func (c *wasmCompiler) compileLambda(n Lambda) uint32 {
	index, table := c.declareFn()
	f := &wasmFunc{c: c, nparams: 3, lambda: &n}
	f.slots = append([]string(nil), n.Params...)
	for _, name := range localDefs(n.Body) {
		f.slots = appendNew(f.slots, name)
	}
	f.boxed = make([]bool, len(f.slots))
	for _, inner := range innerLambdas(n.Body) {
		for _, name := range inner.Captured {
			if i := f.slot(name); i >= 0 {
				f.boxed[i] = true
			}
		}
	}
	for range f.slots {
		f.slotLocal = append(f.slotLocal, f.local(wasm.I64))
	}
	f.scratch = f.local(wasm.I64)
	f.tmp32 = f.local(wasm.I32)

	f.code.LocalGet(2)
	f.code.I32Const(int32(len(n.Params)))
	f.code.Op(wasm.I32Ne)
	f.code.If(wasm.BlockVoid)
	arguments := "arguments"
	if len(n.Params) == 1 {
		arguments = "argument"
	}
	msg := fmt.Sprintf("lambda (%s) expects %d %s, got ", strings.Join(n.Params, " "), len(n.Params), arguments)
	c.fail(&f.code, msg, func() {
		f.code.LocalGet(2)
		f.code.Op(wasm.I64ExtendI32U)
		f.code.Call(c.rt.putInt)
		c.text(&f.code, ": ")
		f.code.LocalGet(1)
		f.code.LocalGet(2)
		f.code.Call(c.rt.putArgs)
	})
	f.code.Op(wasm.End)

	// The parameters, as passed; then a loop that a self tail call jumps back
	// to with new ones. Each pass gives captured variables fresh cells, so a
	// closure made in one pass keeps its own, and starts local defs unbound.
	for i := range n.Params {
		f.code.LocalGet(1)
		f.code.I64Load(uint32(8 * i))
		f.code.LocalSet(f.slotLocal[i])
	}
	f.code.Loop(wasm.BlockI64)
	for i := range f.slots {
		if i < len(n.Params) {
			if !f.boxed[i] {
				continue
			}
			f.code.LocalGet(f.slotLocal[i])
		} else {
			f.code.I64Const(0)
		}
		if f.boxed[i] {
			f.code.Call(c.rt.cell)
		}
		f.code.LocalSet(f.slotLocal[i])
	}
	f.body(n.Body)
	f.code.Op(wasm.End)
	c.define(index, f.locals, f.code)
	return table
}

// innerLambdas returns the lambdas in nodes, not counting lambdas inside
// them.
func innerLambdas(nodes []Node) []Lambda {
	var lambdas []Lambda
	for _, node := range nodes {
		switch n := node.(type) {
		case Lambda:
			lambdas = append(lambdas, n)
		case Def:
			lambdas = append(lambdas, innerLambdas([]Node{n.Value})...)
		case If:
			lambdas = append(lambdas, innerLambdas([]Node{n.Cond, n.Then, n.Else})...)
		case Apply:
			lambdas = append(lambdas, innerLambdas(append([]Node{n.Fn}, n.Args...))...)
		}
	}
	return lambdas
}

// unresolved returns node without what Resolve filled in, to print it as the
// interpreter does in an error.
func unresolved(node Node) Node {
	switch n := node.(type) {
	case Ref:
		return Ref{Name: n.Name}
	case If:
		return If{unresolved(n.Cond), unresolved(n.Then), unresolved(n.Else)}
	case Def:
		return Def{n.Name, unresolved(n.Value)}
	case Apply:
		args := make([]Node, len(n.Args))
		for i, arg := range n.Args {
			args[i] = unresolved(arg)
		}
		return Apply{unresolved(n.Fn), args}
	case Lambda:
		body := make([]Node, len(n.Body))
		for i, form := range n.Body {
			body[i] = unresolved(form)
		}
		return Lambda{Params: n.Params, Body: body}
	}
	return node
}
