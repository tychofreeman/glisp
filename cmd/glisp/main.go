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
	if _, err := os.Stat(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("=>", glisp.ProcessFile(os.Args[1]))
}
