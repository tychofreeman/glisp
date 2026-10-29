package glisp

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"

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
	return compileWasmIn(input, "")
}

// compileWasmIn is CompileWasm for a program read from a file in dir, from
// which it may import modules; dir is "" for one that wasn't.
func compileWasmIn(input, dir string) (module []byte, err error) {
	run := &runState{}
	defer recoverAt(&err, run)
	tokens, lines := tokenizeLines(input)
	return compileWasm(run, tokens, lines, dir), nil
}

// CompileWasmFile is CompileWasm for the program in the file fname.
func CompileWasmFile(fname string) ([]byte, error) {
	program, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return compileWasmIn(string(program), filepath.Dir(fname))
}

// Compiled values are i64s. An odd value is an integer n stored as 2n+1; any
// other value is the address of an object in linear memory, whose first i32
// is its tag. The value 0 marks a variable that has no value yet. What each
// tag means, its type and its layout, is in tagKinds.
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
	tagVector  // length at +4, values from +8
	tagMap     // count at +4, root node at +8
	tagMapNode // bitmaps at +4 and +8, slot count at +12, slots from +16
	tagData    // like a vector: the constructor's name, then the fields
	tagFloat   // the float's bits at +8
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
	gMapChanged         // whether the last map update added or removed a key
	gOwned              // nodes at or above it are hash_map's to change in place
	gScratch            // where printing a float keeps its bignums
	gNames
)

// pending is what a function returns when it makes a tail call: the call
// is left in gPendingFn, gPendingArgc and gPendingArgs for invoke to make,
// after the caller's frame is gone. It is never a value: integers are odd
// and objects are at addresses from 8.
const pending = 2

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
	made     map[string]bool // the optional types the program can make (makers): the module has the runtime of no others
}

func compileWasm(run *runState, tokens List, lines Lines, dir string) []byte {
	scope := &Scope{prev: builtinScope, table: map[string]interface{}{}, run: run}
	expander := NewExpander(scope)
	expander.lines, expander.dir = Lines{}, dir
	expand := func(form interface{}) []Node {
		var nodes []Node
		for _, node := range expander.ExpandFileForm(form) {
			if def, isDef := node.(Def); isDef {
				if _, isLambda := def.Value.(Lambda); isLambda {
					Eval(node, scope) // so later macros can call it
				}
			}
			nodes = append(nodes, Resolve(node))
		}
		return nodes
	}
	var stdlibNodes, programNodes []Node
	nodes, macros := stdlibExpanded()
	for name, macro := range macros {
		scope.add(name, macro)
	}
	for _, node := range nodes {
		if def, isDef := node.(Def); isDef {
			if _, isLambda := def.Value.(Lambda); isLambda {
				Eval(node, scope) // so the program's macros can call it
			}
		}
		stdlibNodes = append(stdlibNodes, Resolve(node))
	}
	var programLines []int
	for _, token := range tokens {
		form := readLines(token, lines, expander.lines)
		for _, node := range expand(form) {
			programLines = append(programLines, expander.formLine(form))
			programNodes = append(programNodes, node)
		}
	}
	expander.Finish()

	used := usedDefs(stdlibNodes, programNodes)
	c := newWasmCompiler(scope, makes(append(append([]Node{}, used...), programNodes...)))
	main := &wasmFunc{c: c, wasmBody: &wasmBody{}}
	main.scratch = main.local(wasm.I64)
	main.tmp32 = main.local(wasm.I32)
	for _, node := range used {
		main.expr(node)
		main.Op(wasm.Drop)
	}
	// Each top-level form starts at its own line, as in the interpreter.
	if len(programNodes) == 0 {
		main.I64Const(addrNil)
	}
	for i, node := range programNodes {
		main.I32Const(int32(programLines[i]))
		main.GlobalSet(gLine)
		main.expr(node)
		if i < len(programNodes)-1 {
			main.Op(wasm.Drop)
		}
	}
	mainIndex := c.declare(wasm.FuncType{Results: []wasm.ValType{wasm.I64}})
	c.setBody(mainIndex, main.wasmBody)

	startIndex := c.declare(wasm.FuncType{})
	c.define(startIndex, func(b *wasmBody) {
		result := b.local(i64)
		b.Call(mainIndex)
		b.LocalSet(result)
		c.text(b, "=> ")
		b.LocalGet(result)
		b.Call(c.rt.putShow)
		b.I32Const('\n')
		b.Call(c.rt.putByte)
		b.Call(c.rt.flush)
	})
	c.m.Exports = []wasm.Export{
		{Name: "_start", Kind: wasm.ExportFunc, Index: startIndex},
		{Name: "memory", Kind: wasm.ExportMemory},
	}
	return c.finish()
}

// usedDefs returns the stdlib forms a program needs: all but the definitions
// of names that nothing it runs refers to, directly or through another
// definition (every definition of the name: a generic operation has one
// per instance). Macro helpers, needed only while expanding, stay out of
// the module.
func usedDefs(stdlib, program []Node) []Node {
	defs := map[string][]Node{}
	for _, node := range stdlib {
		if def, isDef := node.(Def); isDef {
			defs[def.Name] = append(defs[def.Name], def.Value)
		}
	}
	used := map[string]bool{}
	var use func(names []string)
	use = func(names []string) {
		for _, name := range names {
			if !used[name] {
				used[name] = true
				for _, value := range defs[name] {
					use(globalRefs(value))
				}
			}
		}
	}
	for _, node := range program {
		use(globalRefs(node))
	}
	var kept []Node
	for _, node := range stdlib {
		if def, isDef := node.(Def); isDef && !used[def.Name] {
			continue
		}
		kept = append(kept, node)
	}
	return kept
}

// globalRefs returns the names node refers to that Resolve left to look up
// by name: top-level definitions and builtins.
func globalRefs(node Node) []string {
	var names []string
	var walk func(Node)
	walk = func(node Node) {
		switch n := node.(type) {
		case Ref:
			if n.Loc == nil {
				names = append(names, n.Name)
			}
		case If:
			walk(n.Cond)
			walk(n.Then)
			walk(n.Else)
		case Lambda:
			for _, form := range n.Body {
				walk(form)
			}
		case Def:
			walk(n.Value)
		case Apply:
			walk(n.Fn)
			for _, arg := range n.Args {
				walk(arg)
			}
		}
	}
	walk(node)
	return names
}

func newWasmCompiler(scope *Scope, made map[string]bool) *wasmCompiler {
	c := &wasmCompiler{
		m:        &wasm.Module{},
		scope:    scope,
		made:     made,
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
	scratch := outBuf + outCap
	stack := scratch + scratchSize
	heap := stack + stackSize
	c.m.Globals[gSP].Init = int64(stack)
	c.m.Globals[gHP].Init = int64(heap)
	c.m.Globals[gFD].Init = 1
	c.m.Globals[gOutBuf].Init = int64(outBuf)
	c.m.Globals[gScratch].Init = int64(scratch)
	c.m.Globals[gStackEnd].Init = int64(stack + stackSize - stackGuard)
	c.m.Globals[gStackBase].Init = int64(stack)
	c.m.Globals[gHeapStart].Init = int64(heap)
	c.m.Globals[gGCNext].Init = int64(heap + gcThreshold)
	c.m.Globals[gOwned].Init = -1
	c.defineGC()
	c.m.MemoryPages = uint32(heap/65536 + 1)
	c.m.DataAt = 8
	c.m.Data = c.data[8:]
	// A builtin's table entry is live once something made its closure: a
	// builtin is only reached through its name.
	builtin := map[uint32]bool{}
	for _, t := range c.rt.builtins {
		builtin[t] = true
	}
	c.m.Prune(func(entry int) bool {
		_, made := c.closures[uint32(entry)]
		return made || !builtin[uint32(entry)]
	})
	return c.m.Encode()
}

func align8(n int) int { return (n + 7) &^ 7 }

// declare adds a function of type ft and returns its index; define gives it
// a body.
func (c *wasmCompiler) declare(ft wasm.FuncType) uint32 {
	c.m.Funcs = append(c.m.Funcs, wasm.Func{Type: c.m.Type(ft)})
	return uint32(len(c.m.Imports) + len(c.m.Funcs) - 1)
}

// setBody gives function f the body b, already built.
func (c *wasmCompiler) setBody(f uint32, b *wasmBody) {
	i := f - uint32(len(c.m.Imports))
	c.m.Funcs[i].Locals = b.locals
	c.m.Funcs[i].Body = b.Code
}

// wasmBody is a function's body as it is built: its code, and the locals it
// declares, numbered after its parameters.
type wasmBody struct {
	wasm.Code
	nparams uint32
	locals  []wasm.ValType
}

// param is the index of parameter i.
func (b *wasmBody) param(i uint32) uint32 {
	if i >= b.nparams {
		panic(fmt.Sprintf("parameter %d of a function with %d", i, b.nparams))
	}
	return i
}

// local declares a local of type t and returns its index.
func (b *wasmBody) local(t wasm.ValType) uint32 {
	b.locals = append(b.locals, t)
	return b.nparams + uint32(len(b.locals)-1)
}

// define gives function f the body that build makes.
func (c *wasmCompiler) define(f uint32, build func(b *wasmBody)) {
	i := f - uint32(len(c.m.Imports))
	b := &wasmBody{nparams: uint32(len(c.m.Types[c.m.Funcs[i].Type].Params))}
	build(b)
	c.setBody(f, b)
}

// The parameters of every glisp function, a compiled lambda or a builtin:
// the closure called, the address of its arguments on the argument stack,
// and how many there are.
const (
	paramSelf uint32 = iota
	paramArgs
	paramArgc
)

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
	case float64:
		addr := c.static(16)
		c.put32(addr, tagFloat)
		c.put64(addr+8, int64(math.Float64bits(v)))
		return int64(addr)
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
func (c *wasmCompiler) text(code *wasmBody, s string) {
	code.I32Const(int32(c.str(s)))
	code.Call(c.rt.putStr)
}

// fail appends code that reports the error "glisp: " + msg + whatever more
// prints, and exits.
func (c *wasmCompiler) fail(code *wasmBody, msg string, more func()) {
	code.Call(c.rt.failBegin)
	c.text(code, msg)
	if more != nil {
		more()
	}
	code.Call(c.rt.failEnd)
	code.Op(wasm.Unreachable)
}

// failIf appends code that fails as fail does if the i32 on the stack is
// true.
func (c *wasmCompiler) failIf(code *wasmBody, msg string, more func()) {
	code.when(func() { c.fail(code, msg, more) })
}

// wasmFunc compiles the body of main or of one lambda.
type wasmFunc struct {
	c *wasmCompiler
	*wasmBody
	lambda  *Lambda // nil in main
	slots   []string
	boxed   []bool // slots that lambdas inside this one capture, kept in cells
	frame   uint32 // an i32 local: the lambda's frame on the argument stack
	scratch uint32 // an i64 local
	tmp32   uint32 // an i32 local
	blocks  int    // blocks open inside a lambda's loop, for a tail call's br
}

// A lambda keeps its values in a frame on the argument stack, where the
// collector finds them: its closure, then its slots.

// A call on the argument stack and a lambda's frame have one layout: a
// function, then values (the arguments, or the frame's slots).
// valueOffset is the offset of value i.
func valueOffset(i int) uint32 { return uint32(8 * (1 + i)) }

// slotLoad leaves slot i of the lambda's frame.
func (f *wasmFunc) slotLoad(i int) {
	f.LocalGet(f.frame)
	f.I64Load(valueOffset(i))
}

// slotStore stores the value in local v in slot i of the lambda's frame.
func (f *wasmFunc) slotStore(i int, v uint32) {
	f.slotSet(i, func() { f.LocalGet(v) })
}

// slotSet stores what value leaves in slot i of the lambda's frame.
func (f *wasmFunc) slotSet(i int, value func()) {
	f.LocalGet(f.frame)
	value()
	f.I64Store(valueOffset(i))
}

// selfLoad leaves the lambda's closure.
func (f *wasmFunc) selfLoad() {
	f.LocalGet(f.frame)
	f.I64Load(0)
}

// selfSet stores what value leaves as the lambda's closure in its frame.
func (f *wasmFunc) selfSet(value func()) {
	f.LocalGet(f.frame)
	value()
	f.I64Store(0)
}

// at records, for errors, that the program is making the call on line, if
// it is known.
func (f *wasmFunc) at(line int) {
	if line > 0 {
		f.I32Const(int32(line))
		f.GlobalSet(gLine)
	}
}

// body compiles nodes, leaving the last one's value, or nil if there are none.
func (f *wasmFunc) body(nodes []Node) {
	if len(nodes) == 0 {
		f.I64Const(addrNil)
	}
	for i, node := range nodes {
		if i < len(nodes)-1 {
			f.expr(node)
			f.Op(wasm.Drop)
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
		f.I64Const(f.c.constant(n.Value))
	case Quote:
		f.I64Const(f.c.datum(n.Datum))
	case Ref:
		f.ref(n)
	case If:
		f.expr(n.Cond)
		f.I64Const(addrTrue)
		f.Op(wasm.I64Eq)
		f.ifElse(wasm.BlockI64, func() {
			f.expr(n.Then)
		}, func() {
			f.expr(n.Else)
		})
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
				f.c.fail(f.wasmBody, n.Name+" is a macro, not a value", nil)
				return
			}
		}
		f.GlobalGet(f.c.global(n.Name))
		f.checkBound(n.Name)
	case n.Loc.Depth == 0:
		f.slotLoad(n.Loc.Index)
		if f.boxed[n.Loc.Index] {
			f.Op(wasm.I32WrapI64)
			f.I64Load(8)
		}
		if n.Loc.Index >= len(f.lambda.Params) {
			f.checkBound(n.Name)
		}
	default:
		f.cellOf(n.Name)
		f.Op(wasm.I32WrapI64)
		f.I64Load(8)
		f.checkBound(n.Name)
	}
}

// checkBound fails if the value on the stack is 0: a name not yet defined.
func (f *wasmFunc) checkBound(name string) {
	f.LocalTee(f.scratch)
	f.Op(wasm.I64Eqz)
	f.c.failIf(f.wasmBody, "Cannot resolve symbol "+name, nil)
	f.LocalGet(f.scratch)
}

// cellOf leaves the cell of name, a slot of this lambda that inner lambdas
// capture or a variable this lambda captures.
func (f *wasmFunc) cellOf(name string) {
	if i := f.slot(name); i >= 0 {
		f.slotLoad(i)
		return
	}
	f.selfLoad()
	f.Op(wasm.I32WrapI64)
	f.I64Load(16 + 8*f.captured(name))
}

func (f *wasmFunc) def(n Def) {
	f.expr(n.Value)
	if f.lambda == nil {
		f.GlobalSet(f.c.global(n.Name))
	} else if i := f.slot(n.Name); f.boxed[i] {
		f.LocalSet(f.scratch)
		f.slotLoad(i)
		f.Op(wasm.I32WrapI64)
		f.LocalGet(f.scratch)
		f.I64Store(8)
	} else {
		f.LocalSet(f.scratch)
		f.slotStore(i, f.scratch)
	}
	f.I64Const(addrEmpty)
}

// apply pushes the function and arguments on the argument stack and calls
// invoke. Once the call returns it is no longer the call being made, so the
// line goes back to what it was: in tail position nothing comes after the
// call, and the caller's call puts the line back.
func (f *wasmFunc) apply(n Apply) {
	line := f.local(wasm.I32)
	f.GlobalGet(gLine)
	f.LocalSet(line)
	f.at(n.Line)
	f.invoke(f.pushCall(n), n)
	f.LocalGet(line)
	f.GlobalSet(gLine)
}

// pushCall pushes the function n calls, then its arguments, onto the
// argument stack, where they stay while the call is made.
func (f *wasmFunc) pushCall(n Apply) stackCall {
	call := stackCall{f.wasmBody, f.local(wasm.I32), len(n.Args)}
	f.GlobalGet(gSP)
	f.LocalSet(call.base)
	for _, node := range append([]Node{n.Fn}, n.Args...) {
		f.push(func() { f.expr(node) })
	}
	return call
}

// A stackCall is a call on the argument stack, from the address in local
// base: the function, then argc arguments.
type stackCall struct {
	b    *wasmBody
	base uint32
	argc int
}

// fn leaves the function called.
func (s stackCall) fn() {
	s.b.LocalGet(s.base)
	s.b.I64Load(0)
}

// arg leaves argument i.
func (s stackCall) arg(i int) {
	s.b.LocalGet(s.base)
	s.b.I64Load(valueOffset(i))
}

// args leaves the address of the arguments.
func (s stackCall) args() {
	s.b.LocalGet(s.base)
	s.b.I32Const(8)
	s.b.Op(wasm.I32Add)
}

// pop takes the call off the stack.
func (s stackCall) pop() {
	s.b.LocalGet(s.base)
	s.b.GlobalSet(gSP)
}

// invoke makes call, n, and pops it. A builtin with an inline path runs inline when the
// function is still that builtin. Otherwise the call is a safe point: the
// collector may run first, so the function is read from the stack after it.
func (f *wasmFunc) invoke(call stackCall, n Apply) {
	f.at(n.Line) // again, after the arguments' calls
	slow := func() {
		f.c.safePoint(f.wasmBody)
		call.fn()
		call.args()
		f.I32Const(int32(call.argc))
		f.I32Const(int32(f.c.str(unresolved(n).String())))
		f.Call(f.c.rt.invoke)
	}
	if name, inline, found := f.c.inlineFor(n); found {
		call.fn()
		f.I64Const(int64(f.c.closure(f.c.rt.builtins[name])))
		f.Op(wasm.I64Eq)
		f.ifElse(wasm.BlockI64, func() {
			inline.emit(f.wasmBody, &f.c.rt, call.arg, slow)
		}, func() {
			slow()
		})
	} else {
		slow()
	}
	call.pop()
}

// tailExpr compiles node, whose value the lambda returns. In tail position,
// a call to the lambda itself is a jump back to the start of its loop, and
// a call to another lambda is left for invoke to make.
func (f *wasmFunc) tailExpr(node Node) {
	switch n := node.(type) {
	case If:
		f.expr(n.Cond)
		f.I64Const(addrTrue)
		f.Op(wasm.I64Eq)
		f.ifElse(wasm.BlockI64, func() {
			f.blocks++
			f.tailExpr(n.Then)
		}, func() {
			f.tailExpr(n.Else)
			f.blocks--
		})
	case Apply:
		f.tailApply(n)
	default:
		f.expr(node)
	}
}

// tailApply compiles a call in tail position. If the function called is
// this closure itself (and takes no rest parameter), its arguments become
// the parameters and it jumps
// back to the top of the loop; otherwise tailCall makes it.
func (f *wasmFunc) tailApply(n Apply) {
	f.at(n.Line)
	call := f.pushCall(n)
	f.at(n.Line)
	if f.lambda.Rest || len(n.Args) != len(f.lambda.Params) {
		f.tailCall(call, n)
		return
	}
	call.fn()
	f.selfLoad()
	f.Op(wasm.I64Eq)
	f.ifElse(wasm.BlockI64, func() {
		for i := range n.Args {
			f.slotSet(i, func() { call.arg(i) })
		}
		call.pop()
		f.Br(uint32(f.blocks + 1))
	}, func() {
		f.tailCall(call, n)
	})
}

// tailCall makes call, n, in tail position. A lambda's closure is left pending for invoke, which calls it
// once this frame is gone, so tail calls between functions use no stack:
// the arguments stay where they are, above the popped frame, until invoke
// moves them down. Builtins, and values that aren't functions, are called
// (or reported) as usual.
func (f *wasmFunc) tailCall(call stackCall, n Apply) {
	call.fn()
	f.Call(f.c.rt.tagOf)
	f.I32Const(tagClosure)
	f.Op(wasm.I32Eq)
	f.ifElse(wasm.BlockI32, func() {
		call.fn()
		f.tableIndex()
		f.I32Const(int32(len(builtinSpecs)))
		f.Op(wasm.I32GeU)
	}, func() {
		f.I32Const(0)
	})
	f.ifElse(wasm.BlockI64, func() {
		call.fn()
		f.GlobalSet(gPendingFn)
		call.args()
		f.GlobalSet(gPendingArgs)
		f.I32Const(int32(call.argc))
		f.GlobalSet(gPendingArgc)
		f.LocalGet(f.frame)
		f.GlobalSet(gSP)
		f.I64Const(pending)
		f.Op(wasm.Return)
	}, func() {
		f.invoke(call, n)
	})
}

// lambdaValue compiles n's body as a function and leaves a closure of it.
func (f *wasmFunc) lambdaValue(n Lambda) {
	table := f.c.compileLambda(n)
	if len(n.Captured) == 0 {
		f.I64Const(int64(f.c.closure(table)))
		return
	}
	f.I32Const(int32(16 + 8*len(n.Captured)))
	f.Call(f.c.rt.alloc)
	f.LocalSet(f.tmp32)
	for offset, v := range []int32{tagClosure, int32(table), int32(len(n.Captured))} {
		f.LocalGet(f.tmp32)
		f.I32Const(v)
		f.I32Store(uint32(4 * offset))
	}
	for k, name := range n.Captured {
		f.LocalGet(f.tmp32)
		f.cellOf(name)
		f.I64Store(uint32(16 + 8*k))
	}
	f.LocalGet(f.tmp32)
	f.Op(wasm.I64ExtendI32U)
}

// compileLambda compiles n as a glisp function and returns its table index.
// Its slots are those Resolve gave it: parameters, then local defs.
func (c *wasmCompiler) compileLambda(n Lambda) uint32 {
	index, table := c.declareFn()
	f := &wasmFunc{c: c, wasmBody: &wasmBody{nparams: 3}, lambda: &n}
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

	c.checkArity(f.wasmBody, "lambda "+n.paramList(), n.arity(), func() {
		c.text(f.wasmBody, ": ")
		f.LocalGet(paramArgs)
		f.LocalGet(paramArgc)
		f.Call(c.rt.putArgs)
	})

	// The frame, at the top of the argument stack: the closure, then the
	// parameters as passed; then a loop that a self tail call jumps back to
	// with new ones. Each pass gives captured variables fresh cells, so a
	// closure made in one pass keeps its own, and starts local defs unbound.
	f.GlobalGet(gSP)
	f.LocalTee(f.frame)
	f.I32Const(int32(valueOffset(len(f.slots))))
	f.Op(wasm.I32Add)
	f.GlobalSet(gSP)
	f.selfSet(func() { f.LocalGet(paramSelf) })
	for i := range n.Params {
		if n.Rest && i == len(n.Params)-1 {
			f.slotSet(i, func() { f.restList(i) })
		} else {
			f.slotSet(i, func() { f.arg(uint32(i)) })
		}
	}
	f.Loop(wasm.BlockI64)
	for i := range f.slots {
		if i < len(n.Params) && !f.boxed[i] {
			continue
		}
		f.slotSet(i, func() {
			if i < len(n.Params) {
				f.slotLoad(i)
			} else {
				f.I64Const(0)
			}
			if f.boxed[i] {
				f.Call(c.rt.cell)
			}
		})
	}
	f.body(n.Body)
	f.Op(wasm.End)
	f.LocalGet(f.frame)
	f.GlobalSet(gSP)
	c.setBody(index, f.wasmBody)
	return table
}

// restList leaves the list of the lambda's arguments from the from-th on,
// for its rest parameter.
func (f *wasmFunc) restList(from int) {
	k, list := f.tmp32, f.scratch
	f.LocalGet(paramArgc)
	f.LocalSet(k)
	f.I64Const(addrEmpty)
	f.LocalSet(list)
	f.until(func() {
		f.LocalGet(k)
		f.I32Const(int32(from))
		f.Op(wasm.I32LeU)
	}, func() {
		f.LocalGet(k)
		f.I32Const(1)
		f.Op(wasm.I32Sub)
		f.LocalSet(k)
		f.argAt(k)
		f.LocalGet(list)
		f.Call(f.c.rt.pair)
		f.LocalSet(list)
	})
	f.LocalGet(list)
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
		return Lambda{Params: n.Params, Rest: n.Rest, Body: body}
	}
	return node
}
