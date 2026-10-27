// Command glisp runs a glisp program and prints the value of its last
// expression, or with -wasm compiles it to a WebAssembly module for WASI.
//
//	glisp FILE.glisp
//	glisp -wasm OUT.wasm FILE.glisp
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tychofreeman/glisp"
)

func main() {
	wasmOut := flag.String("wasm", "", "compile to a WebAssembly (WASI) module in `file` instead of running")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: glisp [-wasm OUT.wasm] FILE.glisp")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if *wasmOut != "" {
		module, err := glisp.CompileWasmFile(flag.Arg(0))
		if err == nil {
			err = os.WriteFile(*wasmOut, module, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "glisp:", err)
			os.Exit(1)
		}
		return
	}
	value, err := glisp.RunFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "glisp:", err)
		os.Exit(1)
	}
	fmt.Println("=>", glisp.Show(value))
}
