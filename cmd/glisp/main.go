// Command glisp runs a glisp program and prints the value of its last expression.
//
//	glisp FILE.glisp
package main

import (
	"fmt"
	"os"

	"github.com/tychofreeman/glisp"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: glisp FILE.glisp")
		os.Exit(2)
	}
	value, err := glisp.RunFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "glisp:", err)
		os.Exit(1)
	}
	fmt.Println("=>", value)
}
