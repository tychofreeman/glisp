package glisp

import "fmt"

// Vector is a glisp vector: a fixed sequence of values, read by index in
// constant time. Vectors don't change; vector_set makes a new one.
type Vector []interface{}

// makeVector is (vector x ...).
func makeVector(_ *Scope, args List) interface{} {
	return append(Vector{}, args...)
}

// vectorRef is (vector_ref v i): the value at i.
func vectorRef(_ *Scope, args List) interface{} {
	v := args[0].(Vector)
	return v[vectorIndex("vector_ref", v, args[1].(int64))]
}

// vectorLength is (vector_length v).
func vectorLength(_ *Scope, args List) interface{} {
	return int64(len(args[0].(Vector)))
}

// vectorSet is (vector_set v i x): a copy of v with x at i.
func vectorSet(_ *Scope, args List) interface{} {
	v := args[0].(Vector)
	i := vectorIndex("vector_set", v, args[1].(int64))
	w := append(Vector{}, v...)
	w[i] = args[2]
	return w
}

// vectorToList is (vector_to_list v): v's values as a list.
func vectorToList(_ *Scope, args List) interface{} {
	return append(List{}, args[0].(Vector)...)
}

// vectorIndex returns i, an index into v given to the builtin fn, or panics
// if it is out of range.
func vectorIndex(fn string, v Vector, i int64) int64 {
	if i < 0 || i >= int64(len(v)) {
		panic(fmt.Sprintf("%s: index %d out of range for a vector of length %d", fn, i, len(v)))
	}
	return i
}
