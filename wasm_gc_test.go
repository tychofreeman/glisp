package glisp

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
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
		if got, want := runCompiled(t, program), runInterpreted(t, program); got != want {
			t.Errorf("%s\ncompiled:    %q\ninterpreted: %q", program, got, want)
		}
	}
}

func TestCompiledExamplesSurviveFrequentCollection(t *testing.T) {
	withGCThreshold(t, 16<<10)
	for _, file := range []string{"examples/numbers.glisp", "examples/tour.glisp"} {
		program, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := runCompiled(t, string(program)), runInterpreted(t, string(program)); got != want {
			t.Errorf("%s: compiled and interpreted output differ\ncompiled:\n%s\ninterpreted:\n%s", file, got, want)
		}
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
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	var out bytes.Buffer
	mod, err := r.InstantiateWithConfig(ctx, module, wazero.NewModuleConfig().WithStdout(&out).WithStderr(&out))
	if err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if out.String() != "=> true\n" {
		t.Errorf("printed %q", out.String())
	}
	// 50,000 lists of 100 pairs is 120MB allocated; what's kept is a list
	// of 50,000, about 1MB. Memory holds the 32MB argument stack and a heap
	// that doubles as it grows.
	if size := mod.Memory().Size(); size > 80<<20 {
		t.Errorf("memory grew to %d MB", size>>20)
	}
}
