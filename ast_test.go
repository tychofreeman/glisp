package glisp

import "testing"

func TestNodesPrintAsCoreGlisp(t *testing.T) {
	program := Def{"inc_all", Lambda{
		Params: []string{"l"},
		Body: []Node{Apply{Ref{Name: "map"}, []Node{
			Lambda{Params: []string{"n"}, Body: []Node{Apply{Ref{Name: "plus"}, []Node{Ref{Name: "n"}, Const{int64(1)}}}}},
			Ref{Name: "l"},
		}}},
	}}
	want := `(def inc_all (lambda (l) (map (lambda (n) (plus n 1)) l)))`
	if got := program.String(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	for _, c := range []struct {
		node Node
		want string
	}{
		{If{Ref{Name: "c"}, Const{"yes"}, Const{nil}}, `(if c "yes" nil)`},
		{Quote{List{symbol("a"), int64(1), List{"s", nil}}}, `(quote (a 1 ("s" nil)))`},
		{Apply{Lambda{Body: []Node{Const{int64(7)}}}, nil}, `((lambda () 7))`},
		{Def{"flag", Const{true}}, `(def flag true)`},
	} {
		if got := c.node.String(); got != c.want {
			t.Errorf("got %s, want %s", got, c.want)
		}
	}
}
