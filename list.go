package glisp

type List []interface{}

func (all List) First() interface{} {
	if all != nil && len(all) > 0 {
		return all[0]
	}
	return nil
}

func (all List) Second() interface{} {
	return all.Rest().First()
}

func (all List) Rest() List {
	if all != nil && len(all) > 0 {
		return List(all[1:])
	}
	return nil
}

// A Pair is a list cons made: its first element, and the list after it,
// which it shares, so cons takes constant time where a List, a Go slice,
// would copy its tail. The reader, a builtin's arguments and the builtins
// that build a whole list make Lists; either is a glisp list.
type Pair struct {
	car interface{}
	cdr interface{} // a List or a *Pair
}

// uncons is a non-empty list's first element and the list after it.
func uncons(v interface{}) (first, rest interface{}, nonEmpty bool) {
	switch v := v.(type) {
	case List:
		if len(v) > 0 {
			return v[0], v[1:], true
		}
	case *Pair:
		return v.car, v.cdr, true
	}
	return nil, nil, false
}

// isList reports whether v is a list: a List or a Pair.
func isList(v interface{}) bool {
	switch v.(type) {
	case List, *Pair:
		return true
	}
	return false
}

// asList is v as a List, if v is a list: a Pair's elements are copied into
// one.
func asList(v interface{}) (List, bool) {
	if l, isSlice := v.(List); isSlice {
		return l, true
	}
	if !isList(v) {
		return nil, false
	}
	l := List{}
	for first, rest, nonEmpty := uncons(v); nonEmpty; first, rest, nonEmpty = uncons(rest) {
		l = append(l, first)
	}
	return l, true
}

// withoutPairs is v with every Pair in it, at any depth, made a List: what
// a macro gives the expander, and a program gives Go. A list without Pairs
// stays as it is, so the lines of the lists read from source stay found;
// one with Pairs below is changed in place, which keeps its value.
func withoutPairs(v interface{}) interface{} {
	if !hasPairs(v) {
		return v
	}
	l, _ := asList(v)
	for i, x := range l {
		l[i] = withoutPairs(x)
	}
	return l
}

// hasPairs reports whether v is a Pair, or a List with one at any depth.
func hasPairs(v interface{}) bool {
	switch v := v.(type) {
	case *Pair:
		return true
	case List:
		for _, x := range v {
			if hasPairs(x) {
				return true
			}
		}
	}
	return false
}
