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
	run := &runState{}
	defer recoverAt(&err, run)
	tokens, lines := tokenizeLines(input)
	return compileWasm(run, tokens, lines), nil
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
	stackSize  = 16 << 20
	stackGuard = 1 << 16
)

// Global variables of the runtime; the program's top-level names follow.
const (
	gSP          = iota // argument stack pointer
	gHP                 // next free heap address
	gOutLen             // bytes waiting in the output buffer
	gFD                 // where the output buffer goes: 1, or 2 for an error
	gOutBuf             // address of the output buffer
	gStackEnd           // the argument stack is full past here
	gPendingFn          // a tail call's function (i64), for invoke to call
	gPendingArgc        // how many arguments it has
	gPendingArgs        // where they are
	gCapture            // 1 while str captures printing into a string
	gCapEnd             // where the captured bytes go next
	gLine               // the line of the call being made, for errors
	gStackBase          // where the argument stack starts: the collector's roots
	gHeapStart          // where the heap starts
	gGCNext             // collect at the next safe point once the heap passes this
	gToStart            // while collecting: where to-space starts
	gToNext             // and where the next copy goes
	gFromEnd            // and where from-space ends
	gDelta              // and how far to-space moves when it slides down
	gNames
)

// pending is what a function returns when it makes a tail call: the call
// is left in gPendingFn, gPendingArgc and gPendingArgs for invoke to make,
// after the caller's frame is gone. It is never a value: integers are odd
// and objects are at addresses from 8.
const pending = 2

// builtinNames are the builtin functions, compiled as runtime functions.
var builtinNames = []string{"car", "cdr", "atom", "cons", "plus", "-", "*", "/", "mod", "<", ">", "<=", ">=",
	"number?", "string?", "symbol?", "list?", "function?", "boolean?", "nil?", "eq", "apply", "p",
	"str", "string_length", "substring", "chars", "string<", "symbol", "parse_number"}

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

func compileWasm(run *runState, tokens List, lines Lines) []byte {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}, run: run}
	expander := NewExpander(scope)
	expander.lines = Lines{}
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
	var programLines []int
	for _, token := range tokens {
		form := readLines(token, lines, expander.lines)
		programLines = append(programLines, expander.formLine(form))
		programNodes = append(programNodes, expand(form))
	}

	c := newWasmCompiler(scope)
	main := &wasmFunc{c: c}
	main.scratch = main.local(wasm.I64)
	main.tmp32 = main.local(wasm.I32)
	for _, node := range stdlibNodes {
		main.expr(node)
		main.code.Op(wasm.Drop)
	}
	// Each top-level form starts at its own line, as in the interpreter.
	if len(programNodes) == 0 {
		main.code.I64Const(addrNil)
	}
	for i, node := range programNodes {
		main.code.I32Const(int32(programLines[i]))
		main.code.GlobalSet(gLine)
		main.expr(node)
		if i < len(programNodes)-1 {
			main.code.Op(wasm.Drop)
		}
	}
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
	c.m.Globals[gPendingFn].Type = wasm.I64
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
	c.m.Globals[gStackBase].Init = int64(stack)
	c.m.Globals[gHeapStart].Init = int64(heap)
	c.m.Globals[gGCNext].Init = int64(heap + gcThreshold)
	c.defineGC()
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
	c       *wasmCompiler
	code    wasm.Code
	nparams uint32
	locals  []wasm.ValType
	lambda  *Lambda // nil in main
	slots   []string
	boxed   []bool // slots that lambdas inside this one capture, kept in cells
	frame   uint32 // an i32 local: the lambda's frame on the argument stack
	scratch uint32 // an i64 local
	tmp32   uint32 // an i32 local
	blocks  int    // blocks open inside a lambda's loop, for a tail call's br
}

func (f *wasmFunc) local(t wasm.ValType) uint32 {
	f.locals = append(f.locals, t)
	return f.nparams + uint32(len(f.locals)-1)
}

// A lambda keeps its values in a frame on the argument stack, where the
// collector finds them: its closure, then its slots.

// slotLoad leaves slot i of the lambda's frame.
func (f *wasmFunc) slotLoad(i int) {
	f.code.LocalGet(f.frame)
	f.code.I64Load(uint32(8 * (1 + i)))
}

// slotStore stores the value in local v in slot i of the lambda's frame.
func (f *wasmFunc) slotStore(i int, v uint32) {
	f.code.LocalGet(f.frame)
	f.code.LocalGet(v)
	f.code.I64Store(uint32(8 * (1 + i)))
}

// selfLoad leaves the lambda's closure.
func (f *wasmFunc) selfLoad() {
	f.code.LocalGet(f.frame)
	f.code.I64Load(0)
}

// at records, for errors, that the program is making the call on line, if
// it is known.
func (f *wasmFunc) at(line int) {
	if line > 0 {
		f.code.I32Const(int32(line))
		f.code.GlobalSet(gLine)
	}
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
		f.slotLoad(n.Loc.Index)
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
		f.slotLoad(i)
		return
	}
	f.selfLoad()
	f.code.Op(wasm.I32WrapI64)
	f.code.I64Load(16 + 8*f.captured(name))
}

func (f *wasmFunc) def(n Def) {
	f.expr(n.Value)
	if f.lambda == nil {
		f.code.GlobalSet(f.c.global(n.Name))
	} else if i := f.slot(n.Name); f.boxed[i] {
		f.code.LocalSet(f.scratch)
		f.slotLoad(i)
		f.code.Op(wasm.I32WrapI64)
		f.code.LocalGet(f.scratch)
		f.code.I64Store(8)
	} else {
		f.code.LocalSet(f.scratch)
		f.slotStore(i, f.scratch)
	}
	f.code.I64Const(addrEmpty)
}

// apply pushes the function and arguments on the argument stack and calls
// invoke.
func (f *wasmFunc) apply(n Apply) {
	f.at(n.Line)
	f.invoke(f.pushCall(n), n)
}

// pushCall pushes the function n calls, then its arguments, onto the
// argument stack, where they stay while the call is made, and returns a
// local holding the function's address there.
func (f *wasmFunc) pushCall(n Apply) uint32 {
	base := f.local(wasm.I32)
	f.code.GlobalGet(gSP)
	f.code.LocalSet(base)
	for _, node := range append([]Node{n.Fn}, n.Args...) {
		f.code.GlobalGet(gSP)
		f.expr(node)
		f.code.I64Store(0)
		f.code.GlobalGet(gSP)
		f.code.I32Const(8)
		f.code.Op(wasm.I32Add)
		f.code.GlobalSet(gSP)
	}
	return base
}

// invoke calls the function at base with n's arguments, all on the argument
// stack, and pops them. A builtin with an inline path runs inline when the
// function is still that builtin. Otherwise the call is a safe point: the
// collector may run first, so the function is read from the stack after it.
func (f *wasmFunc) invoke(base uint32, n Apply) {
	f.at(n.Line) // again, after the arguments' calls
	call := func() {
		f.c.safePoint(&f.code)
		f.code.LocalGet(base)
		f.code.I64Load(0)
		f.code.LocalGet(base)
		f.code.I32Const(8)
		f.code.Op(wasm.I32Add)
		f.code.I32Const(int32(len(n.Args)))
		f.code.I32Const(int32(f.c.str(unresolved(n).String())))
		f.code.Call(f.c.rt.invoke)
	}
	if name, inline, found := f.c.inlineFor(n); found {
		f.code.LocalGet(base)
		f.code.I64Load(0)
		f.code.I64Const(int64(f.c.closure(f.c.rt.builtins[name])))
		f.code.Op(wasm.I64Eq)
		f.code.If(wasm.BlockI64)
		inline.emit(&f.code, &f.c.rt, func(i int) {
			f.code.LocalGet(base)
			f.code.I64Load(uint32(8 * (1 + i)))
		}, call)
		f.code.Op(wasm.Else)
		call()
		f.code.Op(wasm.End)
	} else {
		call()
	}
	f.code.LocalGet(base)
	f.code.GlobalSet(gSP)
}

// tailExpr compiles node, whose value the lambda returns. In tail position,
// a call to the lambda itself is a jump back to the start of its loop, and
// a call to another lambda is left for invoke to make.
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
		f.tailApply(n)
	default:
		f.expr(node)
	}
}

// tailApply compiles a call in tail position. If the function called is
// this closure itself, its arguments become the parameters and it jumps
// back to the top of the loop; otherwise tailCall makes it.
func (f *wasmFunc) tailApply(n Apply) {
	f.at(n.Line)
	base := f.pushCall(n)
	f.at(n.Line)
	if len(n.Args) != len(f.lambda.Params) {
		f.tailCall(base, n)
		return
	}
	f.code.LocalGet(base)
	f.code.I64Load(0)
	f.selfLoad()
	f.code.Op(wasm.I64Eq)
	f.code.If(wasm.BlockI64)
	for i := range n.Args {
		f.code.LocalGet(f.frame)
		f.code.LocalGet(base)
		f.code.I64Load(uint32(8 * (1 + i)))
		f.code.I64Store(uint32(8 * (1 + i)))
	}
	f.code.LocalGet(base)
	f.code.GlobalSet(gSP)
	f.code.Br(uint32(f.blocks + 1))
	f.code.Op(wasm.Else)
	f.tailCall(base, n)
	f.code.Op(wasm.End)
}

// tailCall calls the function at base, in tail position, with the arguments
// after it. A lambda's closure is left pending for invoke, which calls it
// once this frame is gone, so tail calls between functions use no stack:
// the arguments stay where they are, above the popped frame, until invoke
// moves them down. Builtins, and values that aren't functions, are called
// (or reported) as usual.
func (f *wasmFunc) tailCall(base uint32, n Apply) {
	fn := func() {
		f.code.LocalGet(base)
		f.code.I64Load(0)
	}
	fn()
	f.code.Call(f.c.rt.tagOf)
	f.code.I32Const(tagClosure)
	f.code.Op(wasm.I32Eq)
	f.code.If(wasm.BlockI32)
	fn()
	f.code.Op(wasm.I32WrapI64)
	f.code.I32Load(4)
	f.code.I32Const(int32(len(builtinNames)))
	f.code.Op(wasm.I32GeU)
	f.code.Op(wasm.Else)
	f.code.I32Const(0)
	f.code.Op(wasm.End)
	f.code.If(wasm.BlockI64)
	fn()
	f.code.GlobalSet(gPendingFn)
	f.code.LocalGet(base)
	f.code.I32Const(8)
	f.code.Op(wasm.I32Add)
	f.code.GlobalSet(gPendingArgs)
	f.code.I32Const(int32(len(n.Args)))
	f.code.GlobalSet(gPendingArgc)
	f.code.LocalGet(f.frame)
	f.code.GlobalSet(gSP)
	f.code.I64Const(pending)
	f.code.Op(wasm.Return)
	f.code.Op(wasm.Else)
	f.invoke(base, n)
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
	f.frame = f.local(wasm.I32)
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

	// The frame, at the top of the argument stack: the closure, then the
	// parameters as passed; then a loop that a self tail call jumps back to
	// with new ones. Each pass gives captured variables fresh cells, so a
	// closure made in one pass keeps its own, and starts local defs unbound.
	f.code.GlobalGet(gSP)
	f.code.LocalTee(f.frame)
	f.code.I32Const(int32(8 * (1 + len(f.slots))))
	f.code.Op(wasm.I32Add)
	f.code.GlobalSet(gSP)
	f.code.LocalGet(f.frame)
	f.code.LocalGet(0)
	f.code.I64Store(0)
	for i := range n.Params {
		f.code.LocalGet(f.frame)
		f.code.LocalGet(1)
		f.code.I64Load(uint32(8 * i))
		f.code.I64Store(uint32(8 * (1 + i)))
	}
	f.code.Loop(wasm.BlockI64)
	for i := range f.slots {
		if i < len(n.Params) && !f.boxed[i] {
			continue
		}
		f.code.LocalGet(f.frame)
		if i < len(n.Params) {
			f.slotLoad(i)
		} else {
			f.code.I64Const(0)
		}
		if f.boxed[i] {
			f.code.Call(c.rt.cell)
		}
		f.code.I64Store(uint32(8 * (1 + i)))
	}
	f.body(n.Body)
	f.code.Op(wasm.End)
	f.code.LocalGet(f.frame)
	f.code.GlobalSet(gSP)
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
		return Apply{Fn: unresolved(n.Fn), Args: args, Line: n.Line}
	case Lambda:
		body := make([]Node, len(n.Body))
		for i, form := range n.Body {
			body[i] = unresolved(form)
		}
		return Lambda{Params: n.Params, Body: body}
	}
	return node
}
