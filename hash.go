package glisp

import (
	"encoding/binary"
	"fmt"
)

// Hashing, for maps: 32-bit FNV-1a over a canonical encoding of a value. It
// is the same in both backends, since a map prints in the order of its keys'
// hashes. The encoding: an integer is 'i' and its 8 bytes, little-endian;
// nil, true and false are 'n', 't' and 'f'; a string is 's', its length in
// bytes (4 bytes, little-endian) and its bytes, and a symbol the same after
// 'y'; a list is '(', its elements' encodings and ')'; a vector the same
// between '[' and ']'. Values eq finds equal encode alike. Functions have no
// hash, since eq never finds two of them equal.

const (
	fnvOffset uint32 = 2166136261
	fnvPrime  uint32 = 16777619
)

// hashValue is v's hash. fn names the builtin, for the error a function
// gives.
func hashValue(fn string, v interface{}) uint32 {
	h := fnvOffset
	hashInto(fn, &h, v)
	return h
}

// hashInto feeds v's encoding into the hash *h.
func hashInto(fn string, h *uint32, v interface{}) {
	put := func(bytes ...byte) {
		for _, b := range bytes {
			*h = (*h ^ uint32(b)) * fnvPrime
		}
	}
	text := func(tag byte, s string) {
		put(tag)
		put(binary.LittleEndian.AppendUint32(nil, uint32(len(s)))...)
		put([]byte(s)...)
	}
	switch v := v.(type) {
	case int64:
		put('i')
		put(binary.LittleEndian.AppendUint64(nil, uint64(v))...)
	case float64:
		put('d')
		put(binary.LittleEndian.AppendUint64(nil, floatBits(v))...)
	case nil:
		put('n')
	case bool:
		if v {
			put('t')
		} else {
			put('f')
		}
	case string:
		text('s', v)
	case Symbol:
		text('y', v.Str())
	case List:
		put('(')
		for _, x := range v {
			hashInto(fn, h, x)
		}
		put(')')
	case Vector:
		put('[')
		for _, x := range v {
			hashInto(fn, h, x)
		}
		put(']')
	case Data:
		put('#')
		hashInto(fn, h, v.name)
		for _, x := range v.fields {
			hashInto(fn, h, x)
		}
		put(')')
	default:
		panic(fmt.Sprintf("%s: can't hash %s", fn, described(v)))
	}
}

// hashBuiltin is (hash x): x's hash, a number from 0 to 2^32-1.
func hashBuiltin(_ *Scope, args List) interface{} {
	return int64(hashValue("hash", args[0]))
}
