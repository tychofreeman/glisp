package glisp

import (
	"os"
	"testing"
)

// withGCThreshold compiles with the collector running once the heap grows
// by threshold bytes, until the test ends.
func withGCThreshold(t *testing.T, threshold int) {
	saved := gcThreshold
	gcThreshold = threshold
	t.Cleanup(func() { gcThreshold = saved })
}

// With a tiny threshold the collector runs at nearly every call, moving
// every object over and over: anything it misses or fails to update shows
// up as a wrong answer or a crash.
func TestCompiledProgramsSurviveConstantCollection(t *testing.T) {
	withGCThreshold(t, 64)
	var programs []string
	programs = append(programs, wasmPrograms...)
	for _, c := range stringPrograms {
		programs = append(programs, "(p "+c.src+") (str "+c.src+")")
	}
	for _, c := range lineErrors {
		programs = append(programs, c.src)
	}
	for _, program := range programs {
		assertSameOutput(t, program)
	}
}

func TestCompiledExamplesSurviveFrequentCollection(t *testing.T) {
	withGCThreshold(t, 16<<10)
	for _, file := range []string{"examples/numbers.glisp", "examples/tour.glisp"} {
		program, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		assertSameOutput(t, string(program))
	}
}

// A program that allocates far more than it keeps runs in bounded memory.
func TestCompiledGarbageIsCollected(t *testing.T) {
	withGCThreshold(t, 1<<20)
	module, err := CompileWasm(`
		(defn range (a b) (if (> a b) nil (cons a (range (plus a 1) b))))
		(defn sum (l acc) (if (eq l nil) acc (if (eq l (quote ())) acc (sum (cdr l) (plus acc (car l))))))
		(defn churn (n kept acc)
		  (if (eq n 0) (list? kept)
		    (churn (- n 1) (cons n kept) (plus acc (sum (range 1 100) 0)))))
		(churn 50000 nil 0)`)
	if err != nil {
		t.Fatal(err)
	}
	m := instantiate(t, module)
	defer m.close()
	if err := m.start(); err != nil {
		t.Fatalf("%v: %s", err, m.out.String())
	}
	if m.out.String() != "=> true\n" {
		t.Errorf("printed %q", m.out.String())
	}
	// 50,000 lists of 100 pairs is 120MB allocated; what's kept is a list
	// of 50,000, about 1MB. Memory holds the 32MB argument stack and a heap
	// that doubles as it grows.
	if size := m.mod.Memory().Size(); size > 80<<20 {
		t.Errorf("memory grew to %d MB", size>>20)
	}
}

func TestEveryTagHasAKind(t *testing.T) {
	for tag := int32(tagInt); tag <= tagVector; tag++ {
		found := false
		for _, k := range tagKinds {
			found = found || k.tag == tag
		}
		if !found {
			t.Errorf("tag %d has no entry in tagKinds", tag)
		}
	}
}
