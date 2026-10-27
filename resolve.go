package glisp

// Resolve returns node with each variable reference inside a lambda given
// its Loc, and each Lambda the variables it captures from the lambdas around
// it, for backends that compile variable access. A lambda's slots are its
// parameters, then the names it defines with def (outside nested lambdas).
// References to top-level and builtin names keep a nil Loc: a backend finds
// those by name.
func Resolve(node Node) Node {
	return (&resolver{}).resolve(node)
}

// frame is a lambda being resolved: its slots, and the variables from
// enclosing lambdas that it captures.
type frame struct {
	slots    []string
	captured []string
}

type resolver struct {
	frames []*frame // innermost last
}

func (r *resolver) resolve(node Node) Node {
	switch n := node.(type) {
	case Ref:
		return r.ref(n)
	case If:
		return If{r.resolve(n.Cond), r.resolve(n.Then), r.resolve(n.Else)}
	case Def:
		return Def{n.Name, r.resolve(n.Value)}
	case Apply:
		return Apply{r.resolve(n.Fn), r.resolveAll(n.Args)}
	case Lambda:
		f := &frame{slots: append([]string(nil), n.Params...)}
		for _, name := range localDefs(n.Body) {
			f.slots = appendNew(f.slots, name)
		}
		r.frames = append(r.frames, f)
		body := r.resolveAll(n.Body)
		r.frames = r.frames[:len(r.frames)-1]
		return Lambda{Params: n.Params, Body: body, Captured: f.captured}
	default:
		return node
	}
}

func (r *resolver) resolveAll(nodes []Node) []Node {
	resolved := make([]Node, len(nodes))
	for i, n := range nodes {
		resolved[i] = r.resolve(n)
	}
	return resolved
}

// ref finds the innermost lambda with a slot named n.Name. Every lambda
// between that one and the reference captures the variable.
func (r *resolver) ref(n Ref) Node {
	for depth := 0; depth < len(r.frames); depth++ {
		f := r.frames[len(r.frames)-1-depth]
		for index, slot := range f.slots {
			if slot == n.Name {
				for _, inner := range r.frames[len(r.frames)-depth:] {
					inner.captured = appendNew(inner.captured, n.Name)
				}
				return Ref{Name: n.Name, Loc: &Loc{Depth: depth, Index: index}}
			}
		}
	}
	return n
}

// localDefs returns the names nodes define with def, not counting nested
// lambdas, which have their own slots.
func localDefs(nodes []Node) []string {
	var names []string
	for _, node := range nodes {
		switch n := node.(type) {
		case Def:
			names = appendNew(names, n.Name)
			names = append(names, localDefs([]Node{n.Value})...)
		case If:
			names = append(names, localDefs([]Node{n.Cond, n.Then, n.Else})...)
		case Apply:
			names = append(names, localDefs(append([]Node{n.Fn}, n.Args...))...)
		}
	}
	return names
}

// appendNew appends name to names unless it is already there.
func appendNew(names []string, name string) []string {
	for _, existing := range names {
		if existing == name {
			return names
		}
	}
	return append(names, name)
}
