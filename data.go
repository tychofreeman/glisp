package glisp

import (
	"fmt"
	"strings"
)

// Data is a value made by a constructor of a data type, (data name x ...),
// which the constructors deftype defines call. It compares, prints and
// hashes by its constructor's name and its fields.
type Data struct {
	name   Symbol
	fields []interface{}
}

// makeData is (data name x ...).
func makeData(_ *Scope, args List) interface{} {
	return Data{args[0].(Symbol), append([]interface{}{}, args[1:]...)}
}

// dataName is (data_name d): the name of d's constructor.
func dataName(_ *Scope, args List) interface{} {
	return args[0].(Data).name
}

// dataRef is (data_ref d i): d's field i, counting from 0.
func dataRef(_ *Scope, args List) interface{} {
	d, i := args[0].(Data), args[1].(int64)
	if i < 0 || i >= int64(len(d.fields)) {
		panic(fmt.Sprintf("data_ref: index %d out of range for %s", i, Write(d)))
	}
	return d.fields[i]
}

// dataLength is (data_length d): how many fields d has.
func dataLength(_ *Scope, args List) interface{} {
	return int64(len(args[0].(Data).fields))
}

// write is d as Write writes it: #(name field ...).
func (d Data) write() string {
	parts := []string{d.name.Str()}
	for _, field := range d.fields {
		parts = append(parts, Write(field))
	}
	return "#(" + strings.Join(parts, " ") + ")"
}
