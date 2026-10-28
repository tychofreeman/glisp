package glisp

import (
	"os"
	"strings"
	"testing"
)

// A specExample is a program from docs/spec.md and what cmd/glisp prints
// for it.
type specExample struct {
	program, output string
	line            int // the line of the spec the program starts on
}

// specExamples reads the examples in a spec: each glisp code block, which
// the output block after it must follow.
func specExamples(t *testing.T, spec string) []specExample {
	t.Helper()
	lines := strings.Split(spec, "\n")
	var examples []specExample
	for i := 0; i < len(lines); i++ {
		if lines[i] != "```glisp" {
			continue
		}
		start := i + 1
		program, end := fenced(lines, start)
		next := end + 1
		for next < len(lines) && lines[next] == "" {
			next++
		}
		if next >= len(lines) || lines[next] != "```output" {
			t.Fatalf("docs/spec.md:%d: a glisp block needs an output block after it", start)
		}
		output, outEnd := fenced(lines, next+1)
		examples = append(examples, specExample{program, output, start + 1})
		i = outEnd
	}
	return examples
}

// fenced is the text of the code block whose first line is lines[start], and
// the index of its closing fence.
func fenced(lines []string, start int) (string, int) {
	end := start
	for end < len(lines) && lines[end] != "```" {
		end++
	}
	return strings.Join(lines[start:end], "\n") + "\n", end
}

func TestSpecExamples(t *testing.T) {
	spec, err := os.ReadFile("docs/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	examples := specExamples(t, string(spec))
	if len(examples) == 0 {
		t.Fatal("docs/spec.md has no examples")
	}
	for _, ex := range examples {
		if got := runInterpreted(t, ex.program); got != ex.output {
			t.Errorf("docs/spec.md:%d: interpreted, the program prints\n%s\nnot\n%s", ex.line, got, ex.output)
		}
		if got := compiledPrints(t, ex.program); got != ex.output {
			t.Errorf("docs/spec.md:%d: compiled, the program prints\n%s\nnot\n%s", ex.line, got, ex.output)
		}
	}
}

// compiledPrints is what compiling program with glisp -wasm and running the
// module prints: an error found while compiling is reported as the command
// reports it.
func compiledPrints(t *testing.T, program string) string {
	t.Helper()
	got := runCompiled(t, program)
	if msg, found := strings.CutPrefix(got, "compile error: "); found {
		return "glisp: " + msg + "\n"
	}
	return got
}
