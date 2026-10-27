package wasm

// Code is a function body: a sequence of instructions, each appended by one
// of its methods. Instructions without immediates are appended with Op and
// the opcode constants below.
type Code []byte

// Opcodes of the instructions that take no immediates.
const (
	Unreachable byte = 0x00
	Nop         byte = 0x01
	Else        byte = 0x05
	End         byte = 0x0b
	Return      byte = 0x0f
	Drop        byte = 0x1a
	Select      byte = 0x1b

	I32Eqz byte = 0x45
	I32Eq  byte = 0x46
	I32Ne  byte = 0x47
	I32LtS byte = 0x48
	I32LtU byte = 0x49
	I32GtS byte = 0x4a
	I32GtU byte = 0x4b
	I32LeS byte = 0x4c
	I32LeU byte = 0x4d
	I32GeS byte = 0x4e
	I32GeU byte = 0x4f

	I64Eqz byte = 0x50
	I64Eq  byte = 0x51
	I64Ne  byte = 0x52
	I64LtS byte = 0x53
	I64LtU byte = 0x54
	I64GtS byte = 0x55
	I64GtU byte = 0x56
	I64LeS byte = 0x57
	I64LeU byte = 0x58
	I64GeS byte = 0x59
	I64GeU byte = 0x5a

	I32Add  byte = 0x6a
	I32Sub  byte = 0x6b
	I32Mul  byte = 0x6c
	I32DivS byte = 0x6d
	I32DivU byte = 0x6e
	I32RemS byte = 0x6f
	I32RemU byte = 0x70
	I32And  byte = 0x71
	I32Or   byte = 0x72
	I32Xor  byte = 0x73
	I32Shl  byte = 0x74
	I32ShrS byte = 0x75
	I32ShrU byte = 0x76

	I64Add  byte = 0x7c
	I64Sub  byte = 0x7d
	I64Mul  byte = 0x7e
	I64DivS byte = 0x7f
	I64DivU byte = 0x80
	I64RemS byte = 0x81
	I64RemU byte = 0x82
	I64And  byte = 0x83
	I64Or   byte = 0x84
	I64Xor  byte = 0x85
	I64Shl  byte = 0x86
	I64ShrS byte = 0x87
	I64ShrU byte = 0x88

	I32WrapI64    byte = 0xa7
	I64ExtendI32S byte = 0xac
	I64ExtendI32U byte = 0xad
)

// Op appends instructions that take no immediates.
func (c *Code) Op(ops ...byte) { *c = append(*c, ops...) }

func (c *Code) withIndex(op byte, index uint32) { *c = uleb(append(*c, op), uint64(index)) }

// I32Const appends i32.const v.
func (c *Code) I32Const(v int32) { *c = sleb(append(*c, 0x41), int64(v)) }

// I64Const appends i64.const v.
func (c *Code) I64Const(v int64) { *c = sleb(append(*c, 0x42), v) }

// LocalGet appends local.get i.
func (c *Code) LocalGet(i uint32) { c.withIndex(0x20, i) }

// LocalSet appends local.set i.
func (c *Code) LocalSet(i uint32) { c.withIndex(0x21, i) }

// LocalTee appends local.tee i.
func (c *Code) LocalTee(i uint32) { c.withIndex(0x22, i) }

// GlobalGet appends global.get i.
func (c *Code) GlobalGet(i uint32) { c.withIndex(0x23, i) }

// GlobalSet appends global.set i.
func (c *Code) GlobalSet(i uint32) { c.withIndex(0x24, i) }

// Call appends call f.
func (c *Code) Call(f uint32) { c.withIndex(0x10, f) }

// CallIndirect appends call_indirect of function type t through table 0.
func (c *Code) CallIndirect(t uint32) {
	c.withIndex(0x11, t)
	*c = append(*c, 0x00)
}

// Block appends block with result type bt; close it with Op(End).
func (c *Code) Block(bt BlockType) { *c = append(*c, 0x02, byte(bt)) }

// Loop appends loop with result type bt; close it with Op(End).
func (c *Code) Loop(bt BlockType) { *c = append(*c, 0x03, byte(bt)) }

// If appends if with result type bt; Op(Else) and Op(End) follow.
func (c *Code) If(bt BlockType) { *c = append(*c, 0x04, byte(bt)) }

// Br appends br to the label depth blocks out.
func (c *Code) Br(depth uint32) { c.withIndex(0x0c, depth) }

// BrIf appends br_if to the label depth blocks out.
func (c *Code) BrIf(depth uint32) { c.withIndex(0x0d, depth) }

func (c *Code) memory(op byte, align, offset uint32) {
	*c = uleb(uleb(append(*c, op), uint64(align)), uint64(offset))
}

// I32Load appends i32.load at offset.
func (c *Code) I32Load(offset uint32) { c.memory(0x28, 2, offset) }

// I64Load appends i64.load at offset.
func (c *Code) I64Load(offset uint32) { c.memory(0x29, 3, offset) }

// I32Load8U appends i32.load8_u at offset.
func (c *Code) I32Load8U(offset uint32) { c.memory(0x2d, 0, offset) }

// I32Store appends i32.store at offset.
func (c *Code) I32Store(offset uint32) { c.memory(0x36, 2, offset) }

// I64Store appends i64.store at offset.
func (c *Code) I64Store(offset uint32) { c.memory(0x37, 3, offset) }

// I32Store8 appends i32.store8 at offset.
func (c *Code) I32Store8(offset uint32) { c.memory(0x3a, 0, offset) }

// MemorySize appends memory.size: the memory's size in 64KiB pages.
func (c *Code) MemorySize() { *c = append(*c, 0x3f, 0x00) }

// MemoryGrow appends memory.grow: grow by the pages on the stack, leaving
// the old size, or -1 if the memory can't grow.
func (c *Code) MemoryGrow() { *c = append(*c, 0x40, 0x00) }
