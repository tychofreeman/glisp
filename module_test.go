package glisp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Programs that import modules from files print the same in both
// backends, which is what each case wants.
func TestModulesImport(t *testing.T) {
	geometry := "(module geometry (export area square))\n(defn area (w h) (* w h))\n(defn square (s) (area s s))\n(defn helper (x) x)"
	util := "(module util (export twice unless))\n(defn twice (f x) (f (f x)))\n(defmacro unless (lambda (xs) (list (quote if) (car xs) (car (cdr (cdr xs))) (car (cdr xs)))))"
	for _, c := range []struct {
		files map[string]string
		want  string
	}{
		{map[string]string{"geometry": geometry,
			"main": "(import geometry)\n(p (geometry/area 2 3) (geometry/square 4))"},
			"6 16\n=> ()\n"},
		// Unqualified and under another name; a lambda's parameter hides an
		// unqualified import.
		{map[string]string{"geometry": geometry,
			"main": "(import geometry (area))\n(import geometry as g)\n(p (area 2 3) (g/square 2))\n(defn f (area) area)\n(p (f 9))"},
			"6 4\n9\n=> ()\n"},
		// The module's own area is the one its square uses.
		{map[string]string{"geometry": geometry,
			"main": "(import geometry)\n(defn area (x) 0)\n(p (area 1) (geometry/square 3))"},
			"0 9\n=> ()\n"},
		// A module imports another; macros are exported too.
		{map[string]string{"geometry": geometry, "util": util,
			"shapes": "(module shapes (export double_area))\n(import util)\n(import geometry)\n(defn double_area (w h) (util/twice (lambda (x) (* 2 x)) (geometry/area w h)))",
			"main":   "(import shapes)\n(import util (unless))\n(p (shapes/double_area 1 2) (unless false \"yes\" \"no\") (util/unless true 1 2))"},
			"8 yes 2\n=> ()\n"},
		{map[string]string{"geometry": geometry,
			"main": "(import geometry)\n(geometry/helper 1)"},
			"glisp: line 2: geometry/helper: geometry doesn't export helper\n"},
		{map[string]string{"geometry": geometry,
			"main": "(import geometry (volume))"},
			"glisp: line 1: import: geometry doesn't export volume\n"},
		{map[string]string{"geometry": geometry,
			"main": "(import geometry (area))\n(def area 1)"},
			"glisp: line 2: def: area is imported from geometry\n"},
		{map[string]string{"geometry": geometry,
			"rect": "(module rect (export area))\n(defn area (w h) (* w h))",
			"main": "(import geometry (area))\n(import rect (area))\n(p (area 1 2))"},
			"glisp: line 3: area is imported from more than one module: geometry/area, rect/area\n"},
		{map[string]string{"main": "(import nowhere)"},
			"glisp: line 1: import: no module nowhere: there is no nowhere.glisp next to the file that imports it\n"},
		{map[string]string{"plain": "(def z 1)", "main": "(def x 1)\n(import plain)"},
			"glisp: line 2: import: plain.glisp is not the module plain: it must start with (module plain ...)\n"},
		{map[string]string{
			"a":    "(module a (export x))\n(import b)\n(def x 1)",
			"b":    "(module b (export y))\n(import a)\n(def y 2)",
			"main": "(import a)"},
			// The line is b.glisp's: an error names no file yet.
			"glisp: line 2: import: a cycle: a imports b imports a\n"},
	} {
		dir := t.TempDir()
		for name, src := range c.files {
			if err := os.WriteFile(filepath.Join(dir, name+".glisp"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		main := filepath.Join(dir, "main.glisp")
		interpreted := printedBy(t, func() (interface{}, error) { return RunFile(main) })
		module, err := CompileWasmFile(main)
		compiled := compiledOutput(t, module, err)
		if msg, found := strings.CutPrefix(compiled, "compile error: "); found {
			compiled = "glisp: " + msg + "\n"
		}
		if interpreted != c.want || compiled != c.want {
			t.Errorf("%s\ninterpreted: %q\ncompiled:    %q\nwant:        %q", c.files["main"], interpreted, compiled, c.want)
		}
	}
}
